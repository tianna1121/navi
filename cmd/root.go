package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func defaultScreenshotDir() string {
	if dir := os.Getenv("NAVI_SCREENSHOT_DIR"); dir != "" {
		return dir
	}
	// Check for OpenClaw media dir
	home, _ := os.UserHomeDir()
	openclawMedia := filepath.Join(home, ".openclaw", "media")
	if info, err := os.Stat(openclawMedia); err == nil && info.IsDir() {
		return openclawMedia
	}
	return "/tmp"
}

var (
	cdpURL        string
	tabURL        string
	tabTitle      string
	tabIndex      int
	jsonOut       bool
	timeout       int
	verbose       bool
	screenshotDir string
)

var rootCmd = &cobra.Command{
	Use:   "navi",
	Short: "🧚 Navi — browser debugging CLI for AI agents",
	Long: `Navi is a lightweight CDP client CLI that lets AI agents directly
connect to Chrome for frontend debugging.

"Hey! Listen!" — your browser debugging fairy.`,
}

func init() {
	defaultURL := os.Getenv("NAVI_CDP_URL")
	if defaultURL == "" {
		defaultURL = "localhost:9222"
	}

	rootCmd.PersistentFlags().StringVarP(&cdpURL, "url", "u", defaultURL, "CDP remote debugging address (host:port)")
	rootCmd.PersistentFlags().StringVar(&tabURL, "tab-url", "", "Select tab by URL pattern (glob)")
	rootCmd.PersistentFlags().StringVar(&tabTitle, "tab-title", "", "Select tab by title")
	rootCmd.PersistentFlags().IntVarP(&tabIndex, "tab-index", "t", 0, "Select tab by index")
	rootCmd.PersistentFlags().BoolVarP(&jsonOut, "json", "j", false, "JSON output")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 10, "Command timeout in seconds")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose logging")
	rootCmd.PersistentFlags().StringVar(&screenshotDir, "screenshot-dir", defaultScreenshotDir(), "Default directory for screenshots")
}

func Execute() error {
	return rootCmd.Execute()
}
