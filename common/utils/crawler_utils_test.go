package utils

import (
	"github.com/k0kubun/pp"
	"testing"
)

func TestMapQueryToString(t *testing.T) {
	expects := map[string]map[string][]string{
		"key1=1":                                {"key1": {"1"}},
		"key2=2&key2=4&key=1":                   {"key": {"1"}, "key2": {"2", "4"}},
		"a=asdf&kaa25%23=123132%27&kaa25%23=ss": {"kaa25#": {"123132'", "ss"}, "a": {"asdf"}},
	} // map[result]params

	for result, params := range expects {
		ret := MapQueryToString(params)
		if result != ret {
			pp.ColoringEnabled = false
			t.Logf("params: %s value: %s expect %s", pp.Sprint(params), ret, result)
			t.Fail()
		}
	}
}

func TestDomainToURLFilter(t *testing.T) {
	trueResults := map[string][]string{
		"*.baidu.com": {
			"http://asd.baidu.com",
			"https://a.baidu.com",
			"http://a.s.s.sa.baidu.com/asdfasdfasd",
		},
		"*baidu*": {
			"http://asd.baidu.com/aa",
			"https://baidu.com",
		},
		"baidu.com/a/b/*": {
			"http://baidu.com/a/b/asss",
			"http://baidu.com/a/b/assss",
			"http://baidu.com/a/b/asa.a/a/a/a/aass",
		},
		"*.baidu.com/a/b/*": {
			"http://xxx.baidu.com/a/b/asss",
			"http://a.baidu.com/a/b/assss",
			"http://asdf.baidu.com/a/b/asa.a/a/a/a/aass",
		},
		"baidu.com/a*": {
			"http://baidu.com/a",
			"http://baidu.com/ab/sd",
			"http://baidu.com/a/c",
		},
		"baidu.com": {"http://baidu.com/asdasd", "https://baidu.com"},
	}

	falseResults := map[string][]string{
		"*.baidu.com": {
			"http://baidu.com",
			"http://baidu.com",
			"ftp://aaa.baidu.com",
			"https://asdbaidu.com",
			"http://c.caidu.com/?ref=http://a.baidu.com",
			"http://c.caidu.com?ref=http://a.baidu.com",
		},
		"baidu.com/a*": {
			"http://baidu.com/ba",
			"http://baidu.com/nab/sd",
			"http://baidu.com/casdfa/c",
		},
		"*.baidu.com/a/b/*": {
			"http://baidu.com/a/asss",
			"http://abaidu.com/a/b/assss",
			"http://asdfbaidu.com/a/b/asa.a/a/a/a/aass",
		},
		"baidu.com": {
			"http://a.baidu.com",
			"https://a.baidu.com",
		},
	}

	for pattern, datas := range trueResults {
		re, err := DomainToURLFilter(pattern)
		if err != nil {
			t.Logf("compiled failed: %s", err)
			t.FailNow()
		}
		for _, data := range datas {
			if !re.MatchString(data) {
				t.Logf("[%s] match string false(unexpected): %s using: %s", pattern, data, re.String())
				t.FailNow()
			} else {
				t.Logf("[%s] match %s: pass", pattern, data)
			}
		}
	}

	for pattern, datas := range falseResults {
		re, err := DomainToURLFilter(pattern)
		if err != nil {
			t.Logf("compiled failed: %s", err)
			t.FailNow()
		}
		for _, data := range datas {
			if re.MatchString(data) {
				t.Logf("[%s] match string true(unexpected): %s using: %s", pattern, data, re.String())
				t.FailNow()
			} else {
				t.Logf("[%s] no match for %s pass", pattern, data)
			}
		}
	}
}
