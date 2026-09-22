package dockerhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// apiPath builds a versioned path like /v1.44/containers/json?all=1
func (c *Client) apiPath(p string, query url.Values) string {
	ver := strings.TrimPrefix(c.ClientVersion(), "v")
	var apiPath string
	if ver != "" {
		apiPath = strings.TrimRight(c.host.BasePath, "/") + "/v" + ver + p
	} else {
		apiPath = strings.TrimRight(c.host.BasePath, "/") + p
	}
	u, _ := url.Parse(apiPath)
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

// unversionedPath builds a path without API version (for /_ping).
func (c *Client) unversionedPath(p string) string {
	return strings.TrimRight(c.host.BasePath, "/") + p
}

func (c *Client) ensureNegotiated(ctx context.Context) error {
	c.negotiateMu.Lock()
	need := c.negotiateVersion && !c.negotiated && !c.manualOverride
	c.negotiateMu.Unlock()
	if !need {
		return nil
	}
	info, err := c.pingRaw(ctx)
	if err != nil {
		return err
	}
	return c.setNegotiatedVersion(info.APIVersion)
}

func (c *Client) newRequest(ctx context.Context, method, rawPath string, body io.Reader, contentType string) (*http.Request, error) {
	u, err := url.Parse(rawPath)
	if err != nil {
		return nil, err
	}
	u.Scheme = c.scheme
	u.Host = c.reqHost

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	return req, nil
}

// do sends a request and returns the response. Caller must close Body on success.
func (c *Client) do(ctx context.Context, method, apiPath string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, apiPath, body, contentType)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// doJSON negotiates version, sends JSON body (optional), and decodes success JSON into out (optional).
func (c *Client) doJSON(ctx context.Context, method, p string, query url.Values, in any, out any) error {
	if err := c.ensureNegotiated(ctx); err != nil {
		return err
	}
	var body io.Reader
	var ct string
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
		ct = "application/json"
	}
	resp, err := c.do(ctx, method, c.apiPath(p, query), body, ct)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseErrorBody(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// doRaw negotiates, sends, returns body reader on 2xx (caller closes).
func (c *Client) doRaw(ctx context.Context, method, p string, query url.Values, body io.Reader, contentType string) (*http.Response, error) {
	if err := c.ensureNegotiated(ctx); err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, method, c.apiPath(p, query), body, contentType)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseErrorBody(resp)
	}
	return resp, nil
}

// discardAndClose drains and closes a response body.
func discardAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	_ = resp.Body.Close()
}
