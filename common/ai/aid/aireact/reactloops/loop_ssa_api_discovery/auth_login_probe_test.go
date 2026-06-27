package loop_ssa_api_discovery

import (
	"strings"
	"testing"
)

const task78RedirectFollowStdout = `[info] mode: URL (POST http://192.168.1.4:8080/admin/login)
[info] redirect #1 [302] -> index.html;jsessionid=D8C5B72831F8FB81B4C5BE556C460543
[info] request packet (387 bytes):
GET /admin/login/index.html;jsessionid=D8C5B72831F8FB81B4C5BE556C460543 HTTP/1.1
Host: 192.168.1.4:8080
Cookie: JSESSIONID=D8C5B72831F8FB81B4C5BE556C460543; PUBLICCMS_ADMIN=4_c6ff098e-b02e-470e-a497-5fe151ff7a08

[info] response: HTTP/1.1 404 (body size: 0)
[info] response packet (297 bytes):
HTTP/1.1 404
Set-Cookie: PUBLICCMS_USER=; Expires=Thu, 01 Jan 1970 00:00:10 GMT; Path=/; HttpOnly
`

func TestLoginProbeSuccessful_Task78RedirectFollow404(t *testing.T) {
	if !loginProbeSuccessful(task78RedirectFollowStdout) {
		t.Fatal("302 redirect + session Cookie on follow request should count as login success")
	}
	headers := buildAuthHeadersJSONFromLoginResponse(task78RedirectFollowStdout)
	if !strings.Contains(headers, "JSESSIONID=D8C5B72831F8FB81B4C5BE556C460543") {
		t.Fatalf("expected JSESSIONID in headers json, got %s", headers)
	}
	if !strings.Contains(headers, "PUBLICCMS_ADMIN=4_c6ff098e-b02e-470e-a497-5fe151ff7a08") {
		t.Fatalf("expected PUBLICCMS_ADMIN in headers json, got %s", headers)
	}
}

func TestAnalyzeHTTPOutputForLogin_Task78(t *testing.T) {
	out := analyzeHTTPOutputForLogin(
		"POST",
		"http://192.168.1.4:8080/admin/login",
		"username=admin2&password=admin123&secureLogin=false",
		"",
		task78RedirectFollowStdout,
	)
	if out == nil || !out.Success {
		t.Fatal("expected successful login probe outcome")
	}
	if out.Username != "admin2" {
		t.Fatalf("username=%q", out.Username)
	}
	if out.LoginPath != "/admin/login" {
		t.Fatalf("login_path=%q", out.LoginPath)
	}
}

func TestValidSessionCookiePairs_IgnoresExpiredClearCookie(t *testing.T) {
	pairs := validSessionCookiePairsFromHTTPOutput(task78RedirectFollowStdout)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 session cookies, got %v", pairs)
	}
}
