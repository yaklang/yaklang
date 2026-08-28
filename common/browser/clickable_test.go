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
}

func TestListClickableWithoutPage(t *testing.T) {
	t.Parallel()
	p := &BrowserPage{}
	require.Empty(t, p.ListClickable())
}

func TestParseMarkClickableExtras(t *testing.T) {
	t.Parallel()
	extras := parseMarkClickableExtras(`{"n":1,"extras":[{"name":"阴影按钮","role":"button","x":10,"y":20,"shadow":true}]}`)
	require.Len(t, extras, 1)
	require.Equal(t, "阴影按钮", extras[0].Name)
	require.Equal(t, 10.0, extras[0].X)
}
