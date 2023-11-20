package yak

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	yaklangspec "github.com/yaklang/yaklang/common/yak/yaklang/spec"
)

func TestCounter(t *testing.T) {
	_ = yaklang.New()
	var count = 0
	for key, value := range yaklangspec.Fntable {
		val, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		switch key {
		case "env", "sync", "codec",
			"mmdb", "crawler", "mitm", "tls",
			"xhtml", "cli", "fuzz", "httpool", "http", "httpserver",
			"dictutil", "tools", "synscan", "tcp", "servicescan",
			"subdomain", "exec", "brute", "dns", "ping", "spacengine",
			"json", "dyn", "nuclei", "yakit", "jwt", "java", "poc", "csrf",
			"risk", "report", "xpath", "hook", "yso", "facades", "t3",
			"iiop", "js", "smb", "ldap", "redis", "rdp", "crawlerx":
			count += len(val)
		}
	}
	println(count)
}

func check(t *testing.T, code string, want []string) {
	got := AnalyzeStaticYaklang(code)
	if len(got) != len(want) {
		t.Fatalf("static analyzer error length error want(%d) vs got(%d)", len(want), len(got))
	}

	for i := range got {
		if got[i].Message != want[i] {
			t.Fatalf("static analyzer message error want(%s) vs got(%s)", want[i], got[i].Message)
		}
	}
}

func TestSSARuleMustPassYakCliParameter(t *testing.T) {

	t.Run("cli with check", func(t *testing.T) {
		check(t, `
	cli.String("a")
	cli.check()
		`, []string{})
	})

	t.Run("cli not check", func(t *testing.T) {
		check(t, `
	cli.String("a")
		`, []string{
			rules.NotCallCliCheck(),
		})
	})

	t.Run("cli not check in last", func(t *testing.T) {
		check(t, `
	cli.String("a")
	cli.check()
	cli.String("b")
		`, []string{
			rules.NotCallCliCheck(),
		})
	})

	t.Run("not cli function", func(t *testing.T) {
		check(t, `
		println("aaaa")
		`, []string{})
	})
}
