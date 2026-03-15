package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tianna1121/navi/internal/cdp"
)

var (
	screenshotOutput   string
	screenshotSelector string
	screenshotFullpage bool
	screenshotViewport string
)

var screenshotCmd = &cobra.Command{
	Use:   "screenshot",
	Short: "Capture a page screenshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		client := cdp.NewClient(cdpURL)

		buf, err := client.Screenshot(ctx, tabURL, tabTitle, tabIndex, screenshotSelector, screenshotFullpage)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

		outPath := screenshotOutput
		if outPath == "" {
			outPath = fmt.Sprintf("%s/navi-screenshot-%d.png", screenshotDir, time.Now().Unix())
		}

		if err := os.WriteFile(outPath, buf, 0644); err != nil {
			return err
		}

		fmt.Printf("Screenshot saved: %s (%d bytes)\n", outPath, len(buf))
		return nil
	},
}

func init() {
	screenshotCmd.Flags().StringVarP(&screenshotOutput, "output", "o", "", "Output file path (default: /tmp/navi-screenshot-{timestamp}.png)")
	screenshotCmd.Flags().StringVar(&screenshotSelector, "selector", "", "CSS selector to screenshot")
	screenshotCmd.Flags().BoolVar(&screenshotFullpage, "fullpage", false, "Capture full page (including scroll)")
	screenshotCmd.Flags().StringVar(&screenshotViewport, "viewport", "", "Viewport size (e.g. 1920x1080)")
	rootCmd.AddCommand(screenshotCmd)
}
