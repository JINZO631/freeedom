package cmd

import (
	"context"
	"log"

	"github.com/JINZO631/freeedom/pkg/ubereats"
	"github.com/spf13/cobra"
)

func init() {
	var (
		gmailOAuthClientJSON string
		afterDate            string
		beforeDate           string
	)
	var ubereatsCmd = &cobra.Command{
		Use:   "ubereats",
		Short: "Gmailに保存されているメールからUberEatsの領収書PDFをダウンロードします。PDFはChromeのダウンロードフォルダに保存されます。",
		Long:  `GCP上でGmailAPIを有効化し、OAuthクライアントを作成し、そのクライアントのJSONをダウンロードして引数に指定してください。`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := ubereats.Run(context.Background(), gmailOAuthClientJSON, afterDate, beforeDate); err != nil {
				log.Fatalln(err)
			}
		},
	}
	rootCmd.AddCommand(ubereatsCmd)

	ubereatsCmd.Flags().StringVarP(&gmailOAuthClientJSON, "gmail-api-credentials-path", "g", "", "GmailAPIのクライアントJSONのパス")
	ubereatsCmd.Flags().StringVarP(&afterDate, "after", "a", "", "検索範囲の開始日 (format: 2024-01-01)")
	ubereatsCmd.Flags().StringVarP(&beforeDate, "before", "b", "", "検索範囲の終了日 (format: 2024-01-01)")

	ubereatsCmd.MarkFlagRequired("gmail-api-credentials-path")
	ubereatsCmd.MarkFlagRequired("after")
	ubereatsCmd.MarkFlagRequired("before")
}
