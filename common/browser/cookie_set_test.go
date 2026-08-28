package browser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestSetCookieWithoutPage(t *testing.T) {
	t.Parallel()
	var nilPage *BrowserPage
	require.Error(t, nilPage.SetCookie("sid", "1", "https://app.example"))
	require.Error(t, (&BrowserPage{}).SetCookie("sid", "1", "https://app.example"))
}
