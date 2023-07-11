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

//go:embed vul_jsonp.html
var vulJSONPHTML []byte

func (s *VulinServer) registerJSONP() {
	r := s.router
	r.HandleFunc("/jsonp/center", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	r.HandleFunc("/jsonp/basic", func(writer http.ResponseWriter, request *http.Request) {
		if !ForceEnsureCookie(writer, request, "checkpoint", "1") {
			return
		}
		writer.Write(vulJSONPHTML)
	})
}
