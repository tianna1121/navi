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

var perfCmd = &cobra.Command{
	Use:   "perf",
	Short: "Get page performance metrics",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		client := cdp.NewClient(cdpURL)

		metrics, err := client.GetPerf(ctx, tabURL, tabTitle, tabIndex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

		if jsonOut {
			output.JSON(metrics)
		} else {
			output.Perf(metrics)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(perfCmd)
}
