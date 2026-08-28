package browser

import "testing"

func TestCookieRequestURL(t *testing.T) {
	t.Parallel()
	if got := cookieRequestURL("192.168.1.4:8181/ms/index.do"); got != "http://192.168.1.4:8181/ms/index.do" {
		t.Fatalf("got %q", got)
	}
	if got := cookieRequestURL("https://app.example/admin"); got != "https://app.example/admin" {
		t.Fatalf("got %q", got)
	}
	if cookieRequestURL("  ") != "" {
		t.Fatal("empty should stay empty")
	}
}
