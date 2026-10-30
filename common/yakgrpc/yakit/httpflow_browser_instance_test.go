package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestBrowserInstanceHTTPFlowColor(t *testing.T) {
	for badge, color := range map[string]string{
		"A":  schema.FLOW_COLOR_ORANGE,
		"B":  schema.FLOW_COLOR_BLUE,
		"C":  schema.FLOW_COLOR_GREEN,
		"D":  schema.FLOW_COLOR_PURPLE,
		"E":  schema.FLOW_COLOR_RED,
		"F":  schema.FLOW_COLOR_CYAN,
		"G":  schema.FLOW_COLOR_ORANGE,
		"AA": schema.FLOW_COLOR_GREEN,
	} {
		t.Run(badge, func(t *testing.T) {
			require.Equal(t, color, browserInstanceHTTPFlowColor(badge))
		})
	}
	for _, badge := range []string{"", "a", "A1", "AAA", "@A"} {
		require.Empty(t, browserInstanceHTTPFlowColor(badge))
	}
}

func TestBrowserInstanceHTTPFlowTags(t *testing.T) {
	identity := httpctx.BrowserInstance{Manager: "ytray", InstanceID: "instance-a", Badge: "A"}
	tags := AppendBrowserInstanceHTTPFlowTags([]string{HTTPFlowTagBrowserPlaintext}, identity)
	require.Equal(t, []string{HTTPFlowTagBrowserPlaintext, "[浏览器 A]", schema.FLOW_COLOR_ORANGE}, tags)
	require.Equal(t, tags, AppendBrowserInstanceHTTPFlowTags(tags, identity))

	shared := make([]string, 1, 8)
	shared[0] = "custom"
	a := AppendBrowserInstanceHTTPFlowTags(shared, httpctx.BrowserInstance{Badge: "A"})
	b := AppendBrowserInstanceHTTPFlowTags(shared, httpctx.BrowserInstance{Badge: "B"})
	require.Equal(t, []string{"custom", "[浏览器 A]", schema.FLOW_COLOR_ORANGE}, a)
	require.Equal(t, []string{"custom", "[浏览器 B]", schema.FLOW_COLOR_BLUE}, b)
}

func TestBrowserInstanceHTTPFlowPreservesExplicitColor(t *testing.T) {
	identity := httpctx.BrowserInstance{Manager: "ytray", InstanceID: "instance-b", Badge: "B"}
	flow := &schema.HTTPFlow{Tags: "important|" + schema.FLOW_COLOR_RED}
	ApplyBrowserInstanceToHTTPFlow(flow, identity)
	require.Equal(t, "important|"+schema.FLOW_COLOR_RED+"|[浏览器 B]", flow.Tags)
	require.False(t, flow.HasColor(schema.FLOW_COLOR_BLUE))
	ApplyBrowserInstanceToHTTPFlow(flow, identity)
	require.Equal(t, "important|"+schema.FLOW_COLOR_RED+"|[浏览器 B]", flow.Tags)
}

func TestBrowserInstanceHTTPFlowDefaultsToStoredColor(t *testing.T) {
	flow := &schema.HTTPFlow{}
	ApplyBrowserInstanceToHTTPFlow(flow, httpctx.BrowserInstance{Manager: "ytray", InstanceID: "instance-b", Badge: "B"})
	require.Equal(t, "[浏览器 B]|"+schema.FLOW_COLOR_BLUE, flow.Tags)
	require.True(t, flow.HasColor(schema.FLOW_COLOR_BLUE))

	ordinary := &schema.HTTPFlow{Tags: "custom"}
	ApplyBrowserInstanceToHTTPFlow(ordinary, httpctx.BrowserInstance{})
	require.Equal(t, "custom", ordinary.Tags)
}
