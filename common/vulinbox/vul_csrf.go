package vulinbox

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"html/template"
	"net/http"
)

//go:embed html/vul_csrf_unsafe.html
var unsafeUpdate []byte

//go:embed html/vul_csrf_safe.html
var safeUpdate []byte

var info = "adminPassword"

func (s *VulinServer) registerCsrf() {
	var router = s.router
	csrfGroup := router.PathPrefix("/csrf").Name("表单 CSRF 保护测试").Subrouter()
	csrfRoutes := []*VulInfo{
		{
			DefaultQuery: "",
			Path:         "/unsafe",
			Title:        "没有保护的表单",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if !cookieCheck(writer, request, "/csrf/unsafe") {
					return
				}
				data := map[string]string{
					"Info": info,
				}
				updateInfo(writer, request, data, string(unsafeUpdate))
			},
		},
		{
			DefaultQuery: "",
			Path:         "/safe",
			Title:        "csrf_token保护的表单",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if !cookieCheck(writer, request, "/csrf/safe") {
					return
				}
				if request.Method == http.MethodPost {
					token := request.PostFormValue("csrf_token")
					if token != codec.Md5("confidential_cookie_vulinbox") {
						writer.Write([]byte("csrf_token check error"))
						writer.WriteHeader(http.StatusForbidden)
						return
					}
				}
				data := map[string]string{
					"Info":  info,
					"Token": codec.Md5("confidential_cookie_vulinbox"),
				}
				updateInfo(writer, request, data, string(safeUpdate))
			},
		},
	}

	for _, v := range csrfRoutes {
		addRouteWithVulInfo(csrfGroup, v)
	}

}

func cookieCheck(writer http.ResponseWriter, request *http.Request, location string) bool {
	raw, _ := utils.HttpDumpWithBody(request, true)
	vulCookie := lowhttp.GetHTTPPacketCookieFirst(raw, "vulCookie")
	if vulCookie == "" {
		http.SetCookie(writer, &http.Cookie{
			Name:  "vulCookie",
			Value: "confidential_cookie",
		})
		writer.Header().Set("Location", location)
		writer.WriteHeader(302)
		return false
	}
	return true
}

func updateInfo(writer http.ResponseWriter, request *http.Request, data map[string]string, tp string) {
	if request.Method == http.MethodPost {
		if request.PostFormValue("info") != "" {
			info = request.PostFormValue("info")
			data["Info"] = info
		} else {
			writer.Write([]byte("缺少参数"))
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	tmpl, err := template.New("csrf").Parse(tp)
	if err != nil {
		writer.Write([]byte(err.Error()))
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(writer, data)
	if err != nil {
		writer.Write([]byte(err.Error()))
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.WriteHeader(http.StatusOK)
}
