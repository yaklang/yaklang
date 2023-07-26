package vulinbox

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinbox/verificationcode"
	"net/http"
	"strings"
)

//go:embed route.html
var routeHtml []byte

func (s *VulinServer) init() {
	if s.wsAgent.wChan == nil {
		s.wsAgent.wChan = make(chan any, 10000)
	}

	router := s.router

	// FE AND FEEDBACK
	fe := http.FileServer(http.FS(staticFS))
	router.NotFoundHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		/* load to agent feedback */
		reqRaw, err := utils.HttpDumpWithBody(request, true)
		if err != nil {
			log.Errorf("dump request failed: %v", err)
		}
		if len(reqRaw) > 0 {
			s.wsAgent.TrySend(NewDataBackAction("http-request", string(reqRaw)))
		}

		if strings.HasPrefix(request.URL.Path, "/static") {
			var u, _ = lowhttp.ExtractURLFromHTTPRequest(request, true)
			if u != nil {
				log.Infof("request static file: %v", u.Path)
				// request.URL.Path = strings.TrimLeft(request.URL.Path, "/")
			}
			fe.ServeHTTP(writer, request)
			return
		}
		log.Infof("404 for %s", request.URL.Path)
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(404)
			writer.Write([]byte("404 not found"))
		}).ServeHTTP(writer, request)
	})
	router.Use(func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			reqRaw, err := utils.HttpDumpWithBody(request, false)
			if err != nil {
				log.Errorf("dump request failed: %v", err)
			}
			if len(reqRaw) > 0 {
				s.wsAgent.TrySend(NewDataBackAction("http-request", string(reqRaw)))
			}
			handler.ServeHTTP(writer, request)
		})
	})
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
	s.registerUploadCases()

	// 业务型
	s.registerUserRoute()

	// 验证码
	verificationcode.Register(router)

	s.registerJSONP()
	s.registerPostMessageIframeCase()
	s.registerSensitive()

	// 靶场是否是安全的？
	if !s.safeMode {
		s.registerPingCMDI()
	}
}
