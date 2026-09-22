package yaklib

import (
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMUSTPASS_HTTPServerCaptchaRoutes(t *testing.T) {
	config := &_httpServerConfig{}
	_httpServerOptCaptchaRoute("/protected", 30, func(w http.ResponseWriter, r *http.Request) { t.Error("unverified request reached protected handler") })(config)
	if config.captchaManager == nil {
		t.Fatal("captcha initialization failed")
	}
	w := httptest.NewRecorder()
	config.routeHandler["/protected"](w, httptest.NewRequest("GET", "/protected", nil))
	if w.Code != 302 || len(w.Result().Cookies()) != 1 {
		t.Fatal("missing session", w.Code)
	}
	req := httptest.NewRequest("GET", "/fastgocaptcha/captcha?fastgocaptcha_path=/protected", nil)
	req.AddCookie(w.Result().Cookies()[0])
	w = httptest.NewRecorder()
	config.routeHandler["/fastgocaptcha/*"](w, req)
	var data map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err, w.Body.String())
	}
	for _, field := range []string{"fastgocaptcha_image_base64", "fastgocaptcha_thumb_base64"} {
		uri, ok := data[field].(string)
		if !ok {
			t.Fatal("missing image", field)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/png;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = png.Decode(strings.NewReader(string(raw))); err != nil {
			t.Fatal(err)
		}
	}
	if data["fastgocaptcha_thumb_x"] != float64(0) {
		t.Fatal("unexpected initial slide position")
	}
}
