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

var tabsCmd = &cobra.Command{
	Use:   "tabs",
	Short: "List browser tabs",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		client := cdp.NewClient(cdpURL)
		tabs, err := client.ListTabs(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

		if jsonOut {
			output.JSON(tabs)
		} else {
			output.Tabs(tabs)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tabsCmd)
}
