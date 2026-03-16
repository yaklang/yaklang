package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_NestedAnchorScope_StacksAnchorBase(t *testing.T) {
	code := `<?php
function foo($x) { return $x; }
function bar($x) { return $x; }

foo(1);
foo(2);
bar(3);
?>`

	// Mirrors the SFVM design-doc motivating example:
	//   URL?{<getCall>?{.openStream()}}
	//
	// Outer scope source is (foo,bar) => 2 slots.
	// Inner scope source is <getCall> => 3 callsite slots (foo, foo, bar).
	//
	// Nested anchor scopes must stack anchorBase so inner local bits do NOT overlap with
	// outer slot bits, otherwise the "bar" match can incorrectly map back to a "foo" slot
	// (or trip an out-of-scope anchor-bit error).
	rule := `/^(foo|bar)$/?{opcode:function}?{<getCall>?{<getCallee><name><regexp('^bar$')>}} as $out`

	ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
		"out": {"Function-bar"},
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}
