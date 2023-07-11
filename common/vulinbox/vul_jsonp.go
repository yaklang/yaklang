package vulinbox

import (
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/regen"
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

func (s *VulinServer) registerJSONP() {
	r := s.router
	r.HandleFunc("/jsonp/center", func(writer http.ResponseWriter, request *http.Request) {
		if !ForceEnsureCookie(writer, request, "checkpoint", "1") {
			regen.GenerateOne()
			return
		}


		request.URL.Query().Get("callback")
		writer.Header().Set("Content-Type", "application/javascript")
		writer.Write([]byte(`(function(){
	return "This Message is FROM /jsonp/center~, checkpoint is set~"
})()`))
	})
	r.HandleFunc("/jsonp/basic", func(writer http.ResponseWriter, request *http.Request) {
		if !ForceEnsureCookie(writer, request, "checkpoint", "1") {
			return
		}
		writer.Header().Set("Content-Type", "application/javascript")
		writer.Write([]byte(`(function(){
	return "This Message is FROM /jsonp/center~, checkpoint is set~"
})()`))
	})
}
