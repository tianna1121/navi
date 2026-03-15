package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tianna1121/navi/internal/cdp"
	"github.com/tianna1121/navi/internal/output"
)

var (
	consoleErrors bool
	consoleLevel  string
	consoleLimit  int
	consoleFollow bool
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Get browser console output",
	Long:  `Capture console.log, console.error, console.warn and uncaught exceptions from the browser.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var ctx context.Context
		var cancel context.CancelFunc

		if consoleFollow {
			ctx, cancel = context.WithCancel(context.Background())
		} else {
			ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		}
		defer cancel()

		client := cdp.NewClient(cdpURL)

		var levels []string
		if consoleErrors {
			levels = []string{"error"}
		} else if consoleLevel != "" {
			levels = strings.Split(consoleLevel, ",")
		}

		err := client.GetConsole(ctx, tabURL, tabTitle, tabIndex, levels, consoleLimit, consoleFollow, func(entry cdp.ConsoleEntry) {
			if jsonOut {
				output.JSON(entry)
			} else {
				output.ConsoleEntry(entry)
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
	consoleCmd.Flags().BoolVar(&consoleErrors, "errors", false, "Only show errors")
	consoleCmd.Flags().StringVar(&consoleLevel, "level", "", "Filter by level (comma-separated: error,warn,log,info,debug)")
	consoleCmd.Flags().IntVar(&consoleLimit, "limit", 50, "Maximum number of entries")
	consoleCmd.Flags().BoolVar(&consoleFollow, "follow", false, "Continuously stream new console output")
	rootCmd.AddCommand(consoleCmd)
}
