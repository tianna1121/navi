package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// TabInfo represents a browser tab.
type TabInfo struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
	ID    string `json:"id"`
	WSURL string `json:"wsUrl"`
}

// ConsoleEntry represents a console log entry.
type ConsoleEntry struct {
	Timestamp  string `json:"timestamp"`
	Level      string `json:"level"`
	Text       string `json:"text"`
	Source     string `json:"source,omitempty"`
	StackTrace string `json:"stackTrace,omitempty"`
}

// NetworkEntry represents a network request/response.
type NetworkEntry struct {
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	Status          int64             `json:"status"`
	StatusText      string            `json:"statusText,omitempty"`
	Duration        float64           `json:"duration,omitempty"`
	Type            string            `json:"type,omitempty"`
	Error           string            `json:"error,omitempty"`
	RequestHeaders  map[string]string `json:"requestHeaders,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	ResponseBody    string            `json:"responseBody,omitempty"`
}

// PerfMetrics represents page performance metrics.
type PerfMetrics struct {
	DOMContentLoaded     float64 `json:"domContentLoaded"`
	LoadComplete         float64 `json:"loadComplete"`
	FirstPaint           float64 `json:"firstPaint"`
	FirstContentfulPaint float64 `json:"firstContentfulPaint"`
	DOMNodes             int     `json:"domNodes"`
	JSHeapUsedSize       float64 `json:"jsHeapUsedSize"`
	JSHeapTotalSize      float64 `json:"jsHeapTotalSize"`
}

// StorageEntry represents a storage key-value pair.
type StorageEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CookieEntry represents a cookie.
type CookieEntry struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  string `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httpOnly"`
	Secure   bool   `json:"secure"`
}

// Client is the CDP client.
type Client struct {
	cdpURL string
}

// NewClient creates a new CDP client.
func NewClient(cdpURL string) *Client {
	return &Client{cdpURL: cdpURL}
}

func (c *Client) httpHost() string {
	return strings.TrimPrefix(strings.TrimPrefix(c.cdpURL, "ws://"), "wss://")
}

// cdpConn wraps a raw WebSocket connection to a CDP target.
type cdpConn struct {
	conn    io.ReadWriteCloser
	nextID  atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan json.RawMessage
	events  chan cdpEvent
	done    chan struct{}
}

type cdpEvent struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type cdpMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

func newCDPConn(ctx context.Context, wsURL string) (*cdpConn, error) {
	conn, _, _, err := ws.Dial(ctx, wsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", wsURL, err)
	}

	c := &cdpConn{
		conn:    conn,
		pending: make(map[int64]chan json.RawMessage),
		events:  make(chan cdpEvent, 256),
		done:    make(chan struct{}),
	}

	go c.readLoop()
	return c, nil
}

func (c *cdpConn) readLoop() {
	defer close(c.done)
	for {
		data, err := wsutil.ReadServerText(c.conn)
		if err != nil {
			return
		}

		var msg cdpMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.ID > 0 {
			c.mu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.mu.Unlock()
			if ok {
				if msg.Error != nil {
					errJSON, _ := json.Marshal(msg.Error)
					ch <- errJSON
				} else {
					ch <- msg.Result
				}
			}
		} else if msg.Method != "" {
			select {
			case c.events <- cdpEvent{Method: msg.Method, Params: msg.Params}:
			default:
				// Drop if channel full
			}
		}
	}
}

func (c *cdpConn) call(method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan json.RawMessage, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := struct {
		ID     int64       `json:"id"`
		Method string      `json:"method"`
		Params interface{} `json:"params,omitempty"`
	}{ID: id, Method: method, Params: params}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if err := wsutil.WriteClientText(c.conn, data); err != nil {
		return nil, err
	}

	select {
	case result := <-ch:
		// Check if it's an error
		var cdpErr cdpError
		if json.Unmarshal(result, &cdpErr) == nil && cdpErr.Message != "" {
			return nil, fmt.Errorf("CDP error: %s", cdpErr.Message)
		}
		return result, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("timeout waiting for response to %s", method)
	}
}

func (c *cdpConn) close() {
	c.conn.Close()
}

