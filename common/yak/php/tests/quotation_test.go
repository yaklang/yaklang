package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestDouble_quotation(t *testing.T) {
	t.Run("custom quotation", func(t *testing.T) {
		code := `<?php
$a = "1";

println("$a");
`
		ssatest.CheckPrintlnValue(code, []string{`"1"`}, t)
	})
	t.Run("quotation and contains", func(t *testing.T) {
		code := `<?php
$a = "1";
println("$a"."a"."b");`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`,
			map[string][]string{"param": {`"1"`, `"a"`, `"b"`}},
			ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("test exec quotation $", func(t *testing.T) {
		code := `<?php
	function setVideoImg($file){
		exec("$file");
	}
	setVideoImg("whoami");
`
		ssatest.CheckSyntaxFlow(t, code,
			`
exec(* #-> * as $param)
`,
			map[string][]string{"param": {`"whoami"`}},
			ssaapi.WithLanguage(ssaconfig.PHP))
	})
}
func TestStringPart(t *testing.T) {
	t.Run("double quote", func(t *testing.T) {
		code := `<?php
$a=1;
$b=2;
println("$a+$b");`
		ssatest.CheckPrintlnValue(code, []string{`add(add(1, "+"), 2)`}, t)
	})
	t.Run("signal quote", func(t *testing.T) {
		code := `<?php
$a=1;
$b=2;
println('$a+$b');`
		ssatest.CheckPrintlnValue(code, []string{"\"$a+$b\""}, t)
	})
}
