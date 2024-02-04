package gmail

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gm "google.golang.org/api/gmail/v1"
)

// NewConfig Gmailの設定を返す
func NewConfig(configJSONBytes []byte) (*oauth2.Config, error) {
	config, err := google.ConfigFromJSON(configJSONBytes, gm.GmailReadonlyScope)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func NewClient(ctx context.Context, config *oauth2.Config, token *oauth2.Token) *http.Client {

	return config.Client(ctx, token)
}

func NewService(ctx context.Context, config *oauth2.Config) (*gm.Service, error) {

	// token :=

	return nil, nil
}

// GetAuthCodeURL 認証コードを取得するURLを返す
func GetAuthCodeURL(config *oauth2.Config) string {

	return config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
}
