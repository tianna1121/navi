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
	queryText  bool
	queryAttr  string
	queryHTML  bool
	queryCount bool
	queryAll   bool
)

var queryCmd = &cobra.Command{
	Use:   "query [selector]",
	Short: "Query DOM elements",
	Long:  `Query DOM elements by CSS selector and extract text, attributes, or HTML.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		client := cdp.NewClient(cdpURL)

		// Default to --text if no option specified
		if !queryText && queryAttr == "" && !queryHTML && !queryCount {
			queryText = true
		}

		result, err := client.Query(ctx, tabURL, tabTitle, tabIndex, args[0], queryText, queryAttr, queryHTML, queryCount, queryAll)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

		if jsonOut {
			output.JSON(result)
		} else {
			output.Value(result)
		}
		return nil
	},
}

func init() {
	queryCmd.Flags().BoolVar(&queryText, "text", false, "Get element text content")
	queryCmd.Flags().StringVar(&queryAttr, "attr", "", "Get element attribute value")
	queryCmd.Flags().BoolVar(&queryHTML, "html", false, "Get element outer HTML")
	queryCmd.Flags().BoolVar(&queryCount, "count", false, "Get number of matching elements")
	queryCmd.Flags().BoolVar(&queryAll, "all", false, "Get all matching elements (not just first)")
	rootCmd.AddCommand(queryCmd)
}
