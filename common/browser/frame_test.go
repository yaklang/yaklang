package browser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListFramesWithoutPage(t *testing.T) {
	t.Parallel()
	p := &BrowserPage{}
	require.Empty(t, p.ListFrames())
	require.Error(t, p.UseFrame(0))
	require.Error(t, p.UseFrame(""))
	require.Error(t, p.UseMainFrame())
}

func TestParseFrameArgIndexAndURL(t *testing.T) {
	t.Parallel()
	idx, src, hasIdx := parseFrameArg(2)
	require.True(t, hasIdx)
	require.Equal(t, 2, idx)
	require.Empty(t, src)

	idx, src, hasIdx = parseFrameArg(1.0)
	require.True(t, hasIdx)
	require.Equal(t, 1, idx)

	idx, _, hasIdx = parseFrameArg(1.5)
	require.True(t, hasIdx)
	require.Equal(t, -1, idx)

	idx, src, hasIdx = parseFrameArg("https://app.example/frame/list")
	require.False(t, hasIdx)
	require.Equal(t, "https://app.example/frame/list", src)

	_, src, hasIdx = parseFrameArg("")
	require.False(t, hasIdx)
	require.Empty(t, src)
}

func TestPickFrameSameOriginLargestAndRefuseCross(t *testing.T) {
	t.Parallel()
	frames := []frameInfo{
		{Index: 0, Src: "https://other.example/x", Host: "other.example", SameOrigin: false, Area: 9000},
		{Index: 1, Src: "https://app.example/small", Host: "app.example", SameOrigin: true, Area: 100},
		{Index: 2, Src: "https://app.example/list", Host: "app.example", SameOrigin: true, Area: 4000},
	}
	got, ok := pickFrame(frames, nil)
	require.True(t, ok)
	require.Equal(t, "https://app.example/list", got.Src)

	got, ok = pickFrame(frames, 0)
	require.True(t, ok)
	require.False(t, got.SameOrigin)

	got, ok = pickFrame(frames, "/list")
	require.True(t, ok)
	require.Equal(t, "https://app.example/list", got.Src)

	_, ok = pickFrame(frames, 1.5)
	require.False(t, ok)
	_, ok = pickFrame(frames, "/list/missing")
	require.False(t, ok)
}

func TestParseFrameListJSON(t *testing.T) {
	t.Parallel()
	raw := `[{"index":0,"element_index":2,"src":"http://app/x","url":"http://app/x","host":"app","same_origin":true,"visible":true,"area":12}]`
	got := parseFrameList(raw)
	require.Len(t, got, 1)
	require.True(t, got[0].SameOrigin)
	require.Equal(t, "http://app/x", got[0].Src)
	require.Equal(t, 2, got[0].ElementIndex)

	arr := []any{map[string]any{"index": 0, "src": "http://app/y", "url": "http://app/y", "host": "app", "same_origin": true, "visible": true, "area": 9}}
	got = parseFrameList(arr)
	require.Len(t, got, 1)
	require.Equal(t, "http://app/y", got[0].Src)
}

func TestOriginOfNormalizesDefaultPorts(t *testing.T) {
	t.Parallel()
	require.Equal(t, originOf("https://APP.example/path"), originOf("https://app.example:443/other"))
	require.Equal(t, originOf("http://app.example/path"), originOf("http://app.example:80/other"))
	require.NotEqual(t, originOf("http://app.example"), originOf("https://app.example"))
}
