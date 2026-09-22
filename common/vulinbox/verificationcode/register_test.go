package verificationcode

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/yaklang/fastgocaptcha"
)

// The library migration must retain both intentionally vulnerable scenarios and
// the safe scenario's one-attempt lifecycle.
func TestCaptchaScenarioLifecycle(t *testing.T) {
	for _, scenario := range []struct {
		prefix  string
		consume bool
	}{{"", false}, {"/bad", false}, {"/safe", true}} {
		t.Run(scenario.prefix, func(t *testing.T) {
			router := mux.NewRouter()
			Register(router)
			page := httptest.NewRecorder()
			router.ServeHTTP(page, httptest.NewRequest("GET", "/verification"+scenario.prefix+"/op", nil))
			if page.Code != 200 || len(page.Result().Cookies()) < 1 {
				t.Fatal("session not created")
			}
			cookie := page.Result().Cookies()[0]
			defer sessionCache.Remove(cookie.Value)
			req := httptest.NewRequest("GET", "/verification"+scenario.prefix+"/code", nil)
			req.AddCookie(cookie)
			pic := httptest.NewRecorder()
			router.ServeHTTP(pic, req)
			img, err := png.Decode(bytes.NewReader(pic.Body.Bytes()))
			if err != nil || img.Bounds().Dx() != 150 || img.Bounds().Dy() != 50 {
				t.Fatal("invalid captcha", err)
			}
			values, ok := sessionCache.Get(cookie.Value)
			if !ok {
				t.Fatal("missing session")
			}
			data, ok := values["code"].(*fastgocaptcha.Data)
			if !ok {
				t.Fatal("captcha generator was not migrated")
			}
			form := url.Values{"code": {strings.ToLower(data.Text)}, "password": {defaultPass}}
			for attempt := 0; attempt < 2; attempt++ {
				post := httptest.NewRequest("POST", "/verification"+scenario.prefix+"/op", strings.NewReader(form.Encode()))
				post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				post.AddCookie(cookie)
				result := httptest.NewRecorder()
				router.ServeHTTP(result, post)
				if attempt == 1 && scenario.consume {
					if result.Code == http.StatusOK {
						t.Fatal("safe challenge replayed")
					}
				} else if result.Code != 200 || !bytes.Contains(result.Body.Bytes(), secretHtml) {
					t.Fatal("scenario behavior changed", result.Code)
				}
			}
		})
	}
}
