package httpctx

import (
	"context"
	"net/http"
	"strings"
)

const BrowserInstanceContextKey = "BrowserInstance"

type browserInstanceContextKey struct{}

// BrowserInstance is supplied by an authenticated Bridge, either for a proxy
// ingress or a directly executed browser test, never by a business HTTP header.
type BrowserInstance struct {
	Manager        string
	InstanceID     string
	InstallationID string
	Badge          string
}

func (identity BrowserInstance) HTTPFlowTag() string {
	badge := strings.TrimSpace(identity.Badge)
	if badge == "" {
		return ""
	}
	return "[浏览器 " + badge + "]"
}

// WithBrowserInstance carries an authenticated identity through one tool
// invocation, including concurrent requests, redirects and retries.
func WithBrowserInstance(ctx context.Context, identity BrowserInstance) context.Context {
	return context.WithValue(ctx, browserInstanceContextKey{}, identity)
}

func SetBrowserInstance(req *http.Request, identity BrowserInstance) {
	SetContextValueInfoFromRequest(req, BrowserInstanceContextKey, identity)
	tag := identity.HTTPFlowTag()
	if tag == "" {
		return
	}
	tags := GetFlowTags(req)
	for _, existing := range tags {
		if existing == tag {
			return
		}
	}
	SetFlowTags(req, append(tags, tag))
}

func GetBrowserInstance(req *http.Request) (BrowserInstance, bool) {
	if req == nil {
		return BrowserInstance{}, false
	}
	if identity, ok := req.Context().Value(browserInstanceContextKey{}).(BrowserInstance); ok {
		return identity, true
	}
	identity, ok := GetContextAnyFromRequest(req, BrowserInstanceContextKey).(BrowserInstance)
	return identity, ok
}
