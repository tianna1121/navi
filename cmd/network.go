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
	networkFailed  bool
	networkFilter  string
	networkMethod  string
	networkHeaders bool
	networkBody    bool
	networkLast    bool
	networkLimit   int
	networkFollow  bool
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Get network requests and responses",
	Long:  `Capture HTTP requests and responses including status codes, headers, and response bodies.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var ctx context.Context
		var cancel context.CancelFunc

		if networkFollow {
			ctx, cancel = context.WithCancel(context.Background())
		} else {
			ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		}
		defer cancel()

		client := cdp.NewClient(cdpURL)

		err := client.GetNetwork(ctx, tabURL, tabTitle, tabIndex, networkFailed, networkFilter, networkMethod, networkHeaders, networkBody, networkLast, networkLimit, networkFollow, func(entry cdp.NetworkEntry) {
			if jsonOut {
				output.JSON(entry)
			} else {
				output.NetworkEntry(entry)
			}
		})
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}
		return nil
	},
}

func init() {
	networkCmd.Flags().BoolVar(&networkFailed, "failed", false, "Only show failed requests (non-2xx)")
	networkCmd.Flags().StringVar(&networkFilter, "filter", "", "Filter by URL substring")
	networkCmd.Flags().StringVar(&networkMethod, "method", "", "Filter by HTTP method")
	networkCmd.Flags().BoolVar(&networkHeaders, "headers", false, "Show request/response headers")
	networkCmd.Flags().BoolVar(&networkBody, "body", false, "Show response body (with --last)")
	networkCmd.Flags().BoolVar(&networkLast, "last", false, "Only show the last matching request")
	networkCmd.Flags().IntVar(&networkLimit, "limit", 50, "Maximum number of entries")
	networkCmd.Flags().BoolVar(&networkFollow, "follow", false, "Continuously stream new requests")
	rootCmd.AddCommand(networkCmd)
}
