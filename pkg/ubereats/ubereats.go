package ubereats

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/net/html"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Run GmailからUberEatsの領収書のメールを探してダウンロードする
func Run(ctx context.Context, oauthClientJSONPath, afterDate, beforeDate string) error {

	// Gmailの設定を取得
	b, err := os.ReadFile(oauthClientJSONPath)
	if err != nil {
		return err
	}
	config, err := google.ConfigFromJSON(b, gmail.GmailReadonlyScope)
	if err != nil {
		return err
	}

	// リダイレクト先のURLを設定
	config.RedirectURL = "http://localhost:8080/callback"

	// トークンが保存されているか確認し、保存されている場合はそれを使う
	token, err := GetToken(ctx, config)
	if err != nil {
		return err
	}

	token, err = RefreshToken(ctx, config, token)
	if err != nil {
		return err
	}

	// Gmailサービスを作成
	client := config.Client(ctx, token)
	gmailService, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	// UberEatsの領収書のメールを探す
	fmt.Println("UberEatsの領収書のメールを探します。")
	query := fmt.Sprintf("after:%s before:%s subject:%s", afterDate, beforeDate, "Uber の領収書")
	fmt.Println("query: ", query)

	// メール取得開始
	mails, err := getEmails(gmailService, query)
	if err != nil {
		return err
	}

	fmt.Println("メールを取得しました。 取得数: ", len(mails))

	// メールから領収書PDFのリンクを取り出す
	pdfLinks := []*PDFLink{}
	for i, mail := range mails {
		pdfLink, err := extractPDFLink(mail)
		if err != nil {
			fmt.Println(color.RedString("×"), i, mail.Id)

			// PDFリンクが見つからなかった場合はファイルとして保存しておく
			var pdfLinkNotFound *pdfLinkNotFound
			if errors.As(err, &pdfLinkNotFound) {

				fileName, err := writePDFLinkNotFoundHTML(pdfLinkNotFound)
				if err != nil {
					return err
				}
				fmt.Println("PDFのリンクが見つからなかったメールのHTMLを保存しました。", fileName)
			} else {
				return err
			}
		} else {
			// fmt.Println(color.GreenString("✓"), i, mail.Id)
		}
		pdfLinks = append(pdfLinks, pdfLink)
	}

	// Chromeを自動操作してPDFをダウンロードする
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	chromedpCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	fmt.Println("Chromeを自動操作してPDFをダウンロードします。")
	bar := progressbar.Default(int64(len(pdfLinks)))
	for i, pdfLink := range pdfLinks {

		if i == 0 {
			// 初回のみログイン操作が必要なため処理を変える
			downloadFirstPDF(chromedpCtx, pdfLink)
		} else {
			downloadPDF(chromedpCtx, pdfLink)
		}

		// TooManyRequestsを回避するために待機する
		time.Sleep(5 * time.Second)

		bar.Add(1)
	}

	fmt.Println("PDFのダウンロードが完了しました。")
	return nil
}

// generateRandomState OAuth2用のランダムなstate文字列を生成する
func generateRandomState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// getEmails クエリにマッチするメールを取得する
func getEmails(srv *gmail.Service, query string) ([]*gmail.Message, error) {
	var messages []*gmail.Message

	req := srv.Users.Messages.List("me").Q(query)
	for {
		res, err := req.Do()
		if err != nil {
			return nil, err
		}

		messages = append(messages, res.Messages...)
		if res.NextPageToken == "" {
			break
		}

		req.PageToken(res.NextPageToken)
	}

	fullMessages := make([]*gmail.Message, len(messages))
	count := len(messages)
	bar := progressbar.Default(int64(count))
	for i, message := range messages {
		fullMessage, err := srv.Users.Messages.Get("me", message.Id).Do()
		if err != nil {
			return nil, err
		}
		fullMessages[i] = fullMessage
		bar.Add(1)
	}

	return fullMessages, nil
}

// PDFLink PDFリンクと支払日の情報を持つ構造体
type PDFLink struct {
	URL  string
	Date string
}

