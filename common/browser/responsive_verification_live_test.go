package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func TestLiveViewportDiagnosticsUsesCDPMetricsAndCapturesErrors(t *testing.T) {
	browserPath, ok := launcher.LookPath()
	if !ok {
		t.Skip("local browser is unavailable")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html>
<html>
<head>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>body { margin: 0; } #wide { width: 700px; height: 24px; background: red; }</style>
</head>
<body>
  <div id="wide">wide</div>
  <script>
    console.error("console-error-sentinel");
    setTimeout(function () { throw new Error("runtime-error-sentinel"); }, 0);
    Promise.reject(new Error("unhandled-rejection-sentinel"));
  </script>
</body>
</html>`)
	}))
	defer server.Close()

	id := fmt.Sprintf("viewport-live-%d", time.Now().UnixNano())
	instance, err := Open(
		WithID(id),
		WithHeadless(true),
		WithExePath(browserPath),
		WithTimeout(10),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = CloseByID(WithID(id)) })

	page, err := instance.Navigate(server.URL)
	require.NoError(t, err)
	require.NoError(t, page.SetViewport(390, 844, 1, false))
	require.NoError(t, page.WaitTime(200))

	diagnostics, err := page.ViewportDiagnostics()
	require.NoError(t, err)
	t.Logf("viewport diagnostics: %+v", diagnostics)
	require.Equal(t, 390, diagnostics.InnerWidth)
	require.Equal(t, 844, diagnostics.InnerHeight)
	require.True(t, diagnostics.OverflowX)
	require.Greater(t, diagnostics.HorizontalOverflowPixels, 0)
	require.Greater(t, diagnostics.VisibleElements, 0)
	require.NotEmpty(t, diagnostics.OverflowElements)
	require.Contains(t, diagnostics.ConsoleErrors, "console-error-sentinel")
	require.NotEmpty(t, diagnostics.RuntimeErrors)
}

func TestRuntimeDiagnosticsRejectsEmptyPage(t *testing.T) {
	var nilPage *BrowserPage
	require.Error(t, nilPage.InstallRuntimeDiagnostics())
	_, err := nilPage.ViewportDiagnostics()
	require.Error(t, err)
	require.Error(t, nilPage.SetViewport(390, 844, 1, true))
	require.Error(t, nilPage.ClearViewport())
}
