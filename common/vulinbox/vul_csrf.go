package vulinbox

import (
	_ "embed"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed html/vul_csrf_unsafe.html
var unsafeUpdate []byte

//go:embed html/vul_csrf_safe.html
var safeUpdate []byte

var csrfBruteForceHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>CSRF Brute Force</title>
</head>
<body>
    <h1>Guess the PIN</h1>
    <p>{{.Message}}</p>
    <form action="/csrf/bruteforce" method="post">
        <label for="pin">PIN (4 digits):</label>
        <input type="text" id="pin" name="pin" maxlength="4" pattern="\d{4}" title="PIN must be 4 digits">
        <input type="hidden" name="csrf_token" value="{{.Token}}">
        <input type="submit" value="Submit">
    </form>
</body>
</html>
`

var info = "adminPassword"

var (
	csrfPinCode string
	initPinOnce sync.Once
)

func initCsrfPin() {
	initPinOnce.Do(func() {
		sRand := rand.New(rand.NewSource(time.Now().UnixNano()))
		// generate a number between 0 and 9999
		pin := sRand.Intn(10000)
		csrfPinCode = fmt.Sprintf("%04d", pin)
		log.Infof("CSRF bruteforce PIN generated: %s", csrfPinCode)
	})
}

func (s *VulinServer) registerCsrf() {
	initCsrfPin()
	var router = s.router
	csrfGroup := router.PathPrefix("/csrf").Name("表单 CSRF 保护测试").Subrouter()

	var sessionToToken = sync.Map{}

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
		{
			DefaultQuery: "",
			Path:         "/bruteforce",
			Title:        "CSRF PIN码爆破",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				// 1. Manage session
				var sessionID string
				sessionCookie, err := request.Cookie("session_id")
				if err != nil || sessionCookie.Value == "" {
					if request.Method == http.MethodPost {
						log.Warnf("csrf bruteforce check failed, session cookie not found on post")
						newSessionID := utils.RandStringBytes(32)
						http.SetCookie(writer, &http.Cookie{Name: "session_id", Value: newSessionID, Path: "/"})
						http.Redirect(writer, request, "/csrf/bruteforce", http.StatusFound)
						return
					}
					sessionID = utils.RandStringBytes(32)
					http.SetCookie(writer, &http.Cookie{Name: "session_id", Value: sessionID, Path: "/"})
				} else {
					sessionID = sessionCookie.Value
				}

				message := "请输入4位PIN码进行验证"
				if request.Method == http.MethodPost {
					// 2. CSRF Token validation
					submittedToken := request.PostFormValue("csrf_token")
					expectedToken, ok := sessionToToken.Load(sessionID)

					if !ok || submittedToken == "" || submittedToken != expectedToken.(string) {
						log.Warnf("csrf token check failed for bruteforce with session %s, submitted: %s, expected: %v, ok: %v", sessionID, submittedToken, expectedToken, ok)
						newSessionID := utils.RandStringBytes(32)
						http.SetCookie(writer, &http.Cookie{Name: "session_id", Value: newSessionID, Path: "/"})
						http.Redirect(writer, request, "/csrf/bruteforce", http.StatusFound)
						return
					}
					sessionToToken.Delete(sessionID)

					// 3. Regenerate Session ID for single-use POST
					sessionID = utils.RandStringBytes(32)
					http.SetCookie(writer, &http.Cookie{Name: "session_id", Value: sessionID, Path: "/"})

					// 4. Actual PIN check logic
					pin := request.PostFormValue("pin")
					if pin == csrfPinCode {
						message = "成功! PIN码正确。"
					} else {
						message = "失败! PIN码错误，请重试。"
					}
				}

				// 5. Issue new CSRF token for the (new) session
				newToken := utils.RandStringBytes(32)
				sessionToToken.Store(sessionID, newToken)

				tpl, err := template.New("bruteforce").Parse(csrfBruteForceHTML)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusInternalServerError)
					return
				}

				data := map[string]string{
					"Message": message,
					"Token":   newToken,
				}

				writer.Header().Set("Content-Type", "text/html; charset=utf-8")
				err = tpl.Execute(writer, data)
				if err != nil {
					log.Errorf("CSRF bruteforce template execution failed: %v", err)
				}
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
