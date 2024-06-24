package syntaxflow

import (
	"fmt"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func check(t *testing.T, code string, op sfvm.SFVMOpCode) {
	{
		lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(code))
		lexer.RemoveErrorListeners()
		astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
		astParser.RemoveErrorListeners()
		flow := astParser.Flow()
		fmt.Printf("%v\n", flow.ToStringTree(astParser.RuleNames, astParser))
	}

	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(code)
	assert.NoError(t, err)

	match := false
	vm.Show()
	for _, c := range frame.Codes {
		if op == c.OpCode {
			match = true
			return
		}
	}
	if !match {
		t.Fatalf("opcode %v not found", op)
	}
}

func TestOpcode(t *testing.T) {
	// search
	t.Run("search exact", func(t *testing.T) {
		check(t, `aaa as $target`, sfvm.OpPushSearchExact)
	})
	t.Run("search glob", func(t *testing.T) {
		check(t, `a* as $target`, sfvm.OpPushSearchGlob)
	})
	t.Run("search regex", func(t *testing.T) {
		check(t, `/(a1|b1)/ as $target`, sfvm.OpPushSearchRegexp)
	})
	t.Run("get ref", func(t *testing.T) {
		check(t, `$a.f as $target`, sfvm.OpPushSearchExact)
	})

	// check

	t.Run("check statement only", func(t *testing.T) {
		check(t, `check $a`, sfvm.OpCheckParams)
	})
	t.Run("check statement with then", func(t *testing.T) {
		check(t, `check $a then "pass"`, sfvm.OpCheckParams)
	})
	t.Run("check statement with else", func(t *testing.T) {
		check(t, `check $a else "fail"`, sfvm.OpCheckParams)
	})
	t.Run("check statement full", func(t *testing.T) {
		check(t, `check $a then "pass" else "fail"`, sfvm.OpCheckParams)
	})
	// alert
	t.Run("echo statement", func(t *testing.T) {
		check(t, `alert $a`, sfvm.OpAlert)
	})

	// variable
	t.Run("update ref", func(t *testing.T) {
		check(t, `a as $target`, sfvm.OpUpdateRef)
	})
	t.Run("get ref", func(t *testing.T) {
		check(t, `$a.f as $target`, sfvm.OpNewRef)
	})

	// function call
	t.Run("check function call", func(t *testing.T) {
		check(t, `a() as $target`, sfvm.OpGetCall)
	})
	t.Run("check all argument", func(t *testing.T) {
		check(t, `a(*  as $target)`, sfvm.OpGetAllCallArgs)
	})
	t.Run("check single argument", func(t *testing.T) {
		check(t, `a(*  as $target, )`, sfvm.OpGetCallArgs)
	})

	// condition
	t.Run("opcode condition", func(t *testing.T) {
		check(t, `a?{opcode: const} as $target`, sfvm.OpCompareOpcode)
	})
	t.Run("string condition", func(t *testing.T) {
		check(t, `a?{have: const} as $target`, sfvm.OpCompareString)
	})
	t.Run("bang condition", func(t *testing.T) {
		check(t, `a?{!(have: const)} as $target`, sfvm.OpLogicBang)
	})
	t.Run("logical condition", func(t *testing.T) {
		check(t, `a?{(have: const) || (opcode: const)} as $target`, sfvm.OpLogicOr)
	})

	// use def
	t.Run("get users", func(t *testing.T) {
		check(t, `a -> * as $target`, sfvm.OpGetUsers)
	})
	t.Run("get users empty", func(t *testing.T) {
		check(t, `a ->  as $target`, sfvm.OpGetUsers)
	})

	t.Run("get def", func(t *testing.T) {
		check(t, `a #> * as $target`, sfvm.OpGetDefs)
	})
	t.Run("get def empty", func(t *testing.T) {
		check(t, `a #>  as $target`, sfvm.OpGetDefs)
	})

	t.Run("get users with config", func(t *testing.T) {
		check(t, `a -{depth: 1}-> * as $target`, sfvm.OpGetBottomUsers)
	})
	t.Run("get users empty with config", func(t *testing.T) {
		check(t, `a -{depth: 1}->  as $target`, sfvm.OpGetBottomUsers)
	})

	t.Run("get def with config", func(t *testing.T) {
		check(t, `a #{depth: 1}-> * as $target`, sfvm.OpGetTopDefs)
	})
	t.Run("get def empty with config", func(t *testing.T) {
		check(t, `a #{depth: 1}->  as $target`, sfvm.OpGetTopDefs)
	})

	// example
	t.Run("example 1", func(t *testing.T) {
		check(t, `
		a* as $target1
		$target1?{opcode: const} as $target2
		`, sfvm.OpPushSearchGlob)
	})

	t.Run("example 2 ", func(t *testing.T) {
		check(t, `
		f(* as $obj)
		$obj.a as $a
		`, sfvm.OpNewRef,
		)
	})

	t.Run("example 3", func(t *testing.T) {
		check(t, `
		$a.f 
		f.b()
		`, sfvm.OpCheckStackTop)
	})
}
