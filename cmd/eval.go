package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tianna1121/navi/internal/cdp"
	"github.com/tianna1121/navi/internal/output"
)

var evalCmd = &cobra.Command{
	Use:   "eval [expression]",
	Short: "Execute JavaScript in the page context",
	Long:  `Execute a JavaScript expression and return the result. Use '-' to read from stdin.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		expr := args[0]
		if expr == "-" {
			scanner := bufio.NewScanner(os.Stdin)
			var lines []string
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			expr = strings.Join(lines, "\n")
		}

		client := cdp.NewClient(cdpURL)

		result, err := client.Eval(ctx, tabURL, tabTitle, tabIndex, expr)
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
	rootCmd.AddCommand(evalCmd)
}
