package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tianna1121/navi/internal/cdp"
	"github.com/tianna1121/navi/internal/output"
)

var (
	fetchMethod    string
	fetchHeaders   []string
	fetchBody      string
	fetchTokenKey  string
	fetchNoAuth    bool
)

var fetchCmd = &cobra.Command{
	Use:   "fetch [url]",
	Short: "Fetch a URL in the page context with auto-auth",
	Long: `Execute a fetch() request in the browser page context.
Automatically reads the auth token from localStorage and adds it as
an Authorization header. Also auto-detects x-project-id from the JWT payload.

Examples:
  navi fetch /api/edge-nodes
  navi fetch /api/alerts?limit=10
  navi fetch /api/devices --method POST --body '{"name":"test"}'
  navi fetch /api/public --no-auth`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		client := cdp.NewClient(cdpURL)
		url := args[0]

		// Build the fetch JS
		js := buildFetchJS(url, fetchMethod, fetchHeaders, fetchBody, fetchTokenKey, fetchNoAuth)

		result, err := client.Eval(ctx, tabURL, tabTitle, tabIndex, js)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

		if jsonOut {
			// Try to parse as JSON for pretty printing
			if str, ok := result.(string); ok {
				var parsed interface{}
				if json.Unmarshal([]byte(str), &parsed) == nil {
					output.JSON(parsed)
					return nil
				}
			}
			output.JSON(result)
		} else {
			output.Value(result)
		}
		return nil
	},
}

func buildFetchJS(url, method string, extraHeaders []string, body string, tokenKey string, noAuth bool) string {
	if method == "" {
		method = "GET"
		if body != "" {
			method = "POST"
		}
	}

	// Escape for JS
	urlJS, _ := json.Marshal(url)
	methodJS, _ := json.Marshal(strings.ToUpper(method))
	tokenKeyJS, _ := json.Marshal(tokenKey)

	js := fmt.Sprintf(`(async function() {
  try {
    var headers = {};
    var method = %s;
    var tokenKey = %s;

`, methodJS, tokenKeyJS)

	if !noAuth {
		js += `    // Auto-auth: read token from localStorage
    var token = localStorage.getItem(tokenKey);
    if (!token) {
      // Try common token key names
      var keys = ['token', 'access_token', 'accessToken', 'auth_token', 'authToken', 'jwt'];
      for (var i = 0; i < keys.length; i++) {
        token = localStorage.getItem(keys[i]);
        if (token) break;
      }
    }
    if (token) {
      headers['Authorization'] = 'Bearer ' + token;

      // Auto-detect project ID from JWT
      try {
        var payload = JSON.parse(atob(token.split('.')[1]));
        if (payload.projectId) {
          headers['x-project-id'] = payload.projectId;
        }
        if (payload.tenantId) {
          headers['x-tenant-id'] = payload.tenantId;
        }
      } catch(e) {}
    }
`
	}

	// Add extra headers
	for _, h := range extraHeaders {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			k, _ := json.Marshal(strings.TrimSpace(parts[0]))
			v, _ := json.Marshal(strings.TrimSpace(parts[1]))
			js += fmt.Sprintf("    headers[%s] = %s;\n", k, v)
		}
	}

	js += fmt.Sprintf(`
    var opts = { method: method, headers: headers };
`)

	if body != "" {
		bodyJS, _ := json.Marshal(body)
		js += fmt.Sprintf(`    headers['Content-Type'] = headers['Content-Type'] || 'application/json';
    opts.body = %s;
`, bodyJS)
	}

	js += fmt.Sprintf(`
    var resp = await fetch(%s, opts);
    var text = await resp.text();

    // Try to parse as JSON
    try {
      var json = JSON.parse(text);
      return JSON.stringify({
        status: resp.status,
        statusText: resp.statusText,
        data: json
      }, null, 2);
    } catch(e) {
      return JSON.stringify({
        status: resp.status,
        statusText: resp.statusText,
        data: text.substring(0, 2000)
      }, null, 2);
    }
  } catch(e) {
    return JSON.stringify({ error: e.message });
  }
})()`, urlJS)

	return js
}

func init() {
	fetchCmd.Flags().StringVar(&fetchMethod, "method", "", "HTTP method (default: GET, or POST if --body is set)")
	fetchCmd.Flags().StringArrayVarP(&fetchHeaders, "header", "H", nil, "Extra headers (key: value)")
	fetchCmd.Flags().StringVar(&fetchBody, "body", "", "Request body (JSON)")
	fetchCmd.Flags().StringVar(&fetchTokenKey, "token-key", "token", "localStorage key for auth token")
	fetchCmd.Flags().BoolVar(&fetchNoAuth, "no-auth", false, "Skip auto-auth (don't add Authorization header)")
	rootCmd.AddCommand(fetchCmd)
}
