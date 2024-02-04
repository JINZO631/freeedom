package ubereats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/JINZO631/freeedom/pkg/configdir"
	"golang.org/x/oauth2"
)

// GmailAPITokenPath GmailAPIのトークンのパスを取得する
func GmailAPITokenPath() (string, error) {

	configDirPath, err := configdir.GetConfigDir()
	if err != nil {
		return "", err
	}

	tokenPath := filepath.Join(configDirPath, "/ubereats/token.json")
	return tokenPath, nil
}

// HasGmailAPIToken GmailAPIのトークンが保存されているか確認する
func HasGmailAPIToken() (bool, error) {
	tokenPath, err := GmailAPITokenPath()
	if err != nil {
		return false, err
	}

	_, err = os.Stat(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// GetToken トークンを取得する
func GetToken(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {

	// ローカルにトークンが保存されているばそれを、保存されていない場合はブラウザを開いて認証を行う
	hasToken, err := HasGmailAPIToken()
	if err != nil {
		return nil, err
	}

	if hasToken {
		// ローカルのトークンを読み込む
		tokenPath, err := GmailAPITokenPath()
		if err != nil {
			return nil, err
		}

		return GetTokenFromFile(tokenPath)
	}

	// ブラウザからトークンを取得
	token, err := GetTokenFromWeb(ctx, config)
	if err != nil {
		return nil, err
	}

	return token, nil
}

// RefreshToken トークンをリフレッシュする
func RefreshToken(ctx context.Context, config *oauth2.Config, token *oauth2.Token) (*oauth2.Token, error) {
	tokenSource := config.TokenSource(ctx, token)
	token, err := tokenSource.Token()
	if err != nil {
		if err := RemoveToken(); err != nil {
			return nil, err
		}
		return nil, err
	}

	// トークンを保存
	if err := SaveToken(token); err != nil {
		return nil, err
	}

	return token, nil
}

// SaveToken トークンをファイルに保存する
func SaveToken(token *oauth2.Token) error {
	tokenPath, err := GmailAPITokenPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
		return err
	}

	f, err := os.Create(tokenPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

// RemoveToken トークンを削除する
func RemoveToken() error {
	tokenPath, err := GmailAPITokenPath()
	if err != nil {
		return err
	}

	return os.Remove(tokenPath)
}

// GetTokenFromFile ファイルからトークンを取得する
func GetTokenFromFile(tokenPath string) (*oauth2.Token, error) {
	f, err := os.Open(tokenPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	token := &oauth2.Token{}
	if err := json.NewDecoder(f).Decode(token); err != nil {
		return nil, err
	}

	return token, nil
}

// GetTokenFromWeb ブラウザから認証を行いトークンを取得する
func GetTokenFromWeb(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	// 認証コードを取得するためのURLを生成
	oauthState, err := generateRandomState()
	if err != nil {
		return nil, err
	}
	authURL := config.AuthCodeURL(oauthState, oauth2.AccessTypeOffline)

	// 認証コードを取得するためのサーバーを起動
	errChan := make(chan error)
	quitChan := make(chan *oauth2.Token)
	srv := &http.Server{Addr: ":8080"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {

		// 認証コード取得時にstateをチェック
		state := r.FormValue("state")
		if state != oauthState {
			errChan <- err
		}

		// 認証コードを取得
		code := r.URL.Query().Get("code")
		token, err := config.Exchange(r.Context(), code)
		if err != nil {
			errChan <- err
		}

		// トークンを送り出す
		quitChan <- token
		close(quitChan)

		// サーバーをシャットダウン
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			errChan <- err
		}
	})

	// サーバー起動
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			errChan <- err
		}
	}()

	// ブラウザで認証ページを開き、操作の完了を待つ
	fmt.Println("ブラウザでメール取得を行うGoogleアカウントの認証を行ってください。")
	if err := OpenURL(authURL); err != nil {
		return nil, err
	}

	var token *oauth2.Token
	select {
	case err := <-errChan:
		// サーバー起動失敗エラーを受け取った場合
		return nil, err
	case data := <-quitChan:
		// サーバーシャットダウンを受け取った場合
		token = data
		fmt.Println("トークンを取得しました。")
	}

	// トークンを保存
	if err := SaveToken(token); err != nil {
		return nil, err
	}

	return token, nil
}

// OpenURL ブラウザでURLを開く
func OpenURL(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	default:
		return fmt.Errorf("unsupported platform")
	}

	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
