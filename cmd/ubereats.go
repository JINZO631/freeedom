package cmd

import (
	"context"
	"log"
	"os"

	"github.com/JINZO631/freeedom/pkg/ubereats"
	"github.com/spf13/cobra"
)

func init() {
	var (
		afterDate  string
		beforeDate string
	)
	var ubereatsCmd = &cobra.Command{
		Use:   "ubereats",
		Short: "Gmailに保存されているメールからUberEatsの領収書PDFをダウンロードします。PDFはChromeのダウンロードフォルダに保存されます。",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			if err := ubereats.Run(context.Background(), os.Getenv("RECEIPT_DOWNLOADER_GMAIL_API_CREDENTIALS_PATH"), afterDate, beforeDate); err != nil {
				log.Fatalln(err)
			}
		},
	}
	rootCmd.AddCommand(ubereatsCmd)

	ubereatsCmd.Flags().StringVarP(&afterDate, "after", "a", "", "after date (format: 2024-01-01)")
	ubereatsCmd.Flags().StringVarP(&beforeDate, "before", "b", "", "before date (format: 2024-01-01)")

	ubereatsCmd.MarkFlagRequired("after")
	ubereatsCmd.MarkFlagRequired("before")
}