// ListTabs returns all browser tabs.
func (c *Client) ListTabs(ctx context.Context) ([]TabInfo, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/json", c.httpHost()))
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Chrome at %s: %v\nMake sure Chrome is running with --remote-debugging-port=9222", c.cdpURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var targets []struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		Title string `json:"title"`
		URL   string `json:"url"`
		WSURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, err
	}

	var tabs []TabInfo
	idx := 0
	for _, t := range targets {
		if t.Type == "page" {
			tabs = append(tabs, TabInfo{
				Index: idx,
				Title: t.Title,
				URL:   t.URL,
				Type:  t.Type,
				ID:    t.ID,
				WSURL: t.WSURL,
			})
			idx++
		}
	}
	return tabs, nil
}

// resolveTab finds the tab based on selection criteria and returns its WS URL.
func (c *Client) resolveTab(ctx context.Context, tabURL, tabTitle string, tabIndex int) (*TabInfo, error) {
	tabs, err := c.ListTabs(ctx)
	if err != nil {
		return nil, err
	}
	if len(tabs) == 0 {
		return nil, fmt.Errorf("no browser tabs found")
	}

	if tabURL != "" {
		for _, t := range tabs {
			if matchGlob(t.URL, tabURL) {
				return &t, nil
			}
		}
		return nil, fmt.Errorf("no tab matching URL pattern %q", tabURL)
	}

	if tabTitle != "" {
		for _, t := range tabs {
			if strings.Contains(strings.ToLower(t.Title), strings.ToLower(tabTitle)) {
				return &t, nil
			}
		}
		return nil, fmt.Errorf("no tab matching title %q", tabTitle)
	}

	if tabIndex >= 0 && tabIndex < len(tabs) {
		return &tabs[tabIndex], nil
	}

	return &tabs[0], nil
}

// connectTab opens a raw CDP WebSocket to the specified tab.
func (c *Client) connectTab(ctx context.Context, tabURL, tabTitle string, tabIndex int) (*cdpConn, error) {
	tab, err := c.resolveTab(ctx, tabURL, tabTitle, tabIndex)
	if err != nil {
		return nil, err
	}
	return newCDPConn(ctx, tab.WSURL)
}

// Eval executes JavaScript in the page context.
func (c *Client) Eval(ctx context.Context, tabURL, tabTitle string, tabIndex int, expr string) (interface{}, error) {
	conn, err := c.connectTab(ctx, tabURL, tabTitle, tabIndex)
	if err != nil {
		return nil, err
	}
	defer conn.close()

	params := map[string]interface{}{
		"expression":    expr,
		"returnByValue": true,
		"awaitPromise":  true,
	}

	result, err := conn.call("Runtime.evaluate", params)
	if err != nil {
		return nil, err
	}

	var evalResult struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return nil, err
	}

	if evalResult.ExceptionDetails != nil {
		desc := evalResult.ExceptionDetails.Exception.Description
		if desc == "" {
			desc = evalResult.ExceptionDetails.Text
		}
		return nil, fmt.Errorf("exception: %s", desc)
	}

	var value interface{}
	if err := json.Unmarshal(evalResult.Result.Value, &value); err != nil {
		// Return raw string if not valid JSON
		return string(evalResult.Result.Value), nil
	}
	return value, nil
}

