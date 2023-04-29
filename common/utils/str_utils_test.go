package utils

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"yaklang/common/log"
	"testing"
	"time"
)

func TestRemoveUnprintableChars(t *testing.T) {
	cases := map[string]string{
		"\x00W\xffO\x00R\x00K": "WORK",
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

func TestUrlJoin(t *testing.T) {
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

func TestParseStringToHostPort(t *testing.T) {
	type Result struct {
		Host string
		Port int
	}
	cases := map[string]Result{
		"http://baidu.com":     {Host: "baidu.com", Port: 80},
		"https://baidu.com":    {Host: "baidu.com", Port: 443},
		"https://baidu.com:88": {Host: "baidu.com", Port: 88},
		"http://baidu.com:88":  {Host: "baidu.com", Port: 88},
		"1.2.3.4:1":            {Host: "1.2.3.4", Port: 1},
		"baidu.com:1":          {Host: "baidu.com", Port: 1},
	}

	falseCases := []string{
		"baidu.com", "1.2.3.5", "[1:123:123:123]",
	}

	for raw, result := range cases {
		host, port, err := ParseStringToHostPort(raw)
		if err != nil {
			t.Errorf("parse %s failed: %s", raw, err)
			t.FailNow()
		}

		if result.Host == host && result.Port == port {
			continue
		} else {
			t.Errorf("parse result failed: %s expect: %s:%v actually: %s:%v", raw, result.Host, result.Port,
				host, port)
			t.FailNow()
		}
	}

	for _, c := range falseCases {
		_, _, err := ParseStringToHostPort(c)
		if err != nil {

		} else {
			t.Errorf("%s should failed now", c)
			t.FailNow()
		}
	}
}

func TestSliceGroup(t *testing.T) {
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

func TestGetFirstIPFromHostWithTimeout(t *testing.T) {
	ip := GetFirstIPFromHostWithTimeout(5*time.Second, "baidu.com", nil)
	spew.Dump(ip)
}

func TestParseStringToCClassHosts(t *testing.T) {
	spew.Dump(ParseStringToCClassHosts("192.168.1.2,baidu.com,192.168.1.22,www.uestc.edu.cn"))
}

func TestGetIPFromHostWithContextAndDNSServers(t *testing.T) {
	err := GetIPFromHostWithContextAndDNSServers(FloatSecondDuration(1), "xqfunds.com", nil, func(domain string) bool {
		println(domain)
		return true
	})
	if err != nil {
		_ = err
	}
}
