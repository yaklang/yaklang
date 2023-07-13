package vulinbox

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/vulinbox/verificationcode"
	"net/http"
)

//go:embed route.html
var routeHtml []byte

func (s *VulinServer) init() {
	router := s.router

	router.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=UTF8")
		var renderedData = `<script>const c = document.getElementById("safestyle"); if (c) c.style.display='none';</script>`
		var bytes []byte
		var err error
		if s.safeMode {
			bytes, err = unsafeTemplate(string(routeHtml), map[string]any{
				"safescript": renderedData,
			})
		} else {
			bytes, err = unsafeTemplate(string(routeHtml), map[string]any{
				"safescript": "",
			})
		}
		if err != nil {
			writer.Write(routeHtml)
		} else {
			writer.Write(bytes)
		}
	})

	// agent ws connector
	s.registerWSAgent()

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

	// 验证码
	verificationcode.Register(router)

	s.registerJSONP()
	s.registerPostMessageIframeCase()

	// 靶场是否是安全的？
	if !s.safeMode {
		s.registerPingCMDI()
	}
}
