package test

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func init() { ssatest.SetLanguage(ssaconfig.BASH) }

// A real shell function has no argv[0] operand. This makes the existing
// CheckPrintln helpers observe $1 without a special production-only command.
const sink = "println() { :; }; "

func TestBashPrintlnBasic(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		want         []string
	}{
		{"assignment and quotes", `x=hi; println "pre${x}post"; println '$x'; println ""`, []string{`"prehipost"`, `"$x"`, `""`}},
		{"escaped words", `println a\ b; println "a\"b"; println $'a\tb'`, []string{`"a b"`, `"a\"b"`, `"a\tb"`}},
		{"empty and fallback", `x=before; x=; println "${x:-after}"; println "${x-after}"; println "${x:=set}"; println "$x"`, []string{`"after"`, `""`, `"set"`, `"set"`}},
		{"special parameters", `set -- 'one two' '' three; println "$#"; println "$1"; println "$2"; println "$*"; shift; println "$#"; println "$2"`, []string{`3`, `"one two"`, `""`, `"one two  three"`, `2`, `"three"`}},
		{"status", `false; println "$?"; ! false; println "$?"; x=alone; println "$?"; if false; then x=no; fi; println "$?"; f(){ return 4; }; f; println "$?"`, []string{`1`, `0`, `0`, `0`, `4`}},
		{"arrays and record fields", `a=(zero one [4]=four); a[1]=new; println "${a[0]}"; println "${a[1]}"; println "${a[4]}"; declare -A rec=([name]=blue [size]=wide); rec[name]=green; println "${rec[name]}"; println "${rec[size]}"`, []string{`"zero"`, `"new"`, `"four"`, `"green"`, `"wide"`}},
	} {
		t.Run(tc.name, func(t *testing.T) { ssatest.CheckPrintlnValue(sink+tc.script, tc.want, t) })
	}
}
