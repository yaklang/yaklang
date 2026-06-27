package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// liveHTTPInvoker executes real HTTP against remote targets for live tests.
type liveHTTPInvoker struct {
	*mock.MockInvoker
	client *http.Client
}

func newLiveHTTPInvoker() *liveHTTPInvoker {
	return &liveHTTPInvoker{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (l *liveHTTPInvoker) ExecuteToolRequiredAndCallWithoutRequired(_ context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
	if toolName != "do_http_request" {
		return l.MockInvoker.ExecuteToolRequiredAndCallWithoutRequired(context.Background(), toolName, params)
	}
	norm := make(aitool.InvokeParams, len(params)+4)
	for k, v := range params {
		norm[k] = v
	}
	norm, _ = augmentDoHTTPParams(norm)

	rawURL := strings.TrimSpace(fmt.Sprint(norm["url"]))
	if rawURL == "" {
		return nil, false, fmt.Errorf("empty url")
	}
	method := strings.ToUpper(strings.TrimSpace(fmt.Sprint(norm["method"])))
	if method == "" {
		method = http.MethodGet
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, false, err
	}
	if q := strings.TrimSpace(fmt.Sprint(norm["query-params"])); q != "" {
		qv, _ := url.ParseQuery(q)
		existing := u.Query()
		for k, vals := range qv {
			for _, v := range vals {
				existing.Add(k, v)
			}
		}
		u.RawQuery = existing.Encode()
	}

	var bodyReader io.Reader
	ct := strings.TrimSpace(fmt.Sprint(norm["content-type"]))
	postParams := strings.TrimSpace(fmt.Sprint(norm["post-params"]))
	body := strings.TrimSpace(fmt.Sprint(norm["body"]))
	if postParams != "" {
		bodyReader = strings.NewReader(postParams)
	} else if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, u.String(), bodyReader)
	if err != nil {
		return nil, false, err
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if hdr := strings.TrimSpace(fmt.Sprint(norm["headers"])); hdr != "" {
		for _, line := range strings.Split(strings.ReplaceAll(hdr, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var b strings.Builder
	fmt.Fprintf(&b, "HTTP/1.1 %d %s\r\n", resp.StatusCode, http.StatusText(resp.StatusCode))
	for k, vals := range resp.Header {
		for _, v := range vals {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	b.WriteString("\r\n")
	b.Write(respBody)
	return &aitool.ToolResult{
		Success: true,
		Data:    &aitool.ToolExecutionResult{Stdout: b.String()},
	}, false, nil
}
