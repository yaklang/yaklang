package yak

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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

func TestBuildInMethod(t *testing.T) {
	code := `
	a = [] 
	a.Append(1)
	println(a)
	`

	prog := ssaapi.Parse(code, plugin_type_analyzer.GetPluginSSAOpt("yak")...)
	if prog.IsNil() {
		t.Fatal("parse error")
	}
	users := prog.Ref("a").GetUsers()
	if len(users) != 2 {
		t.Fatal("user length error : ", users.String())
	}
}

func TestSSARuleMustPassRiskOption(t *testing.T) {
	t.Run("risk with nothing", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc"
		)
			`, []string{
			rules.ErrorRiskCheck(),
		})
	})
	t.Run("risk with cve", func(t *testing.T) {
		check(t, `
	risk.NewRisk(
		"abc", 
		risk.cve("abc")
	)
		`, []string{})
	})
	t.Run("risk with description and solution", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.solution("abc"),
			risk.description("abc")
		)
		`, []string{})
	})
	t.Run("risk with description", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.description("abc")
		)
			`, []string{
			rules.ErrorRiskCheck(),
		})
	})
	t.Run("risk with solution", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.solution("abc")
		)
			`, []string{
			rules.ErrorRiskCheck(),
		})
	})
	t.Run("risk with all", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.solution("abc"),
			risk.description("abc"),
			risk.cve("abc")
		)
			`, []string{})
	})

}

func TestSSARuleMustPassRiskCreate(t *testing.T) {
	t.Run("risk create not use", func(t *testing.T) {
		check(t, `
		risk.CreateRisk(
			"abc", 
			risk.cve("abc")
		)
			`, []string{
			rules.ErrorRiskCreateNotSave(),
		})
	})

	t.Run("risk create used but not saved", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc", 
			risk.cve("abc")
		)
		println(r)
			`, []string{
			rules.ErrorRiskCreateNotSave(),
		})
	})

	t.Run("risk create saved", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc", 
			risk.cve("abc")
		)
		risk.Save(r)
			`, []string{})
	})

	t.Run("risk create saved and used", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc",
			risk.cve("abc")
		)
		println(r)
		risk.Save(r)
			`, []string{})
	})

}

func checkWithType(t *testing.T, code, typ string, want []string) {
	got := AnalyzeStaticYaklangWithType(code, typ)
	if len(got) != len(want) {
		t.Fatalf("static analyzer error length error want(%d) vs got(%d)", len(want), len(got))
	}

	for i := range got {
		if got[i].Message != want[i] {
			t.Fatalf("static analyzer message error want(%s) vs got(%s)", want[i], got[i].Message)
		}
	}
}

func TestSSARuleMustPassCliDisable(t *testing.T) {
	t.Run("cli disable in mitm", func(t *testing.T) {
		checkWithType(t, `
		domains = cli.String("domains")
		cli.check()
			`,
			"mitm",
			[]string{rules.ErrorDisableCLi(), rules.ErrorDisableCLi()})
	})

	t.Run("cli disable in yak", func(t *testing.T) {
		checkWithType(t, `
		domains = cli.String("domains")
		cli.check()
			`,
			"yak",
			[]string{})
	})
}

func TestSSARuleMustPassMitmDisable(t *testing.T) {
	t.Run("test pack in mitm main", func(t *testing.T) {
		checkWithType(t, `
		r = risk.CreateRisk(
			"abc",
			risk.cve("abc")
		)
		println(r)
		risk.Save(r)
			`,
			"mitm",
			[]string{
				rules.MITMNotSupport("risk.CreateRisk"),
				rules.MITMNotSupport("risk.cve"),
				rules.MITMNotSupport("risk.Save"),
			})
	})

	t.Run("test in mitm func", func(t *testing.T) {
		checkWithType(t, `
		() => {
			r = risk.CreateRisk(
				"abc",
				risk.cve("abc")
			)
			println(r)
			risk.Save(r)
		}
			`,
			"mitm",
			[]string{})
	})

	t.Run("test in other plugin type", func(t *testing.T) {
		checkWithType(t, `
		r = risk.CreateRisk(
			"abc",
			risk.cve("abc")
		)
		println(r)
		risk.Save(r)
			`,
			"yak",
			[]string{})
	})
}
