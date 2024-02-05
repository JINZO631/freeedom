package bookwalker

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

// Run メイン処理
func Run(ctx context.Context, after, before, outputDir string) error {

	// 取得対象の年月範囲を生成
	targetDate, err := generateYearMonths(after, before)
	if err != nil {
		return err
	}

	// chromedpの設定
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	ctx, cancel = chromedp.NewContext(allocCtx)
	defer cancel()

	// BOOKWALKERログイン
	fmt.Println("Chromeを自動操作してBOOKWALKERにログインします。")
	fmt.Println("ログイン情報を入力してください。")
	fmt.Printf("メールアドレス📩: ")
	email := ""
	// メールアドレスを入力させる
	fmt.Scanln(&email)
	fmt.Printf("パスワード🔑: ")
	password, err := ReadPassword()
	if err != nil {
		return err
	}
	fmt.Println()

	// Chromeでのログイン処理
	if err := Login(ctx, email, string(password)); err != nil {
		return err
	}

	// reCAPTCHAが入ることがあるのでそれを待機する
	fmt.Println("ログインボタンを押してください。(reCAPTCHAが表示されたら手動で操作して完了してください)")
	if err := WaitLogin(ctx); err != nil {
		return err
	}

	// 領収書ページを開いて各領収書のURLを取得
	fmt.Println("領収書のURLを取得します")
	receiptURLs := []string{}
	gerURLProgressBar := progressbar.Default(int64(len(targetDate)))
	for _, date := range targetDate {
		// TODO: 決済履歴ページのページネーションに対応する、現状は1ページ目のみ見ている
		urls, err := GetReceiptURLs(ctx, date, 1)
		if err != nil {
			return err
		}
		receiptURLs = append(receiptURLs, urls...)
		gerURLProgressBar.Add(1)
	}

	fmt.Println("領収書のURLを取得しました 件数:", len(receiptURLs))

	// 領収書をダウンロード
	fmt.Println("領収書をダウンロードします")
	downloadProgressBar := progressbar.Default(int64(len(receiptURLs)))
	for _, url := range receiptURLs {
		if err := DownloadReceipt(ctx, url, outputDir); err != nil {
			return err
		}
		downloadProgressBar.Add(1)
	}

	return nil
}

// ReadPassword パスワード入力モードで入力させる
func ReadPassword() ([]byte, error) {
	// Ctrl+Cのシグナルをキャプチャする
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	defer signal.Stop(signalChan)

	// 現在のターミナルの状態をコピーしておく
	currentState, err := term.GetState(int(syscall.Stdin))
	if err != nil {
		return nil, err
	}

	go func() {
		<-signalChan
		// Ctrl+Cを受信後、ターミナルの状態を先ほどのコピーを用いて元に戻す
		term.Restore(int(syscall.Stdin), currentState)
		os.Exit(1)
	}()

	return term.ReadPassword(syscall.Stdin)
}

// Login Chromeを自動操作してBOOKWALKERにログインする
func Login(ctx context.Context, email string, password string) error {
	chromedp.Run(ctx,
		chromedp.Navigate("https://member.bookwalker.jp/app/03/login"), // BOOKWALKERのログインページに遷移
		chromedp.WaitVisible(`#mailAddress`, chromedp.ByQuery),         // メールアドレスの入力欄が表示されるまで待機
		chromedp.SendKeys(`#mailAddress`, email, chromedp.ByQuery),     // メールアドレスを入力
		chromedp.SendKeys(`#password`, password, chromedp.ByQuery),     // パスワードを入力
	)
	return nil
}

// WaitLogin ログイン完了まで待機する
func WaitLogin(ctx context.Context) error {
	chromedp.Run(ctx,
		chromedp.WaitVisible((`#lt_payment_history`), chromedp.ByID), // 決済履歴ボタンが表示されるまで待機
	)
	return nil
}

