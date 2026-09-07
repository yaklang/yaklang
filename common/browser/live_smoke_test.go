package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
)

func TestLiveBrowserFrameClickableAndNetworkTap(t *testing.T) {
	browserPath, ok := launcher.LookPath()
	if !ok {
		t.Skip("local browser is unavailable")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><html><body>
<iframe src="/hidden" style="display:none"></iframe>
<iframe src="/frame" style="width:500px;height:300px"></iframe>
<script>fetch('/api').then(r => r.text()).then(v => { window.apiBody = v; window.apiDone = true; });</script>
</body></html>`)
	})
	mux.HandleFunc("/hidden", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "hidden")
	})
	mux.HandleFunc("/frame", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><html><body>
<div id="clickable" style="cursor:pointer;width:200px;height:40px" onclick="window.clicked=true">中文按钮文案测试</div>
<x-shadow-action></x-shadow-action>
<script>
customElements.define('x-shadow-action', class extends HTMLElement {
  connectedCallback() {
    const root = this.attachShadow({mode: 'open'});
    root.innerHTML = '<div style="cursor:pointer;width:200px;height:40px" onclick="window.shadowClicked=true">阴影点击目标</div>';
  }
});
</script>
</body></html>`)
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "api-ok")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	inst, err := newBrowserInstance("live-smoke", parseBrowserOptions(
		WithExePath(browserPath),
		WithHeadless(true),
		WithLeakless(false),
		WithTimeout(10),
	))
	require.NoError(t, err)
	defer func() { require.NoError(t, inst.Close()) }()

	rodPage, err := inst.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	require.NoError(t, err)
	page := newBrowserPage(rodPage, inst, 10*time.Second)
	require.NoError(t, page.StartNetworkTap())
	require.NoError(t, page.Navigate(server.URL))
	require.NoError(t, page.WaitFunction("window.apiDone === true"))

	deadline := time.Now().Add(2 * time.Second)
	var records []map[string]any
	for time.Now().Before(deadline) {
		records = append(records, page.DrainNetworkTap()...)
		for _, record := range records {
			if record["url"] == server.URL+"/api" {
				require.Equal(t, "api-ok", record["response"])
				goto networkCaptured
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("network tap did not capture /api")

networkCaptured:
	frames := page.listFrameInfos()
	require.Len(t, frames, 1)
	require.Equal(t, 1, frames[0].ElementIndex)
	require.NoError(t, page.UseFrame(0))

	items := page.ListClickable()
	var clickableRef string
	var shadowRef string
	for _, item := range items {
		if item["name"] == "中文按钮文案测试" {
			clickableRef, _ = item["ref"].(string)
		}
		if item["name"] == "阴影点击目标" {
			shadowRef, _ = item["ref"].(string)
		}
	}
	require.NotEmpty(t, clickableRef)
	require.NoError(t, page.Click(clickableRef))
	clicked, err := page.Evaluate("window.clicked === true")
	require.NoError(t, err)
	require.Equal(t, true, clicked)
	require.NotEmpty(t, shadowRef)
	require.NoError(t, page.Click(shadowRef))
	shadowClicked, err := page.Evaluate("window.shadowClicked === true")
	require.NoError(t, err)
	require.Equal(t, true, shadowClicked)
	require.NoError(t, page.UseMainFrame())
}
