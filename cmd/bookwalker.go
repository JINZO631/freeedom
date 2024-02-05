package cmd

import (
	"context"
	"fmt"

	"github.com/JINZO631/freeedom/pkg/bookwalker"
	"github.com/spf13/cobra"
)

func init() {
	var (
		afterDate  string
		beforeDate string
		outputDir  string
	)

	// bookwalkerCmd represents the bookwalker command
	var bookwalkerCmd = &cobra.Command{
		Use:   "bookwalker",
		Short: "BOOLWALKERから領収書PDFをダウンロードします。",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			if err := bookwalker.Run(context.Background(), afterDate, beforeDate, outputDir); err != nil {
				fmt.Println(err)
			}
		},
	}

	rootCmd.AddCommand(bookwalkerCmd)
	bookwalkerCmd.Flags().StringVarP(&afterDate, "after", "a", "", "検索範囲の開始年月 (format: 202401)")
	bookwalkerCmd.Flags().StringVarP(&beforeDate, "before", "b", "", "検索範囲の終了年月 (format: 202401)")
	bookwalkerCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "出力先ディレクトリ (デフォルト: カレントディレクトリ)")

	bookwalkerCmd.MarkFlagRequired("after")
}
