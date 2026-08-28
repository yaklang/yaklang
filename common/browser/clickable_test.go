package browser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkClickableJSCoversPointerAndClickHints(t *testing.T) {
	t.Parallel()
	require.Contains(t, markClickableJS, "cursor === \"pointer\"")
	require.Contains(t, markClickableJS, "getAttribute(\"onclick\")")
	require.Contains(t, markClickableJS, "role === \"menuitem\"")
	require.Contains(t, markClickableJS, "tab === \"0\"")
	require.Contains(t, markClickableJS, "intentScore")
	require.Contains(t, markClickableJS, "新增")
	require.Contains(t, markClickableJS, "保存")
	require.Contains(t, markClickableJS, "ranked.sort")
	require.Contains(t, markClickableJS, "shadowRoot")
	require.Contains(t, markClickableJS, "extras")
	require.Contains(t, markClickableJS, "aria-disabled")
	require.Contains(t, markClickableJS, clickableMarkerPrefix)
}

func TestListClickableWithoutPage(t *testing.T) {
	t.Parallel()
	p := &BrowserPage{}
	require.Empty(t, p.ListClickable())
}

func TestParseMarkClickableExtras(t *testing.T) {
	t.Parallel()
	result := parseMarkClickableResult(`{"marker":"data-yaklang-clickable-abc-123","n":1,"extras":[{"name":"阴影按钮","role":"button","x":10,"y":20,"index":3,"shadow":true}]}`)
	require.Equal(t, "data-yaklang-clickable-abc-123", result.Marker)
	require.Len(t, result.Extras, 1)
	require.Equal(t, "阴影按钮", result.Extras[0].Name)
	require.Equal(t, 10.0, result.Extras[0].X)
	require.Equal(t, 3, result.Extras[0].Index)

	result = parseMarkClickableResult(`{"marker":"data-yaklang-clickable-bad\"]","extras":[]}`)
	require.Empty(t, result.Marker)
}

func TestClipRunesPreservesUTF8(t *testing.T) {
	t.Parallel()
	require.Equal(t, "中文按钮", clipRunes("中文按钮文案", 4))
	require.Equal(t, "中文按钮...", truncateRunes("中文按钮文案", 4))
	require.Empty(t, clipRunes("text", 0))
}

func TestListItemIsNotInteractiveByDefault(t *testing.T) {
	t.Parallel()
	require.False(t, interactiveRoles["listitem"])
}
