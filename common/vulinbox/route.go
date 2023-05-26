package vulinbox

import (
	_ "embed"
	"net/http"
)

//go:embed route.html
var routeHtml []byte

func (s *VulinServer) init() {
	router := s.router

	/*
		SQL注入CASE：http://www.bjski.com.cn/info.php?fid=8&id=1
	*/
	router.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=UTF8")
		writer.Write(routeHtml)
	})
	s.registerSQLinj()
	s.registerSSRF()
	s.registerPingCMDI()
	s.registerExprInj()
	s.registerWebsocket()
}