// GetReceiptURLs BOOKWALKERの領収書のURLを取得する
// date: YYYYMM
// page: ページ(1始まり)
func GetReceiptURLs(ctx context.Context, date string, page int) ([]string, error) {
	// 決済履歴ページのURL
	paymentHistoryURL := fmt.Sprintf("https://member.bookwalker.jp/app/03/my/paymenthistory/%s?page=%d", date, page)
	// fmt.Println("決済履歴ページのURL : " + paymentHistoryURL)

	// 決済履歴ページから領収書URLを取得
	var receiptURLs []string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(paymentHistoryURL),
		chromedp.Evaluate(`
			Array.from(document.querySelectorAll('.PaymentDetails')).map(el => {
				const price = parseInt(el.querySelector('.payment_total .ja_val').innerText.replace(/,/g, ''), 10);
				const receiptLink = el.querySelector('.purchase_books .ja_val a');
				if (price > 0 && receiptLink) {
					return receiptLink.href;
				}
				return null;
			}).filter(url => url !== null);
		`, &receiptURLs),
	); err != nil {
		return nil, fmt.Errorf("failed to fetch receipt URLs: %w", err)
	}

	return receiptURLs, nil
}

// DownloadReceipt 領収書PDFページを開き、保存する
func DownloadReceipt(ctx context.Context, receiptURL string, outputDir string) error {
	var pdfBuf []byte
	if err := chromedp.Run(ctx,
		chromedp.Navigate(receiptURL),
		chromedp.WaitVisible(`#main1`, chromedp.ByQuery), // 領収書の要素が表示されるまで待機
		chromedp.ActionFunc(func(ctx context.Context) error {
			printParams := page.PrintToPDF()
			printParams.PrintBackground = true
			printParams.PaperWidth = 8.27 // A4 paper size
			printParams.PaperHeight = 11.69

			pdf, _, err := printParams.Do(ctx)
			if err != nil {
				return fmt.Errorf("failed to generate PDF: %w", err)
			}
			pdfBuf = pdf
			return nil
		}),
	); err != nil {
		return fmt.Errorf("failed to download receipt: %w", err)
	}

	// URLの https://user.bookwalker.jp/app/purchaseDetail/{id}/ja　から{id}部分を取り出す正規表現
	// 領収書のIDをファイル名にする
	receiptID := regexp.MustCompile(`https://user.bookwalker.jp/app/purchaseDetail/(\d+)/ja`).FindStringSubmatch(receiptURL)[1]

	// ダウンロードしたPDFを保存
	fileName := fmt.Sprintf("%s.pdf", receiptID)
	pdfPath := filepath.Join(outputDir, fileName)

	// ディレクトリがなければ作成する
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("出力先ディレクトリの作成に失敗しました: %w", err)
	}

	if err := os.WriteFile(pdfPath, pdfBuf, 0o644); err != nil {
		return fmt.Errorf("PDFの保存に失敗しました : %w", err)
	}

	return nil
}

// generateYearMonths 年月範囲の文字列のスライスを作る
func generateYearMonths(after, before string) ([]string, error) {
	// after: "202301"
	// before: "202312"
	// 例:[ "202301", "202302", "202303", ... , "202312"]
	// beforeが空文字の場合はafterの年月のみを取得対象とする

	startDate, err := time.Parse("200601", after)
	if err != nil {
		return nil, fmt.Errorf("error parsing start date: %v", err)
	}

	if before == "" {
		return []string{startDate.Format("200601")}, nil
	}

	endDate, err := time.Parse("200601", before)
	if err != nil {
		return nil, fmt.Errorf("error parsing end date: %v", err)
	}

	if endDate.Before(startDate) {
		return nil, fmt.Errorf("end date must be equal to or after start date")
	}

	yearMonths := []string{}
	current := startDate
	for !current.After(endDate) {
		yearMonths = append(yearMonths, current.Format("200601"))
		current = current.AddDate(0, 1, 0)
	}

	return yearMonths, nil
}
