package browser

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/ysmood/gson"
)

const networkTapBodyLimit = 2048

type pendingNetworkReq struct {
	url     string
	method  string
	body    string
	headers map[string]string
	status  int
}

type networkTap struct {
	mu      sync.Mutex
	pending map[proto.NetworkRequestID]*pendingNetworkReq
	done    []map[string]any
}

// EvalOnNewDocument injects JavaScript that runs before every new document.
func (p *BrowserPage) EvalOnNewDocument(js string) error {
	if p == nil {
		return fmt.Errorf("eval on new document: empty page")
	}
	sess := p.rootRod()
	if sess == nil {
		return fmt.Errorf("eval on new document: empty page")
	}
	_, err := sess.EvalOnNewDocument(js)
	if err != nil {
		return err
	}
	return nil
}

// StartNetworkTap enables CDP Network events and records finished HTTP exchanges.
// Call this before Navigate so load-time XHR/fetch is captured.
func (p *BrowserPage) StartNetworkTap() error {
	if p == nil {
		return fmt.Errorf("start network tap: empty page")
	}
	p.tapMu.Lock()
	defer p.tapMu.Unlock()
	if p.tap != nil {
		return nil
	}
	sess := p.rootRod()
	if sess == nil {
		return fmt.Errorf("start network tap: empty page")
	}
	tap := &networkTap{
		pending: map[proto.NetworkRequestID]*pendingNetworkReq{},
	}

	maxPost := 8192
	maxRes := 1024 * 1024
	maxTot := 10 * 1024 * 1024
	err := proto.NetworkEnable{
		MaxPostDataSize:       &maxPost,
		MaxResourceBufferSize: &maxRes,
		MaxTotalBufferSize:    &maxTot,
	}.Call(sess)
	if err != nil {
		return err
	}

	page := sess
	wait := page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			tap.onRequest(e)
		},
		func(e *proto.NetworkResponseReceived) {
			tap.onResponse(e)
		},
		func(e *proto.NetworkLoadingFinished) {
			go tap.onFinished(page, e.RequestID)
		},
		func(e *proto.NetworkLoadingFailed) {
			tap.onFailed(e)
		},
	)
	p.tap = tap
	go wait()
	return nil
}

// DrainNetworkTap returns captured exchanges since the last drain and clears them.
func (p *BrowserPage) DrainNetworkTap() []map[string]any {
	if p == nil {
		return []map[string]any{}
	}
	p.tapMu.Lock()
	tap := p.tap
	p.tapMu.Unlock()
	if tap == nil {
		return []map[string]any{}
	}
	return tap.drain()
}

func (t *networkTap) onRequest(e *proto.NetworkRequestWillBeSent) {
	if e == nil || e.Request == nil {
		return
	}
	if skipNetworkURL(e.Request.URL) || skipNetworkResource(e.Type) {
		return
	}
	headers := networkHeadersToMap(e.Request.Headers)
	body := e.Request.PostData
	if body == "" && len(e.Request.PostDataEntries) > 0 {
		var b strings.Builder
		for _, ent := range e.Request.PostDataEntries {
			if ent != nil && len(ent.Bytes) > 0 {
				b.Write(ent.Bytes)
			}
		}
		body = b.String()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending == nil {
		t.pending = make(map[proto.NetworkRequestID]*pendingNetworkReq)
	}
	if prev, ok := t.pending[e.RequestID]; ok && prev != nil {
		status := prev.status
		if e.RedirectResponse != nil {
			status = e.RedirectResponse.Status
		}
		prev.status = status
		t.done = append(t.done, networkTapRecord(prev, ""))
	}
	t.pending[e.RequestID] = &pendingNetworkReq{
		url:     e.Request.URL,
		method:  e.Request.Method,
		body:    clipNetworkBody(body),
		headers: headers,
	}
}

func (t *networkTap) onResponse(e *proto.NetworkResponseReceived) {
	if e == nil || e.Response == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	pending, ok := t.pending[e.RequestID]
	if !ok || pending == nil {
		return
	}
	pending.status = e.Response.Status
}

func (t *networkTap) onFailed(e *proto.NetworkLoadingFailed) {
	if e == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	pending, ok := t.pending[e.RequestID]
	if !ok || pending == nil {
		return
	}
	delete(t.pending, e.RequestID)
	t.done = append(t.done, networkTapRecord(pending, ""))
}

func (t *networkTap) onFinished(page *rod.Page, id proto.NetworkRequestID) {
	t.mu.Lock()
	pending, ok := t.pending[id]
	if !ok || pending == nil {
		delete(t.pending, id)
		t.mu.Unlock()
		return
	}
	delete(t.pending, id)
	rec := networkTapRecord(pending, "")
	t.mu.Unlock()

	respBody := ""
	if page != nil {
		res, err := proto.NetworkGetResponseBody{RequestID: id}.Call(page)
		if err != nil {
			log.Debugf("network tap get body: %v", err)
		} else if res != nil {
			respBody = res.Body
			if res.Base64Encoded && respBody != "" {
				if decoded, decErr := base64.StdEncoding.DecodeString(respBody); decErr == nil {
					respBody = string(decoded)
				}
			}
		}
	}
	rec["response"] = clipNetworkBody(respBody)

	t.mu.Lock()
	t.done = append(t.done, rec)
	t.mu.Unlock()
}

func (t *networkTap) drain() []map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.done
	t.done = nil
	if out == nil {
		return []map[string]any{}
	}
	return out
}

func networkTapRecord(pending *pendingNetworkReq, response string) map[string]any {
	return map[string]any{
		"url":      pending.url,
		"method":   pending.method,
		"body":     pending.body,
		"status":   pending.status,
		"headers":  pending.headers,
		"response": response,
	}
}

func clipNetworkBody(s string) string {
	if len(s) <= networkTapBodyLimit {
		return s
	}
	end := networkTapBodyLimit
	for end > 0 && end < len(s) && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end]
}

func skipNetworkURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "" {
		return true
	}
	return strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "blob:") ||
		strings.HasPrefix(lower, "about:") ||
		strings.HasPrefix(lower, "chrome-extension:") ||
		strings.HasPrefix(lower, "chrome:")
}

func skipNetworkResource(t proto.NetworkResourceType) bool {
	switch t {
	case proto.NetworkResourceTypeImage,
		proto.NetworkResourceTypeFont,
		proto.NetworkResourceTypeMedia,
		proto.NetworkResourceTypeStylesheet,
		proto.NetworkResourceTypeManifest,
		proto.NetworkResourceTypePing,
		proto.NetworkResourceTypePrefetch,
		proto.NetworkResourceTypeScript:
		return true
	default:
		return false
	}
}

func networkHeadersToMap(h proto.NetworkHeaders) map[string]string {
	out := map[string]string{}
	if h == nil {
		return out
	}
	for k, v := range h {
		s := headerJSONString(v)
		if s == "" {
			continue
		}
		out[k] = s
	}
	return out
}

func headerJSONString(v gson.JSON) string {
	return strings.TrimSpace(v.Str())
}
