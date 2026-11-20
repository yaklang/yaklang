package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

var bashLanguage = ssaapi.WithLanguage(ssaconfig.BASH)

// TestBashSyntaxFlowFunctionParameters covers the callee parameter vector:
// call arguments, omitted positions, the positional count, and "$@" spread.
func TestBashSyntaxFlowFunctionParameters(t *testing.T) {
	t.Run("call argument reaches positional member", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t,
			`remove() { rm -rf "$1"; }; remove /`,
			`remove(* as $arg)`,
			map[string][]string{
				"arg": {`"/"`},
			},
			bashLanguage,
		)
	})

	t.Run("second positional member", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`
f() {
  println "$2"
}
f one two
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"two"`},
			},
			bashLanguage,
		)
	})

	t.Run("omitted argument is empty", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`
f() {
  println "$1"
}
f
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`""`},
			},
			bashLanguage,
		)
	})

	t.Run("positional count", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`
f() {
  println "$#"
}
f a b
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`2`},
			},
			bashLanguage,
		)
	})

	t.Run("spread arguments", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`
set -- a b
f() {
  println "$1"
  println "$2"
}
f "$@"
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"a"`, `"b"`},
			},
			bashLanguage,
		)
	})
}

// TestBashSyntaxFlowCrossProcedure covers values that travel through one or
// more shell functions, including the original review example:
//
//	remove() { rm -rf "$1"; }; remove /
func TestBashSyntaxFlowCrossProcedure(t *testing.T) {
	t.Run("helper forwards root to rm", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t,
			`remove() { rm -rf "$1"; }; remove /`,
			`rm(*<slice(start=1)> #-> as $args)
$args?{any: /^\/$/} as $hit`,
			map[string][]string{
				"args": {`"-rf"`, `"/"`},
				"hit":  {`"/"`},
			},
			bashLanguage,
		)
	})

	t.Run("nested helper forwards parameter", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`
a() { b "$1"; }
b() { println "$1"; }
a payload
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"payload"`},
			},
			bashLanguage,
		)
	})

	t.Run("call before definition", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`
f secret
f() { println "$1"; }
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"secret"`},
			},
			bashLanguage,
		)
	})

	t.Run("free value", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`
root=/srv
f() { println "$root"; }
f
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"/srv"`},
			},
			bashLanguage,
		)
	})

	t.Run("loop unrolling keeps each argument", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`for p in / /tmp; do rm -rf "$p"; done`,
			`rm(*<slice(start=1)> #-> as $args)`,
			map[string][]string{
				"args": {`"/"`, `"/tmp"`},
			},
			bashLanguage,
		)
	})
}