// Query queries DOM elements.
func (c *Client) Query(ctx context.Context, tabURL, tabTitle string, tabIndex int, selector string, getText bool, getAttr string, getHTML bool, getCount bool, getAll bool) (interface{}, error) {
	if getCount {
		return c.Eval(ctx, tabURL, tabTitle, tabIndex,
			fmt.Sprintf(`document.querySelectorAll(%q).length`, selector))
	}

	if getText {
		if getAll {
			return c.Eval(ctx, tabURL, tabTitle, tabIndex,
				fmt.Sprintf(`Array.from(document.querySelectorAll(%q)).map(e => e.innerText || e.textContent)`, selector))
		}
		return c.Eval(ctx, tabURL, tabTitle, tabIndex,
			fmt.Sprintf(`(function(){ var e = document.querySelector(%q); return e ? (e.innerText || e.textContent) : null; })()`, selector))
	}

	if getAttr != "" {
		if getAll {
			return c.Eval(ctx, tabURL, tabTitle, tabIndex,
				fmt.Sprintf(`Array.from(document.querySelectorAll(%q)).map(e => e.getAttribute(%q))`, selector, getAttr))
		}
		return c.Eval(ctx, tabURL, tabTitle, tabIndex,
			fmt.Sprintf(`(function(){ var e = document.querySelector(%q); return e ? e.getAttribute(%q) : null; })()`, selector, getAttr))
	}

	if getHTML {
		if getAll {
			return c.Eval(ctx, tabURL, tabTitle, tabIndex,
				fmt.Sprintf(`Array.from(document.querySelectorAll(%q)).map(e => e.outerHTML)`, selector))
		}
		return c.Eval(ctx, tabURL, tabTitle, tabIndex,
			fmt.Sprintf(`(function(){ var e = document.querySelector(%q); return e ? e.outerHTML : null; })()`, selector))
	}

	// Default: text
	return c.Eval(ctx, tabURL, tabTitle, tabIndex,
		fmt.Sprintf(`(function(){ var e = document.querySelector(%q); return e ? (e.innerText || e.textContent) : null; })()`, selector))
}

// GetPerf returns page performance metrics.
func (c *Client) GetPerf(ctx context.Context, tabURL, tabTitle string, tabIndex int) (*PerfMetrics, error) {
	result, err := c.Eval(ctx, tabURL, tabTitle, tabIndex, `(function(){
		var t = performance.timing;
		var navStart = t.navigationStart;
		var entries = performance.getEntriesByType('paint');
		var fp = 0, fcp = 0;
		entries.forEach(function(e) {
			if (e.name === 'first-paint') fp = e.startTime;
			if (e.name === 'first-contentful-paint') fcp = e.startTime;
		});
		return {
			domContentLoaded: t.domContentLoadedEventEnd - navStart,
			loadComplete: t.loadEventEnd - navStart,
			firstPaint: fp,
			firstContentfulPaint: fcp,
			domNodes: document.getElementsByTagName('*').length,
			jsHeapUsedSize: performance.memory ? performance.memory.usedJSHeapSize : 0,
			jsHeapTotalSize: performance.memory ? performance.memory.totalJSHeapSize : 0
		};
	})()`)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var metrics PerfMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return nil, err
	}
	return &metrics, nil
}

// GetStorage returns localStorage, sessionStorage, or cookies.
func (c *Client) GetStorage(ctx context.Context, tabURL, tabTitle string, tabIndex int, storageType string, filterKey string) (interface{}, error) {
	if storageType == "cookie" {
		conn, err := c.connectTab(ctx, tabURL, tabTitle, tabIndex)
		if err != nil {
			return nil, err
		}
		defer conn.close()

		result, err := conn.call("Network.getCookies", nil)
		if err != nil {
			return nil, err
		}

		var cookieResult struct {
			Cookies []struct {
				Name     string  `json:"name"`
				Value    string  `json:"value"`
				Domain   string  `json:"domain"`
				Path     string  `json:"path"`
				Expires  float64 `json:"expires"`
				HTTPOnly bool    `json:"httpOnly"`
				Secure   bool    `json:"secure"`
			} `json:"cookies"`
		}
		if err := json.Unmarshal(result, &cookieResult); err != nil {
			return nil, err
		}

		var entries []CookieEntry
		for _, ck := range cookieResult.Cookies {
			if filterKey != "" && !strings.Contains(strings.ToLower(ck.Name), strings.ToLower(filterKey)) {
				continue
			}
			entry := CookieEntry{
				Name:     ck.Name,
				Value:    ck.Value,
				Domain:   ck.Domain,
				Path:     ck.Path,
				HTTPOnly: ck.HTTPOnly,
				Secure:   ck.Secure,
			}
			if ck.Expires > 0 {
				entry.Expires = time.Unix(int64(ck.Expires), 0).Format(time.RFC3339)
			}
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		return entries, nil
	}

	// localStorage or sessionStorage
	storage := "localStorage"
	if storageType == "session" {
		storage = "sessionStorage"
	}

	js := fmt.Sprintf(`(function(){
		var s = %s;
		var result = [];
		for (var i = 0; i < s.length; i++) {
			var key = s.key(i);
			result.push({key: key, value: s.getItem(key)});
		}
		return result;
	})()`, storage)

	result, err := c.Eval(ctx, tabURL, tabTitle, tabIndex, js)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var entries []StorageEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	if filterKey != "" {
		var filtered []StorageEntry
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Key), strings.ToLower(filterKey)) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries, nil
}