// extractPDFLink メールからPDFのリンクを抽出する
func extractPDFLink(message *gmail.Message) (*PDFLink, error) {

	// base64された本文htmlをデコードする
	data, err := base64.URLEncoding.DecodeString(message.Payload.Body.Data)
	if err != nil {
		return nil, err
	}
	htmlContent := string(data)

	// htmlをパースする
	r := strings.NewReader(htmlContent)
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	// PDFのリンクを抽出する
	pdfURL := findPDFLink(doc)
	if pdfURL == "" {

		// PDFのリンクが見つからなかった場合はエラーを返す
		return nil, &pdfLinkNotFound{
			Message:     "メール本文からPDFリンクが見つかりません",
			ID:          message.Id,
			HTMLContent: htmlContent,
		}
	}

	// 支払日を取得する
	date := findPaymentDate(doc)
	if date == "" {
		return nil, errors.New("メール本文から支払日が見つかりません")
	}

	// PDFのリンクと支払日を返す
	return &PDFLink{
		URL:  pdfURL,
		Date: date,
	}, nil
}

// findPDFLink メール本文のhtmlからPDFのリンクを探す
func findPDFLink(n *html.Node) string {
	// タグが<a>である要素を探す
	if n.Type == html.ElementNode && n.Data == "a" {
		// リンクのテキストが「この PDF をダウンロードしてください」 or 「PDF をダウンロードする >」であるかどうかを確認
		text := n.FirstChild.Data
		switch text {
		case "この PDF をダウンロードしてください", "PDF をダウンロードする >":
			// href属性を取得
			for _, a := range n.Attr {
				if a.Key == "href" {
					return a.Val
				}
			}
		}
	}

	// 子ノードを再帰的に探索
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		link := findPDFLink(c)
		if link != "" {
			return link
		}
	}

	return ""
}

// findPaymentDate メール本文のhtmlから支払日を探す
func findPaymentDate(n *html.Node) string {
	// タグが<span>である要素を探す
	if n.Type == html.ElementNode && n.Data == "span" {
		// リンクのテキストがyyyy年mm月dd日であるかどうかを確認
		text := n.FirstChild.Data
		re := regexp.MustCompile(`\d{4}年\d{1,2}月\d{1,2}日`)
		if re.MatchString(text) {
			// yyyy年mm月dd日をyyyy-mm-ddに変換
			text = strings.Replace(text, "年", "-", 1)
			text = strings.Replace(text, "月", "-", 1)
			text = strings.Replace(text, "日", "", 1)
			return text
		}
	}

	// 子ノードを再帰的に探索
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		date := findPaymentDate(c)
		if date != "" {
			return date
		}
	}

	return ""
}

// pdfLinkNotFound PDFリンクが見つからなかった場合のエラー
type pdfLinkNotFound struct {
	Message     string
	ID          string
	HTMLContent string
}

func (e *pdfLinkNotFound) Error() string {
	return fmt.Sprintf("メール本文からPDFリンクが見つかりません。 Message: %s", e.Message)
}

func writePDFLinkNotFoundHTML(pdfLinkNotFound *pdfLinkNotFound) (string, error) {
	fileName := "error_" + pdfLinkNotFound.ID + ".html"
	file, err := os.Create(fileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = file.WriteString(pdfLinkNotFound.HTMLContent)
	if err != nil {
		return "", err
	}
	return fileName, nil
}

// downloadFirstPDF 初回のPDFをダウンロードする
func downloadFirstPDF(ctx context.Context, pdfLink *PDFLink) {

	chromedp.Run(ctx,
		chromedp.Navigate(pdfLink.URL),
	)
	fmt.Printf("初回はUberEatsのログイン操作が必要です、Chromeでのダウンロードが完了したらEnterを押して処理を続行してください。")

	bufio.NewScanner(os.Stdin).Scan()
}

// downloadPDF PDFをダウンロードする
func downloadPDF(ctx context.Context, pdfLink *PDFLink) {

	chromedp.Run(ctx,
		chromedp.Navigate(pdfLink.URL),
	)
}
