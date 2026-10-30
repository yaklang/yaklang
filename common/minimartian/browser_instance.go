package minimartian

import (
	"net/http"

	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func applyBrowserInstance(req *http.Request, ctx *Context) {
	if identity, ok := ctx.GetSessionValue("ConnMetaInfo_" + httpctx.BrowserInstanceContextKey).(httpctx.BrowserInstance); ok {
		httpctx.SetBrowserInstance(req, identity)
	}
}
