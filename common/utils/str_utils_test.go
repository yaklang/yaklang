package utils

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
)

func TestRemoveUnprintableChars(t *testing.T) {
	cases := map[string]string{
		"\x00W\xffO\x00R\x00K": `\x00W\xffO\x00R\x00K`,
	}
	for input, output := range cases {
		if result := RemoveUnprintableChars(input); result == output {
			continue
		} else {
			t.Logf("expect %#v got %#v", output, result)
			t.FailNow()
		}
	}
}

func TestParseStringToLines(t *testing.T) {
	a := ParseStringToLines(`abc
ccc
ddd`)
	spew.Dump(a)
	assert.Equal(t, a[0], "abc")
	assert.Equal(t, a[1], "ccc")
	assert.Equal(t, a[2], "ddd")
}

func TestMUSTPASS_UrlJoin2(t *testing.T) {
	u, err := UrlJoin("https://baidu.com/a/b.html", "c.html")
	if err != nil {
		panic(err)
	}
	assert.Equal(t, "https://baidu.com/a/c.html", u)
}

func TestMUSTPASS_UrlJoin(t *testing.T) {
	cases := map[string][2]string{
		"/abc":                          {"https://baidu.com/root", "https://baidu.com/abc"},
		"/abc/":                         {"https://baidu.com/root", "https://baidu.com/abc/"},
		"abc":                           {"https://baidu.com/root", "https://baidu.com/root/abc"},
		"abc/":                          {"https://baidu.com/root", "https://baidu.com/root/abc/"},
		"/index.php":                    {"https://baidu.com/root", "https://baidu.com/index.php"},
		"/index.php?a=b":                {"https://baidu.com/root", "https://baidu.com/index.php?a=b"},
		"login.php":                     {"https://baidu.com/root", "https://baidu.com/root/login.php"},
		"login.php?ab=1":                {"https://baidu.com/root", "https://baidu.com/root/login.php?ab=1"},
		"./index.php":                   {"https://baidu.com/root", "https://baidu.com/root/index.php"},
		"../index.php":                  {"https://baidu.com/root", "https://baidu.com/index.php"},
		"./../.././../index.php":        {"https://baidu.com/root/a/b/c/d/e/f", "https://baidu.com/root/a/b/c/index.php"},
		"./././././.././index.php":      {"https://baidu.com/root", "https://baidu.com/index.php"},
		"./index.php?c=123":             {"https://baidu.com/root", "https://baidu.com/root/index.php?c=123"},
		"https://example.com/index.php": {"https://baidu.com/root", "https://example.com/index.php"},
		"http://example.com/index.php":  {"https://baidu.com/root", "http://example.com/index.php"},

		// 这两个不知道应不应该在这么做，但是先这样吧
		"./././././.././a/b/./index.php":  {"https://baidu.com/root", "https://baidu.com/a/b/./index.php"},
		"./././././.././a/b/../index.php": {"https://baidu.com/root", "https://baidu.com/a/b/../index.php"},
	}
	for input, output := range cases {
		input := input
		origin := output[0]
		expected := output[1]
		if result, err := UrlJoin(origin, input); err != nil {
			panic(err)
		} else {
			if result != expected {
				t.Logf("origin: %v input %v", origin, input)
				t.Logf("expect %#v got %#v", expected, result)
				t.FailNow()
			}
		}
	}
}

func TestMUSTPASS_ParseStringToHostPort(t *testing.T) {
	type Result struct {
		Host   string
		Port   int
		hasErr bool
	}
	cases := map[string]Result{
		"http://baidu.com":     {Host: "baidu.com", Port: 80},
		"https://baidu.com":    {Host: "baidu.com", Port: 443},
		"https://baidu.com:88": {Host: "baidu.com", Port: 88},
		"http://baidu.com:88":  {Host: "baidu.com", Port: 88},
		"ws://baidu.com":       {Host: "baidu.com", Port: 80},
		"wss://baidu.com":      {Host: "baidu.com", Port: 443},
		"1.2.3.4:1":            {Host: "1.2.3.4", Port: 1},
		"baidu.com:1":          {Host: "baidu.com", Port: 1},
		"http://[::1]:1":       {Host: "::1", Port: 1},
		"baidu.com":            {Host: "baidu.com", Port: 0, hasErr: true},
		"1.2.3.5":              {Host: "1.2.3.5", Port: 0, hasErr: true},
		"[1:123:123:123]":      {Host: "1:123:123:123", Port: 0, hasErr: true},
		"::1":                  {Host: "::1", Port: 0, hasErr: true},
	}

	for raw, result := range cases {
		host, port, err := ParseStringToHostPort(raw)
		require.Equal(t, result.Host, host)
		require.Equal(t, result.Port, port)
		if result.hasErr {
			require.Error(t, err, "should have error")
		} else {
			require.NoError(t, err, "should not have error")
		}
	}
}

func TestMUSTPASS_SliceGroup(t *testing.T) {
	s := SliceGroup([]string{
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
		"1", "1", "1",
	}, 3)
	log.Info(spew.Sdump(s))
	assert.True(t, len(s) == 7, "%v", spew.Sdump(s))
}

func TestMUSTPASS_HostPort_AppendDefaultPort(t *testing.T) {
	type Case struct {
		Raw  string
		Port int
		Res  string
	}
	cases := []Case{
		{"::1", 113, "[::1]:113"},
		{"baidu.com", 88, "baidu.com:88"},
		{"baidu.com:80", 80, "baidu.com:80"},
		{"http://127.0.0.1", 111, "127.0.0.1:80"},
		{"http://127.0.0.1:8888", 111, "127.0.0.1:8888"},
		{"127.0.0.1", 113, "127.0.0.1:113"},
		{"[::1]:111", 113, "[::1]:111"},
		{"https://[::1]:111", 113, "[::1]:111"},
	}
	for _, c := range cases {
		if res := AppendDefaultPort(c.Raw, c.Port); res != c.Res {
			t.Errorf("expect %s got %s", c.Res, res)
		}
	}
}

func TestMUSTPASS_HostPort(t *testing.T) {
	assert.Equal(t, "127.0.0.1:80", AppendDefaultPort("127.0.0.1:80", 8787))
	assert.Equal(t, "127.0.0.1:8787", AppendDefaultPort("127.0.0.1", 8787))
	assert.Equal(t, "127.0.0.1:80", AppendDefaultPort("http://127.0.0.1", 8787))
	assert.Equal(t, "127.0.0.1:443", AppendDefaultPort("https://127.0.0.1", 8787))
	assert.Equal(t, "127.0.0.1:7777", AppendDefaultPort("https://127.0.0.1:7777", 8787))
	assert.Equal(t, "127.0.0.1:80", AppendDefaultPort("ws://127.0.0.1", 8787))
	assert.Equal(t, "127.0.0.1:443", AppendDefaultPort("wss://127.0.0.1", 8787))
	assert.Equal(t, ":7777", AppendDefaultPort(":7777", 8787))
	assert.Equal(t, ":8787", AppendDefaultPort(":8787", 8787))
	assert.Equal(t, "127.0.0.1:8787", AppendDefaultPort("127.0.0.1", 8787))
	assert.Equal(t, "yaklang.io:8787", AppendDefaultPort("yaklang.io", 8787))
}

func TestMUSSPASS_StringGlobArrayContains(t *testing.T) {
	assert.Equal(t, true, StringGlobArrayContains([]string{"/api/push?pass=*"}, "localhost/api/push?pass=123"))
	assert.Equal(t, true, StringGlobArrayContains([]string{"/api/push?pass=*&abc=123"}, "localhost/api/push?pass=123&abc=123"))
}
