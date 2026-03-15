package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/tianna1121/navi/internal/cdp"
)

// JSON outputs data as JSON.
func JSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// Tabs prints tabs in human-readable format.
func Tabs(tabs []cdp.TabInfo) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "INDEX\tTITLE\tURL")
	for _, t := range tabs {
		title := t.Title
		if len(title) > 60 {
			title = title[:57] + "..."
		}
		url := t.URL
		if len(url) > 80 {
			url = url[:77] + "..."
		}
		fmt.Fprintf(w, "%d\t%s\t%s\n", t.Index, title, url)
	}
	w.Flush()
}

// ConsoleEntry prints a single console entry.
func ConsoleEntry(e cdp.ConsoleEntry) {
	levelColor := ""
	resetColor := "\033[0m"
	switch e.Level {
	case "error":
		levelColor = "\033[31m" // red
	case "warning", "warn":
		levelColor = "\033[33m" // yellow
	case "info":
		levelColor = "\033[36m" // cyan
	default:
		levelColor = ""
		resetColor = ""
	}

	fmt.Printf("%s %s[%s]%s %s\n", e.Timestamp, levelColor, strings.ToUpper(e.Level), resetColor, e.Text)
	if e.Source != "" {
		fmt.Printf("  └─ %s\n", e.Source)
	}
	if e.StackTrace != "" {
		fmt.Printf("%s\n", e.StackTrace)
	}
}

// NetworkEntry prints a single network entry.
func NetworkEntry(e cdp.NetworkEntry) {
	statusColor := ""
	resetColor := "\033[0m"
	if e.Status >= 400 {
		statusColor = "\033[31m" // red
	} else if e.Status >= 300 {
		statusColor = "\033[33m" // yellow
	} else if e.Status >= 200 {
		statusColor = "\033[32m" // green
	}

	if e.Error != "" {
		fmt.Printf("\033[31m[FAILED]\033[0m %s %s — %s\n", e.Method, e.URL, e.Error)
	} else {
		duration := ""
		if e.Duration > 0 {
			duration = fmt.Sprintf(" (%.0fms)", e.Duration)
		}
		fmt.Printf("%s%d%s %s %s%s\n", statusColor, e.Status, resetColor, e.Method, e.URL, duration)
	}

	if e.ResponseBody != "" {
		fmt.Printf("  Body: %s\n", truncate(e.ResponseBody, 2000))
	}

	if len(e.RequestHeaders) > 0 {
		fmt.Println("  Request Headers:")
		for k, v := range e.RequestHeaders {
			fmt.Printf("    %s: %s\n", k, v)
		}
	}
	if len(e.ResponseHeaders) > 0 {
		fmt.Println("  Response Headers:")
		for k, v := range e.ResponseHeaders {
			fmt.Printf("    %s: %s\n", k, v)
		}
	}
}

// Perf prints performance metrics.
func Perf(m *cdp.PerfMetrics) {
	fmt.Println("Page Performance Metrics")
	fmt.Println("───────────────────────")
	fmt.Printf("  DOMContentLoaded:     %.0f ms\n", m.DOMContentLoaded)
	fmt.Printf("  Load Complete:        %.0f ms\n", m.LoadComplete)
	fmt.Printf("  First Paint:          %.0f ms\n", m.FirstPaint)
	fmt.Printf("  First Contentful Paint: %.0f ms\n", m.FirstContentfulPaint)
	fmt.Printf("  DOM Nodes:            %d\n", m.DOMNodes)
	if m.JSHeapUsedSize > 0 {
		fmt.Printf("  JS Heap Used:         %.1f MB\n", m.JSHeapUsedSize/1024/1024)
		fmt.Printf("  JS Heap Total:        %.1f MB\n", m.JSHeapTotalSize/1024/1024)
	}
}

// Storage prints storage entries.
func Storage(entries interface{}) {
	switch v := entries.(type) {
	case []cdp.StorageEntry:
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tVALUE")
		for _, e := range v {
			val := truncate(e.Value, 100)
			fmt.Fprintf(w, "%s\t%s\n", e.Key, val)
		}
		w.Flush()
	case []cdp.CookieEntry:
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVALUE\tDOMAIN\tPATH\tSECURE\tHTTPONLY")
		for _, e := range v {
			val := truncate(e.Value, 40)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\t%v\n", e.Name, val, e.Domain, e.Path, e.Secure, e.HTTPOnly)
		}
		w.Flush()
	default:
		JSON(entries)
	}
}

// Value prints a generic value.
func Value(v interface{}) {
	switch val := v.(type) {
	case string:
		fmt.Println(val)
	case []string:
		for _, s := range val {
			fmt.Println(s)
		}
	case int:
		fmt.Println(val)
	default:
		JSON(v)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