// Screenshot captures a screenshot.
func (c *Client) Screenshot(ctx context.Context, tabURL, tabTitle string, tabIndex int, selector string, fullpage bool) ([]byte, error) {
	conn, err := c.connectTab(ctx, tabURL, tabTitle, tabIndex)
	if err != nil {
		return nil, err
	}
	defer conn.close()

	params := map[string]interface{}{
		"format":  "png",
		"quality": 90,
	}

	if fullpage {
		// Get full page dimensions
		layoutResult, err := conn.call("Page.getLayoutMetrics", nil)
		if err == nil {
			var layout struct {
				ContentSize struct {
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"contentSize"`
			}
			if json.Unmarshal(layoutResult, &layout) == nil {
				params["clip"] = map[string]interface{}{
					"x":      0,
					"y":      0,
					"width":  layout.ContentSize.Width,
					"height": layout.ContentSize.Height,
					"scale":  1,
				}
				params["captureBeyondViewport"] = true
			}
		}
	}

	if selector != "" {
		// Get element bounding box via JS
		js := fmt.Sprintf(`(function(){
			var el = document.querySelector(%q);
			if (!el) return null;
			var rect = el.getBoundingClientRect();
			return {x: rect.x, y: rect.y, width: rect.width, height: rect.height};
		})()`, selector)

		boxResult, err := c.Eval(ctx, tabURL, tabTitle, tabIndex, js)
		if err != nil {
			return nil, fmt.Errorf("selector %q: %v", selector, err)
		}
		if boxResult == nil {
			return nil, fmt.Errorf("element not found: %s", selector)
		}

		boxData, _ := json.Marshal(boxResult)
		var box struct {
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		}
		if err := json.Unmarshal(boxData, &box); err != nil {
			return nil, err
		}

		params["clip"] = map[string]interface{}{
			"x":      box.X,
			"y":      box.Y,
			"width":  box.Width,
			"height": box.Height,
			"scale":  1,
		}
	}

	result, err := conn.call("Page.captureScreenshot", params)
	if err != nil {
		return nil, err
	}

	var ssResult struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &ssResult); err != nil {
		return nil, err
	}

	return base64.StdEncoding.DecodeString(ssResult.Data)
}

// GetConsole collects console messages.
func (c *Client) GetConsole(ctx context.Context, tabURL, tabTitle string, tabIndex int, levels []string, limit int, follow bool, callback func(ConsoleEntry)) error {
	conn, err := c.connectTab(ctx, tabURL, tabTitle, tabIndex)
	if err != nil {
		return err
	}
	defer conn.close()

	levelSet := make(map[string]bool)
	for _, l := range levels {
		levelSet[strings.ToLower(l)] = true
	}

	// Enable Runtime to receive console events
	if _, err := conn.call("Runtime.enable", nil); err != nil {
		return err
	}

	count := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-conn.events:
			if !ok {
				return nil
			}
			switch ev.Method {
			case "Runtime.consoleAPICalled":
				var params struct {
					Type      string `json:"type"`
					Timestamp float64 `json:"timestamp"`
					Args      []struct {
						Type        string          `json:"type"`
						Value       json.RawMessage `json:"value"`
						Description string          `json:"description"`
					} `json:"args"`
					StackTrace *struct {
						CallFrames []struct {
							FunctionName string `json:"functionName"`
							URL          string `json:"url"`
							LineNumber   int    `json:"lineNumber"`
							ColumnNumber int    `json:"columnNumber"`
						} `json:"callFrames"`
					} `json:"stackTrace"`
				}
				if err := json.Unmarshal(ev.Params, &params); err != nil {
					continue
				}

				level := params.Type
				if len(levelSet) > 0 && !levelSet[level] {
					continue
				}

				var textParts []string
				for _, arg := range params.Args {
					val := string(arg.Value)
					if val == "" && arg.Description != "" {
						val = arg.Description
					}
					if val == "" {
						val = arg.Type
					}
					// Remove surrounding quotes
					if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
						var s string
						if json.Unmarshal([]byte(val), &s) == nil {
							val = s
						}
					}
					textParts = append(textParts, val)
				}

				ts := time.UnixMilli(int64(params.Timestamp)).Format("15:04:05.000")

				entry := ConsoleEntry{
					Timestamp: ts,
					Level:     level,
					Text:      strings.Join(textParts, " "),
				}

				if params.StackTrace != nil && len(params.StackTrace.CallFrames) > 0 {
					f := params.StackTrace.CallFrames[0]
					entry.Source = fmt.Sprintf("%s:%d:%d", f.URL, f.LineNumber, f.ColumnNumber)
				}

				callback(entry)
				count++
				if !follow && limit > 0 && count >= limit {
					return nil
				}

			case "Runtime.exceptionThrown":
				if len(levelSet) > 0 && !levelSet["error"] {
					continue
				}

				var params struct {
					Timestamp        float64 `json:"timestamp"`
					ExceptionDetails struct {
						Text string `json:"text"`
						Exception *struct {
							Description string `json:"description"`
							Value       json.RawMessage `json:"value"`
						} `json:"exception"`
						StackTrace *struct {
							CallFrames []struct {
								FunctionName string `json:"functionName"`
								URL          string `json:"url"`
								LineNumber   int    `json:"lineNumber"`
								ColumnNumber int    `json:"columnNumber"`
							} `json:"callFrames"`
						} `json:"stackTrace"`
					} `json:"exceptionDetails"`
				}
				if err := json.Unmarshal(ev.Params, &params); err != nil {
					continue
				}

				text := ""
				if params.ExceptionDetails.Exception != nil {
					text = params.ExceptionDetails.Exception.Description
					if text == "" {
						text = string(params.ExceptionDetails.Exception.Value)
					}
				}
				if text == "" {
					text = params.ExceptionDetails.Text
				}

				ts := time.UnixMilli(int64(params.Timestamp)).Format("15:04:05.000")
				entry := ConsoleEntry{
					Timestamp: ts,
					Level:     "error",
					Text:      text,
				}

				if params.ExceptionDetails.StackTrace != nil && len(params.ExceptionDetails.StackTrace.CallFrames) > 0 {
					f := params.ExceptionDetails.StackTrace.CallFrames[0]
					entry.Source = fmt.Sprintf("%s:%d:%d", f.URL, f.LineNumber, f.ColumnNumber)
					var stackLines []string
					for _, sf := range params.ExceptionDetails.StackTrace.CallFrames {
						stackLines = append(stackLines, fmt.Sprintf("  at %s (%s:%d:%d)", sf.FunctionName, sf.URL, sf.LineNumber, sf.ColumnNumber))
					}
					entry.StackTrace = strings.Join(stackLines, "\n")
				}

				callback(entry)
				count++
				if !follow && limit > 0 && count >= limit {
					return nil
				}
			}

		case <-conn.done:
			return nil
		}
	}
}

// GetNetwork collects network requests.
func (c *Client) GetNetwork(ctx context.Context, tabURL, tabTitle string, tabIndex int, failed bool, filter string, method string, showHeaders bool, showBody bool, last bool, limit int, follow bool, callback func(NetworkEntry)) error {
	conn, err := c.connectTab(ctx, tabURL, tabTitle, tabIndex)
	if err != nil {
		return err
	}
	defer conn.close()

	type requestInfo struct {
		URL     string
		Method  string
		Headers map[string]string
		Type    string
	}
	requests := make(map[string]*requestInfo)
	var entries []NetworkEntry
	var mu sync.Mutex

	// Enable Network
	if _, err := conn.call("Network.enable", nil); err != nil {
		return err
	}

	count := 0
	for {
		select {
		case <-ctx.Done():
			goto done
		case ev, ok := <-conn.events:
			if !ok {
				goto done
			}
			switch ev.Method {
			case "Network.requestWillBeSent":
				var params struct {
					RequestID string `json:"requestId"`
					Request   struct {
						URL     string                 `json:"url"`
						Method  string                 `json:"method"`
						Headers map[string]interface{} `json:"headers"`
					} `json:"request"`
					Type string `json:"type"`
				}
				if json.Unmarshal(ev.Params, &params) != nil {
					continue
				}
				headers := make(map[string]string)
				for k, v := range params.Request.Headers {
					headers[k] = fmt.Sprintf("%v", v)
				}
				mu.Lock()
				requests[params.RequestID] = &requestInfo{
					URL:     params.Request.URL,
					Method:  params.Request.Method,
					Headers: headers,
					Type:    params.Type,
				}
				mu.Unlock()

			case "Network.responseReceived":
				var params struct {
					RequestID string `json:"requestId"`
					Response  struct {
						Status     int64                  `json:"status"`
						StatusText string                 `json:"statusText"`
						Headers    map[string]interface{} `json:"headers"`
						Timing     *struct {
							ReceiveHeadersEnd float64 `json:"receiveHeadersEnd"`
						} `json:"timing"`
					} `json:"response"`
				}
				if json.Unmarshal(ev.Params, &params) != nil {
					continue
				}

				mu.Lock()
				req, ok := requests[params.RequestID]
				mu.Unlock()
				if !ok {
					continue
				}

				status := params.Response.Status
				if failed && status >= 200 && status < 400 {
					continue
				}
				if filter != "" && !strings.Contains(req.URL, filter) {
					continue
				}
				if method != "" && !strings.EqualFold(req.Method, method) {
					continue
				}

				respHeaders := make(map[string]string)
				for k, v := range params.Response.Headers {
					respHeaders[k] = fmt.Sprintf("%v", v)
				}

				duration := 0.0
				if params.Response.Timing != nil {
					duration = params.Response.Timing.ReceiveHeadersEnd
				}

				entry := NetworkEntry{
					URL:        req.URL,
					Method:     req.Method,
					Status:     status,
					StatusText: params.Response.StatusText,
					Duration:   duration,
					Type:       req.Type,
				}
				if showHeaders {
					entry.RequestHeaders = req.Headers
					entry.ResponseHeaders = respHeaders
				}

				if showBody {
					// Try to get response body
					bodyResult, err := conn.call("Network.getResponseBody", map[string]interface{}{
						"requestId": params.RequestID,
					})
					if err == nil {
						var bodyResp struct {
							Body           string `json:"body"`
							Base64Encoded  bool   `json:"base64Encoded"`
						}
						if json.Unmarshal(bodyResult, &bodyResp) == nil {
							entry.ResponseBody = bodyResp.Body
						}
					}
				}

				mu.Lock()
				entries = append(entries, entry)
				mu.Unlock()

				if !last {
					callback(entry)
					count++
					if !follow && limit > 0 && count >= limit {
						goto done
					}
				}

			case "Network.loadingFailed":
				var params struct {
					RequestID string `json:"requestId"`
					ErrorText string `json:"errorText"`
				}
				if json.Unmarshal(ev.Params, &params) != nil {
					continue
				}

				mu.Lock()
				req, ok := requests[params.RequestID]
				mu.Unlock()
				if !ok {
					continue
				}

				if filter != "" && !strings.Contains(req.URL, filter) {
					continue
				}

				entry := NetworkEntry{
					URL:    req.URL,
					Method: req.Method,
					Error:  params.ErrorText,
					Type:   req.Type,
				}

				mu.Lock()
				entries = append(entries, entry)
				mu.Unlock()

				if !last {
					callback(entry)
					count++
					if !follow && limit > 0 && count >= limit {
						goto done
					}
				}
			}
		case <-conn.done:
			goto done
		}
	}

done:
	if last && len(entries) > 0 {
		callback(entries[len(entries)-1])
	}
	return nil
}

// matchGlob does simple glob matching with * wildcards.
func matchGlob(s, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}

	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return strings.Contains(s, pattern)
	}

	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(s[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}

	if !strings.HasSuffix(pattern, "*") {
		return strings.HasSuffix(s, parts[len(parts)-1])
	}
	return true
}

// Verbose controls debug output
var Verbose bool

func debugf(format string, args ...interface{}) {
	if Verbose {
		fmt.Fprintf(os.Stderr, "[debug] "+format+"\n", args...)
	}
}
