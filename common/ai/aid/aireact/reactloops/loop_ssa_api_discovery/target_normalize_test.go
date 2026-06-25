package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeTargetString(t *testing.T) {
	require.Equal(t, "http://127.0.0.1:8080", NormalizeTargetString("127.0.0.1:8080"))
	require.Equal(t, "http://10.0.0.1:9090", NormalizeTargetString("10.0.0.1:9090"))
	require.Equal(t, "http://example.com", NormalizeTargetString("example.com"))
	require.Equal(t, "https://x.example/foo/bar", NormalizeTargetString("<https://x.example/foo/bar>"))
	require.Equal(t, "http://a/b", NormalizeTargetString("[click](http://a/b)"))
}

func TestParseUserInput_ChineseTargetLabel(t *testing.T) {
	in := "Code path: /tmp/p\n靶机： http://127.0.0.1:8088\n"
	p, err := ParseUserInput(in)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8088", p.TargetRaw)
}
