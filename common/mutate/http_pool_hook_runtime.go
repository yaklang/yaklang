package mutate

type HTTPPoolRequestHookRuntimeMeta struct {
	IsHTTPS       bool
	OriginRequest []byte
	Source        string
}

type HTTPPoolRequestHookRuntime interface {
	BeforeRequest(req []byte) []byte
	AfterRequest(req []byte, rsp []byte) []byte
	MirrorHTTPFlow(req []byte, rsp []byte, existed map[string]string) map[string]string
	RetryHandler(retryCount int, req []byte, rsp []byte, retry func(...[]byte))
	CustomFailureChecker(req []byte, rsp []byte, fail func(string))
	MockHTTPRequest(url string, req []byte, mockResponse func(interface{}))
}

type HTTPPoolRequestHookRuntimeFactory func(meta *HTTPPoolRequestHookRuntimeMeta) HTTPPoolRequestHookRuntime

type httpPoolRequestHookHandlers struct {
	https   bool
	config  *httpPoolConfig
	runtime HTTPPoolRequestHookRuntime
}

func newHTTPPoolRequestHookHandlers(config *httpPoolConfig, https bool, originRequest []byte) *httpPoolRequestHookHandlers {
	if config == nil {
		return nil
	}
	handlers := &httpPoolRequestHookHandlers{
		https:  https,
		config: config,
	}
	if config.HookRuntimeFactory != nil {
		handlers.runtime = config.HookRuntimeFactory(&HTTPPoolRequestHookRuntimeMeta{
			IsHTTPS:       https,
			OriginRequest: append([]byte(nil), originRequest...),
			Source:        config.Source,
		})
	}
	if handlers.runtime == nil &&
		config.HookBeforeRequest == nil &&
		config.HookAfterRequest == nil &&
		config.MirrorHTTPFlow == nil &&
		config.RetryHandler == nil &&
		config.CustomFailureChecker == nil &&
		config.MockHTTPRequest == nil {
		return nil
	}
	return handlers
}

func (h *httpPoolRequestHookHandlers) BeforeRequest(req []byte) []byte {
	if h == nil {
		return req
	}
	if h.runtime != nil {
		if out := h.runtime.BeforeRequest(req); len(out) > 0 {
			return out
		}
		return req
	}
	if h.config.HookBeforeRequest == nil {
		return req
	}
	if out := h.config.HookBeforeRequest(h.https, nil, req); len(out) > 0 {
		return out
	}
	return req
}

func (h *httpPoolRequestHookHandlers) AfterRequest(req []byte, rsp []byte) []byte {
	if h == nil {
		return rsp
	}
	if h.runtime != nil {
		if out := h.runtime.AfterRequest(req, rsp); len(out) > 0 {
			return out
		}
		return rsp
	}
	if h.config.HookAfterRequest == nil {
		return rsp
	}
	if out := h.config.HookAfterRequest(h.https, nil, req, nil, rsp); len(out) > 0 {
		return out
	}
	return rsp
}

func (h *httpPoolRequestHookHandlers) MirrorHTTPFlow(req []byte, rsp []byte, existed map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	if h.runtime != nil {
		return h.runtime.MirrorHTTPFlow(req, rsp, existed)
	}
	if h.config.MirrorHTTPFlow == nil {
		return nil
	}
	return h.config.MirrorHTTPFlow(req, rsp, existed)
}

func (h *httpPoolRequestHookHandlers) RetryHandler(retryCount int, req []byte, rsp []byte, retry func(...[]byte)) {
	if h == nil {
		return
	}
	if h.runtime != nil {
		h.runtime.RetryHandler(retryCount, req, rsp, retry)
		return
	}
	if h.config.RetryHandler != nil {
		h.config.RetryHandler(h.https, retryCount, req, rsp, retry)
	}
}

func (h *httpPoolRequestHookHandlers) CustomFailureChecker(req []byte, rsp []byte, fail func(string)) {
	if h == nil {
		return
	}
	if h.runtime != nil {
		h.runtime.CustomFailureChecker(req, rsp, fail)
		return
	}
	if h.config.CustomFailureChecker != nil {
		h.config.CustomFailureChecker(h.https, req, rsp, fail)
	}
}

func (h *httpPoolRequestHookHandlers) MockHTTPRequest(url string, req []byte, mockResponse func(interface{})) {
	if h == nil {
		return
	}
	if h.runtime != nil {
		h.runtime.MockHTTPRequest(url, req, mockResponse)
		return
	}
	if h.config.MockHTTPRequest != nil {
		h.config.MockHTTPRequest(h.https, url, req, mockResponse)
	}
}
