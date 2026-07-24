package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsHTTPFlowBuiltinTag(t *testing.T) {
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagDiscarded))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagAutoFixResponse))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagResend))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagResend+"tag1"))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagWebFuzzer))

	require.False(t, IsHTTPFlowBuiltinTag("webfuzzer"))
	require.False(t, IsHTTPFlowBuiltinTag("custom-tag"))
}

func TestHTTPFlowTagsFromCounts(t *testing.T) {
	tags := HTTPFlowTagsFromCounts(map[string]int{
		"custom-tag":         3,
		HTTPFlowTagDiscarded: 2,
	})

	byValue := make(map[string]*TagAndStatusCode, len(tags))
	for _, tag := range tags {
		byValue[tag.Value] = tag
	}

	require.Equal(t, 3, byValue["custom-tag"].Count)
	require.False(t, byValue["custom-tag"].Builtin)

	require.Equal(t, 2, byValue[HTTPFlowTagDiscarded].Count)
	require.True(t, byValue[HTTPFlowTagDiscarded].Builtin)

	for builtin := range HTTPFlowBuiltinTags {
		got, ok := byValue[builtin]
		require.True(t, ok, "missing builtin tag %s", builtin)
		require.True(t, got.Builtin)
		if builtin != HTTPFlowTagDiscarded {
			require.Equal(t, 0, got.Count)
		}
	}
}
