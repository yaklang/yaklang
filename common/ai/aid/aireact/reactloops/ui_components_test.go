package reactloops

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func newUIProjectionLoop(t *testing.T) (*ReActLoop, *aicommon.Config) {
	t.Helper()
	raw, err := os.ReadFile("../../interaction/testdata/catalog.json")
	require.NoError(t, err)
	capabilities, err := json.Marshal(aicommon.UIClientCapabilities{Protocol: "memfit.ui/1", Questions: true, Catalog: raw})
	require.NoError(t, err)
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithClientUI(string(capabilities)), aicommon.WithAllowRequireForUserInteract(true))
	return NewMinimalReActLoop(cfg, nil), cfg
}

func TestUIComponentProjectionLifecycle(t *testing.T) {
	loop, cfg := newUIProjectionLoop(t)
	task := aicommon.NewStatefulTaskBase("first", "visualize", context.Background(), cfg.Emitter)
	defer task.Cancel()
	loop.SetCurrentTask(task)
	require.Empty(t, loop.renderUIComponents())
	receipt, err := loop.LoadUIComponents([]string{"BarChart", "BarChart"})
	require.NoError(t, err)
	require.Equal(t, []string{"BarChart"}, receipt["names"])
	before := loop.renderUIComponents()
	require.Contains(t, before, `"name":"BarChart"`)
	_, err = loop.LoadUIComponents([]string{"BarChart"})
	require.NoError(t, err)
	require.Equal(t, before, loop.renderUIComponents())
	_, err = loop.LoadUIComponents([]string{"unknown"})
	require.Error(t, err)
	require.Equal(t, before, loop.renderUIComponents(), "failed loads must not destroy usable definitions")
	_, err = loop.LoadUIComponents([]string{"Text"})
	require.NoError(t, err)
	require.NotContains(t, loop.renderUIComponents(), `"name":"BarChart"`)
	require.Contains(t, loop.renderUIComponents(), `"name":"Text"`)
	timeline := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.NotContains(t, timeline.Open+timeline.Frozen, "UI_COMPONENT_DEFINITIONS")

	other := NewMinimalReActLoop(cfg, nil)
	require.Empty(t, other.renderUIComponents(), "workers sharing Config must not share loaded definitions")
	second := aicommon.NewStatefulTaskBase("second", "another task", context.Background(), cfg.Emitter)
	defer second.Cancel()
	loop.SetCurrentTask(second)
	require.Empty(t, loop.renderUIComponents())
	_, err = loop.LoadUIComponents([]string{"Text"})
	require.NoError(t, err)
	_, err = loop.LoadUIComponents([]string{})
	require.NoError(t, err)
	require.Empty(t, loop.renderUIComponents())
	_, err = loop.LoadUIComponents([]string{"Text"})
	require.NoError(t, err)
	second.Cancel()
	require.Empty(t, loop.renderUIComponents(), "cancelled tasks cannot retain definitions")
}

func TestUIComponentProjectionInvalidatesVersionAndPermissions(t *testing.T) {
	loop, cfg := newUIProjectionLoop(t)
	_, err := loop.LoadUIComponents([]string{"DataTable"})
	require.NoError(t, err)
	require.NotEmpty(t, loop.renderUIComponents())
	require.NoError(t, aicommon.WithAllowRequireForUserInteract(false)(cfg))
	require.NotEmpty(t, loop.renderUIComponents(), "display definitions do not depend on question permission")
	_, err = loop.LoadUIComponents([]string{"ParameterForm"})
	require.Error(t, err)
	page, err := loop.SearchUIComponents("ParameterForm", 0)
	require.NoError(t, err)
	require.Empty(t, page.Matches)
	_, err = loop.LoadUIComponents([]string{"DataTable"})
	require.NoError(t, err)
	require.NotContains(t, loop.renderUIComponents(), `"actions":`)
	require.NotContains(t, loop.renderUIComponents(), `"bindings":`)

	catalog := cfg.UICatalog()
	for _, changeVersion := range []bool{true, false} {
		_, err = loop.LoadUIComponents([]string{"Text"})
		require.NoError(t, err)
		if changeVersion {
			catalog.Version = "new-version"
		} else {
			catalog.Components[0].Description += " changed within the same version"
		}
		raw, _ := json.Marshal(catalog)
		caps, _ := json.Marshal(aicommon.UIClientCapabilities{Protocol: "memfit.ui/1", Catalog: raw})
		require.NoError(t, aicommon.WithClientUI(string(caps))(cfg))
		require.Empty(t, loop.renderUIComponents())
	}
	_, err = loop.LoadUIComponents([]string{"Text"})
	require.NoError(t, err)
	loop.Release()
	require.Empty(t, loop.renderUIComponents())
}
