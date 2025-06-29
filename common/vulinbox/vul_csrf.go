package vulinbox

import (
	_ "embed"
	"html/template"
	"net/http"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed html/vul_csrf_unsafe.html
var unsafeUpdate []byte

//go:embed html/vul_csrf_safe.html
var safeUpdate []byte

var info = "adminPassword"

func (s *VulinServer) registerCsrf() {
	var router = s.router
	csrfGroup := router.PathPrefix("/csrf").Name("表单 CSRF 保护测试").Subrouter()

	var sessionToToken = sync.Map{}

	csrfRoutes := []*VulInfo{
		{
			DefaultQuery: "",
			Path:         "/unsafe",
			Title:        "没有保护的表单",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
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
				// 1. Manage session
				var sessionID string
				sessionCookie, err := request.Cookie("session_id")
				if err != nil || sessionCookie.Value == "" {
					// No session, create one if it is a GET request
					if request.Method == http.MethodPost {
						log.Warnf("csrf check failed, session cookie not found on post")
						writer.Write([]byte("session cookie not found"))
						writer.WriteHeader(http.StatusForbidden)
						return
					}
					sessionID = utils.RandStringBytes(32)
					http.SetCookie(writer, &http.Cookie{Name: "session_id", Value: sessionID, Path: "/"})
				} else {
					sessionID = sessionCookie.Value
				}

				// On POST, verify the token
				if request.Method == http.MethodPost {
					submittedToken := request.PostFormValue("csrf_token")
					expectedToken, ok := sessionToToken.LoadAndDelete(sessionID)

					if !ok || submittedToken == "" || submittedToken != expectedToken.(string) {
						log.Warnf("csrf token check failed for session %s, submitted: %s, expected: %v, ok: %v", sessionID, submittedToken, expectedToken, ok)
						writer.Write([]byte("csrf_token check error"))
						writer.WriteHeader(http.StatusForbidden)
						return
					}

					// CSRF check passed. Regenerate the session ID to make it single-use for POST operations.
					sessionID = utils.RandStringBytes(32)
					http.SetCookie(writer, &http.Cookie{Name: "session_id", Value: sessionID, Path: "/"})
				}

				// For every request (GET or successful POST), issue a new CSRF token for the session.
				newToken := utils.RandStringBytes(32)
				sessionToToken.Store(sessionID, newToken)

				data := map[string]string{
					"Info":  info,
					"Token": newToken,
				}
				updateInfo(writer, request, data, string(safeUpdate))
			},
		},
	}

	for _, v := range csrfRoutes {
		addRouteWithVulInfo(csrfGroup, v)
	}

}

func updateInfo(writer http.ResponseWriter, request *http.Request, data map[string]string, tpl string) {
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
	tmpl, err := template.New("csrf").Parse(tpl)
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
