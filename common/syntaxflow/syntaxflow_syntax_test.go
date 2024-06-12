package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func TestSyntaxInOne(t *testing.T) {
	for _, i := range []string{
		"$",
		"*",
		".*",
		"a.*",
		"exec",    // Ref
		".member", // Field
		".*exec*",
		"*exec",
		"exe*c",
		"/$reexc/",
		"./$reexc/",
		"a[1]",
		"a.b",
		"c.d",
		"a[1]",
		"b?{!1}",
		"b?{>1}",
		"b?{!/abc/}",
		"f()",
		"f() as $a",
		"f(*,)",
		"f(* as $arg1,)",
		"f(,*)",
		"f(*,*)",
		"f(*,*,)",
		"f(,*,*,)",
		"f(,*,*)",
		"runtime.exec",
		"runtime.exec()",
		"runtime.exec(,,)",
		"/(?i)runtime/.exec(,,,#>*exec)",
		"exec as $rough",
		"/(?i)runtime/.exec(,,,#>*exec) as $a",
		"/(?i)runtime/.exec(,,,#>*exec as $b) as $a",
		"/(?i)runtime/.exec(,,,#>*exec,,) as $a",
		"/(?i)runtime/.exec(,,,#>*exec as $ccc,,) as $a",
		"/(?i)runtime/.exec(,,,#>*exec,,,) as $a",
		"a->b",
		"a#>b",
		"a-->b",
		"a#->b",
		"a->b->c",
		"a#>b#>c",
		"a-->b.exec()-->c",
		"a#->b.exec()#->c",
		"a-{}->b",
		"a#{}->b",
		"system(#>get)",
		"system(#>*)",
		"system(#>a*get)",
		"a*",
		"*a",
		"*",
		"system(#{}*a)",
		"system(#{}*)",
		"a-{depth:1}->b",
		"a#{depth:1}->b",
		"a-{depth:1, " + "\nkey:value}->b",
		"a#{depth:1, " + "\nkey:value}->b",
		"a-{depth:1, " + "\nkey:value,}->b",
		"a#{depth:1, " + "\nkey:value,}->b",
		`*.mem as $a
		$a.exec() as $exec
		`,
		`
		a -{
			until:` + " `*->a`" + `
		}-> *`,
		"a -<abc>- b",
		"a -<abc{depth: 1}>- b",
		"a -<abc(depth: 1, asdf: `a -{}-> *`)>- b",
		"a ->",
		"a #>",
		"a #->",
		"a #{}->",
		"a #{depth: 1}->",
		"a -->",
		"a -{depth: 1}->",
		"a -{}->",
		"a -> as $b",
		"a #> as $b",
		"a #-> as $b",
		"a --> as $b",
		`a;a;a;a;;;;;`,
		"assert $abc",
		"assert $abc;;;;;",
		"assert $abc then Finished else BAD",
		"assert $abc else BAD",
		"assert $abc then Finished",
		"assert $abc then Finished else BAD;;;; desc{a: b};; desc{title: SprintChecking}",
		"desc(a: b, c: eee, e,e,e,e)",
		"desc(a: b, c: eee, e,e,e,e);;;;assert $abc then GOOD else BAD",
	} {
		vm := sfvm.NewSyntaxFlowVirtualMachine().Debug(true)
		err := vm.Compile(i)
		if err != nil {
			t.Fatalf("syntax failed: %#v, reason: %v", i, err)
		}
	}
}
