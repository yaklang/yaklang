package vulinbox

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net/http"
)

func ForceEnsureCookie(writer http.ResponseWriter, r *http.Request, key, val string) bool {
	if r == nil {
		Failed(writer, r, "request is nil")
		return false
	}
	cookie, err := r.Cookie(key)
	if cookie != nil && cookie.Name != "" && err == nil {
		return true
	}
	writer.Header().Add("Set-Cookie", lowhttp.CookiesToString([]*http.Cookie{{
		Name:  key,
		Value: val,
	}}))
	writer.Header().Set("Location", r.RequestURI)
	writer.WriteHeader(302)
	return false
}

//go:embed html/vul_jsonp.html
var vulJSONPHTML []byte

func (s *VulinServer) registerJSONP() {
	r := s.router

	// 创建一个路由分组 "/jsonp"
	jsonpGroup := r.PathPrefix("/jsonp").Name("JSONP 通信与 iframe postMessage 通信案例").Subrouter()

	jsonpRoutes := []*VulInfo{
		{
			DefaultQuery: "",
			Path:         "/basic",
			Title:        "JSONP 的最基础案例",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if !ForceEnsureCookie(writer, request, "checkpoint", "1") {
					return
				}
				writer.Write(vulJSONPHTML)
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "",
			Path:         "/center",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if !ForceEnsureCookie(writer, request, "checkpoint", "1") {
					return
				}

				callbackName := request.URL.Query().Get("callback")
				writer.Header().Set("Content-Type", "application/javascript")
				switch callbackName {
				case "exec_checkpoint":
					writer.Write([]byte(`(function(){
	setTimeout(function(){
		document.getElementById("abc").innerText = "This Message is FROM /jsonp/center~, checkpoint is set~"
	}, 1500)
	return "This Message is FROM /jsonp/center~, checkpoint is set~"
})()`))
				default:
					writer.Write([]byte(`(function(){
	alert("NO CHECKING POINT")
	return "no checkpoint cookie"
})()`))
				}

			},
		},
	}
	for _, v := range jsonpRoutes {
		addRouteWithVulInfo(jsonpGroup, v)
	}
}
