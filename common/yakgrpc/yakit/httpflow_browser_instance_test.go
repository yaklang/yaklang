package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestBrowserInstanceHTTPFlowTags(t *testing.T) {
	identity := httpctx.BrowserInstance{Manager: "ytray", InstanceID: "instance-a", Badge: "A"}
	tags := AppendBrowserInstanceHTTPFlowTags([]string{HTTPFlowTagBrowserPlaintext}, identity)
	require.Equal(t, []string{HTTPFlowTagBrowserPlaintext, "[浏览器 A]"}, tags)
	require.Equal(t, tags, AppendBrowserInstanceHTTPFlowTags(tags, identity))

	shared := make([]string, 1, 8)
	shared[0] = "custom"
	a := AppendBrowserInstanceHTTPFlowTags(shared, httpctx.BrowserInstance{Badge: "A"})
	b := AppendBrowserInstanceHTTPFlowTags(shared, httpctx.BrowserInstance{Badge: "B"})
	require.Equal(t, []string{"custom", "[浏览器 A]"}, a)
	require.Equal(t, []string{"custom", "[浏览器 B]"}, b)
}

func TestBrowserInstanceHTTPFlowPreservesExplicitColor(t *testing.T) {
	identity := httpctx.BrowserInstance{Manager: "ytray", InstanceID: "instance-b", Badge: "B", ColorizeHTTPFlow: true}
	flow := &schema.HTTPFlow{Tags: "important|" + schema.FLOW_COLOR_RED}
	ApplyBrowserInstanceToHTTPFlow(flow, identity)
	require.Equal(t, "important|"+schema.FLOW_COLOR_RED+"|[浏览器 B]", flow.Tags)
	require.False(t, flow.HasColor(schema.FLOW_COLOR_BLUE))
	ApplyBrowserInstanceToHTTPFlow(flow, identity)
	require.Equal(t, "important|"+schema.FLOW_COLOR_RED+"|[浏览器 B]", flow.Tags)
}

func TestBrowserInstanceHTTPFlowDoesNotAssignColor(t *testing.T) {
	flow := &schema.HTTPFlow{}
	ApplyBrowserInstanceToHTTPFlow(flow, httpctx.BrowserInstance{Manager: "ytray", InstanceID: "instance-b", Badge: "B"})
	require.Equal(t, "[浏览器 B]", flow.Tags)
	require.NotContains(t, flow.Tags, schema.COLORPREFIX)

	ordinary := &schema.HTTPFlow{Tags: "custom"}
	ApplyBrowserInstanceToHTTPFlow(ordinary, httpctx.BrowserInstance{})
	require.Equal(t, "custom", ordinary.Tags)
}

func TestBrowserInstanceHTTPFlowAgentColorIsOptIn(t *testing.T) {
	shared := make([]string, 1, 8)
	shared[0] = "custom"
	for badge, color := range map[string]string{"A": schema.FLOW_COLOR_ORANGE, "B": schema.FLOW_COLOR_BLUE} {
		t.Run(badge, func(t *testing.T) {
			identity := httpctx.BrowserInstance{Badge: badge}
			ordinary := AppendBrowserInstanceHTTPFlowTags(shared, identity)
			require.Equal(t, []string{"custom", identity.HTTPFlowTag()}, ordinary)
			identity.ColorizeHTTPFlow = true
			agent := AppendBrowserInstanceHTTPFlowTags(shared, identity)
			require.Equal(t, []string{"custom", identity.HTTPFlowTag(), color}, agent)
			require.Equal(t, agent, AppendBrowserInstanceHTTPFlowTags(agent, identity))
			require.Equal(t, []string{"custom"}, shared)
		})
	}
}
