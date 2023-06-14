package vulinbox

import (
	_ "embed"
	"net/http"
)

//go:embed route.html
var routeHtml []byte

//go:embed route_safe.html
var routeSafeHtml []byte

func (s *VulinServer) init() {
	router := s.router

	// 创建UserManager实例
	userMgr := newUserManager(s.database.db)
	s.userMgr = userMgr

	router.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=UTF8")
		if s.safeMode {
			writer.Write(routeSafeHtml)
		} else {
			writer.Write(routeHtml)
		}
	})
	// 通用型
	s.registerSQLinj()
	s.registerXSS()
	s.registerSSRF()
	s.registerMockVulShiro()
	s.registerExprInj()
	s.registerWebsocket()
	s.registerLoginRoute()
	s.registerCryptoJS()
	s.registerCryptoSM()

	// 业务型
	s.registerUserRoute()

	// 靶场是否是安全的？
	if !s.safeMode {
		s.registerPingCMDI()
	}
}
