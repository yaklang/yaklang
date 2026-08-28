package browser

import (
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/ysmood/gson"
)

const networkTapBodyLimit = 2048

type pendingNetworkReq struct {
	url          string
	method       string
	body         string
	headers      map[string]string
	status       int
	resourceType proto.NetworkResourceType
}

type networkTap struct {
	mu      sync.Mutex
	pending map[proto.NetworkRequestID]*pendingNetworkReq
	done    []map[string]any
	started bool
}

// EvalOnNewDocument injects JavaScript that runs before every new document.
func (p *BrowserPage) EvalOnNewDocument(js string) error {
	sess := p.rootRod()
	if sess == nil {
		sess = p.page
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
	p.tapMu.Lock()
	if p.tap != nil && p.tap.started {
		p.tapMu.Unlock()
		return nil
	}
	tap := &networkTap{
		pending: map[proto.NetworkRequestID]*pendingNetworkReq{},
	}
	p.tap = tap
	p.tapMu.Unlock()

	maxPost := 8192
	maxRes := 1024 * 1024
	maxTot := 10 * 1024 * 1024
	sess := p.rootRod()
	if sess == nil {
		sess = p.page
	}
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
	)
	go wait()
	time.Sleep(100 * time.Millisecond)
	tap.mu.Lock()
	tap.started = true
	tap.mu.Unlock()
	return nil
}

// DrainNetworkTap returns captured exchanges since the last drain and clears them.
func (p *BrowserPage) DrainNetworkTap() []map[string]any {
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
	if prev, ok := t.pending[e.RequestID]; ok && prev != nil {
		status := prev.status
		if e.RedirectResponse != nil {
			status = e.RedirectResponse.Status
		}
		t.done = append(t.done, map[string]any{
			"url":      prev.url,
			"method":   prev.method,
			"body":     prev.body,
			"status":   status,
			"headers":  prev.headers,
			"response": "",
		})
	}
	t.pending[e.RequestID] = &pendingNetworkReq{
		url:          e.Request.URL,
		method:       e.Request.Method,
		body:         clipNetworkBody(body),
		headers:      headers,
		resourceType: e.Type,
	}
}

func (t *networkTap) onResponse(e *proto.NetworkResponseReceived) {
	if e == nil || e.Response == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	pending, ok := t.pending[e.RequestID]
	if !ok {
		return
	}
	pending.status = e.Response.Status
}

func (t *networkTap) onFinished(page *rod.Page, id proto.NetworkRequestID) {
	t.mu.Lock()
	pending, ok := t.pending[id]
	if !ok {
		t.mu.Unlock()
		return
	}
	delete(t.pending, id)
	rec := map[string]any{
		"url":      pending.url,
		"method":   pending.method,
		"body":     pending.body,
		"status":   pending.status,
		"headers":  pending.headers,
		"response": "",
	}
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
	for id, pending := range t.pending {
		if pending == nil {
			continue
		}
		out = append(out, map[string]any{
			"url":      pending.url,
			"method":   pending.method,
			"body":     pending.body,
			"status":   pending.status,
			"headers":  pending.headers,
			"response": "",
		})
		delete(t.pending, id)
	}
	if out == nil {
		return []map[string]any{}
	}
	return out
}

func clipNetworkBody(s string) string {
	if len(s) <= networkTapBodyLimit {
		return s
	}
	return s[:networkTapBodyLimit]
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
