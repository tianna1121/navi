package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tianna1121/navi/internal/cdp"
	"github.com/tianna1121/navi/internal/output"
)

var (
	storageLocal   bool
	storageSession bool
	storageCookie  bool
	storageFilter  string
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "View browser storage (localStorage/sessionStorage/cookies)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		client := cdp.NewClient(cdpURL)

		storageType := "local"
		if storageSession {
			storageType = "session"
		} else if storageCookie {
			storageType = "cookie"
		}

		result, err := client.GetStorage(ctx, tabURL, tabTitle, tabIndex, storageType, storageFilter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

		if jsonOut {
			output.JSON(result)
		} else {
			output.Storage(result)
		}
		return nil
	},
}

func init() {
	storageCmd.Flags().BoolVar(&storageLocal, "local", false, "Show localStorage (default)")
	storageCmd.Flags().BoolVar(&storageSession, "session", false, "Show sessionStorage")
	storageCmd.Flags().BoolVar(&storageCookie, "cookie", false, "Show cookies")
	storageCmd.Flags().StringVar(&storageFilter, "filter", "", "Filter by key name")
	rootCmd.AddCommand(storageCmd)
}
