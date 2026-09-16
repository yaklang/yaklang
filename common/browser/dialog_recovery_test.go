package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func TestLiveBrowserDialogRecovery(t *testing.T) {
	path, ok := launcher.LookPath()
	if !ok {
		t.Skip("local browser is unavailable")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alert" {
			_, _ = w.Write([]byte(`<script>alert("first");confirm("second");</script><p>loaded</p>`))
			return
		}
		_, _ = w.Write([]byte(`<p>next</p>`))
	}))
	defer server.Close()
	inst, err := newBrowserInstance("dialog-recovery", parseBrowserOptions(WithExePath(path), WithHeadless(true), WithLeakless(false), WithTimeout(1)))
	require.NoError(t, err)
	defer inst.Close()
	page, err := inst.Navigate(server.URL + "/alert")
	var dialogErr *DialogBlockingError
	require.ErrorAs(t, err, &dialogErr)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, "first", dialogErr.Dialog.Message)
	require.NotNil(t, page)
	current, err := inst.CurrentPage()
	require.NoError(t, err)
	require.Same(t, page, current)
	require.NoError(t, current.HandleJavaScriptDialog(true, ""))
	require.Eventually(t, func() bool { d, ok := current.GetPendingDialog(); return ok && d.Message == "second" }, 2e9, 10e6)
	require.NoError(t, current.HandleJavaScriptDialog(false, ""))
	require.NoError(t, current.Navigate(server.URL+"/next"))
	_, pending := current.GetPendingDialog()
	require.False(t, pending)
	require.NoError(t, inst.Close())
	_, err = inst.CurrentPage()
	require.ErrorContains(t, err, "closed")
}
