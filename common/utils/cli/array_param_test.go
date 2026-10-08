package cli

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStringSliceJSONAndLegacyArguments(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		want []string
	}{
		{`["a,b"," x\n\"\\ ",""]`, []string{"a,b", " x\n\"\\ ", ""}},
		{`[]`, []string{}},
		{`a, b`, []string{"a", "b"}},
	} {
		app := NewCliApp()
		app.SetArgs([]string{"--values", tc.arg})
		require.Equal(t, tc.want, app.StringSlice("values"))
		files := NewCliApp()
		files.SetArgs([]string{"--values", tc.arg})
		require.Equal(t, tc.want, files.FileNames("values"))
	}
}

func TestJSONArrayNetworkParameters(t *testing.T) {
	for _, tc := range []struct{ raw, legacy string }{
		{`["127.0.0.1","192.168.1.0/30"]`, `127.0.0.1,192.168.1.0/30`},
		{`["80","443","8000-8001"]`, `80,443,8000-8001`},
		{`["example.com","yaklang.com:443"]`, `example.com,yaklang.com:443`},
	} {
		a, b := NewCliApp(), NewCliApp()
		a.SetArgs([]string{"--values", tc.raw})
		b.SetArgs([]string{"--values", tc.legacy})
		require.Equal(t, b.Hosts("values"), a.Hosts("values"))
		require.Equal(t, b.Ports("ports", b.SetDefault(tc.legacy)), a.Ports("ports", a.SetDefault(tc.raw)))
		require.Equal(t, b.Urls("urls", b.SetDefault(tc.legacy)), a.Urls("urls", a.SetDefault(tc.raw)))
	}
	app := NewCliApp()
	app.SetArgs([]string{"--lines", `["a\nb","c"]`})
	require.Equal(t, []string{"a", "b", "c"}, app.LineDict("lines"))
}
