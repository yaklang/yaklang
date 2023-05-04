package suricata

type HttpBaseStickyRule struct {
	FileData          bool
	HttpContentType   bool
	HttpContentLength bool
	HttpStart         bool
	HttpProtocol      bool
	HttpHeaderNames   bool
}
type HttpBaseModifierRule struct {
	HttpHeader    bool
	HttpRawHeader bool
	HttpCookie    bool
}

func (h *HttpBaseModifierRule) HaveBeenSet() bool {
	if h == nil {
		return false
	}
	return h.HttpHeader || h.HttpRawHeader || h.HttpCookie
}

type HttpRequestStickyRule struct {
	HttpRequestLine bool
	HttpAccept      bool
	HttpAcceptLang  bool
	HttpAcceptEnc   bool
	HttpReferer     bool
	HttpConnection  bool
}

type HttpRequestModifierRule struct {
	HttpUri       bool
	HttpRawUri    bool
	HttpMethod    bool
	HttpUserAgent bool
	HttpHost      bool
	HttpRawHost   bool
}

func (h *HttpRequestModifierRule) HaveBeenSet() bool {
	if h == nil {
		return false
	}
	return h.HttpUri || h.HttpRawUri || h.HttpMethod || h.HttpUserAgent || h.HttpHost || h.HttpRawHost
}

type HttpResponseStickyRule struct {
	HttpResponseLine bool
}
type HttpResponseModifierRule struct {
	HttpStatMsg    bool
	HttpStatCode   bool
	HttpServerBody bool
	HttpServer     bool
	HttpLocation   bool
}

func (h *HttpResponseModifierRule) HaveBeenSet() bool {
	if h == nil {
		return false
	}
	return h.HttpStatMsg || h.HttpStatCode || h.HttpServerBody || h.HttpServer || h.HttpLocation
}

func (h *HttpBaseStickyRule) HaveBeenSet() bool {
	if h == nil {
		return false
	}
	return h.HttpContentType || h.HttpProtocol || h.HttpContentLength || h.HttpHeaderNames || h.FileData || h.HttpStart
}

func (h *HttpRequestStickyRule) HaveBeenSet() bool {
	if h == nil {
		return false
	}
	return h.HttpRequestLine || h.HttpAccept || h.HttpAcceptLang || h.HttpAcceptEnc || h.HttpReferer || h.HttpConnection
}

func (h *HttpResponseStickyRule) HaveBeenSet() bool {
	if h == nil {
		return false
	}
	return h.HttpResponseLine
}
