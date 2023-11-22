package yak

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	yaklangspec "github.com/yaklang/yaklang/common/yak/yaklang/spec"
)

func TestCounter(t *testing.T) {
	_ = yaklang.New()
	count := 0
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

func TestSSARuleMustPassYakCliParamName(t *testing.T) {
	t.Run("cli same paramName", func(t *testing.T) {
		check(t, `
cli.String("a")
cli.String("a")
cli.check()
	`, []string{rules.ErrorStrSameParamName("a", 2)})
	})

	t.Run("cli invalid paramName", func(t *testing.T) {
		check(t, `
cli.String("!@#")
cli.String("a")
cli.check()
	`, []string{rules.ErrorStrInvalidParamName("!@#")})
	})
}

func TestSSARuleMustPassYakCliCheck(t *testing.T) {
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
			rules.ErrorStrNotCallCliCheck(),
		})
	})
	t.Run("cli not check in last", func(t *testing.T) {
		check(t, `
		cli.String("a")
		cli.check()
		cli.String("b")
			`, []string{
			rules.ErrorStrNotCallCliCheck(),
		})
	})
	t.Run("not cli function", func(t *testing.T) {
		check(t, `
			println("aaaa")
			`, []string{})
	})
}
