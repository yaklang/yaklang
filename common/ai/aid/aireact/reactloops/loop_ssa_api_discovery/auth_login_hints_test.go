package loop_ssa_api_discovery

import (
	"strings"
	"testing"
)

func TestBuildLoginProbeAttempts_IncludesAuthLoginJSON(t *testing.T) {
	paths := []string{"/api/auth/login", "/login"}
	attempts := buildLoginProbeAttempts("admin", "Admin@2024!", paths)
	if len(attempts) < 2 {
		t.Fatalf("expected multiple attempts, got %d", len(attempts))
	}
	foundJSON := false
	foundAuthPath := false
	for _, a := range attempts {
		if a.path == "/api/auth/login" {
			foundAuthPath = true
		}
		if a.contentType == "application/json" && strings.Contains(a.body, `"username":"admin"`) &&
			strings.Contains(a.body, `"password":"Admin@2024!"`) {
			foundJSON = true
		}
	}
	if !foundAuthPath {
		t.Fatal("expected /api/auth/login attempt")
	}
	if !foundJSON {
		t.Fatal("expected JSON login body with credentials")
	}
}

func TestBuildLoginProbeAttempts_EmptyPaths(t *testing.T) {
	if attempts := buildLoginProbeAttempts("admin", "x", nil); len(attempts) != 0 {
		t.Fatalf("expected no attempts without evidence paths, got %d", len(attempts))
	}
}

func TestBuildLoginProbeAttempts_FormBodyIsURLEncoded(t *testing.T) {
	attempts := buildLoginProbeAttempts("admin", "Admin@2024!", []string{"/login"})
	for _, a := range attempts {
		if a.contentType != "application/x-www-form-urlencoded" {
			continue
		}
		if !strings.Contains(a.body, "password=Admin%402024%21") && !strings.Contains(a.body, "password=Admin%402024!") {
			t.Fatalf("expected encoded password in form body, got %q", a.body)
		}
		return
	}
	t.Fatal("expected form-urlencoded attempt")
}

func TestLoginProbeRejected_405And415(t *testing.T) {
	if !loginProbeRejected("HTTP/1.1 405 Method Not Allowed") {
		t.Fatal("405 should be rejected")
	}
	if !loginProbeRejected("HTTP/1.1 415 Unsupported Media Type") {
		t.Fatal("415 should be rejected")
	}
	if loginProbeRejected("HTTP/1.1 404 Not Found") {
		t.Fatal("404 alone should not reject login (redirect target may 404)")
	}
}

func TestLoginProbeSuccessful_RedirectWithCookieDespite404(t *testing.T) {
	content := "HTTP/1.1 302 Found\r\nSet-Cookie: JSESSIONID=abc; Path=/\r\nSet-Cookie: APP=1; Path=/\r\nLocation: /admin/\r\n\r\nHTTP/1.1 404 Not Found"
	if !loginProbeSuccessful(content) {
		t.Fatal("302 + Set-Cookie should succeed even if follow-up is 404")
	}
}

func TestExtractAllSetCookiePairsFromResponse(t *testing.T) {
	content := "HTTP/1.1 302\r\nSet-Cookie: JSESSIONID=abc; Path=/\r\nSet-Cookie: APP=token; Path=/\r\n"
	pairs := extractAllSetCookiePairsFromResponse(content)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 cookies, got %v", pairs)
	}
	cookie := extractSetCookieFromResponse(content)
	if cookie != "JSESSIONID=abc; APP=token" {
		t.Fatalf("unexpected merged cookie: %q", cookie)
	}
}

func TestBuildAuthHeadersJSONFromLoginResponse(t *testing.T) {
	content := "HTTP/1.1 302\r\nSet-Cookie: A=1\r\nSet-Cookie: B=2\r\n"
	jsonStr := buildAuthHeadersJSONFromLoginResponse(content)
	if !strings.Contains(jsonStr, `"Cookie":"A=1; B=2"`) && !strings.Contains(jsonStr, `"Cookie": "A=1; B=2"`) {
		t.Fatalf("unexpected headers json: %s", jsonStr)
	}
}
