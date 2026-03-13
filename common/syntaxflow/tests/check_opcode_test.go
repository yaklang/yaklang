package syntaxflow

import (
	"fmt"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func compileFrame(t *testing.T, code string) *sfvm.SFFrame {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(code)
	require.NoError(t, err)
	return frame
}

func checkOpcodeSequence(t *testing.T, code string, expected ...sfvm.SFVMOpCode) {
	frame := compileFrame(t, code)
	actual := make([]sfvm.SFVMOpCode, 0, len(frame.Codes))
	for _, c := range frame.Codes {
		actual = append(actual, c.OpCode)
	}
	if len(expected) == 0 {
		return
	}

	for i := 0; i+len(expected) <= len(actual); i++ {
		matched := true
		for j, op := range expected {
			if actual[i+j] != op {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}

	t.Fatalf("opcode sequence not found.\ncode: %s\nexpected: %v\nactual: %v", code, expected, actual)
}

func checkNo(t *testing.T, code string, op sfvm.SFVMOpCode) {
	if checkContain(t, code, op) {
		t.Fatalf("found %v", op)
	}
}
func check(t *testing.T, code string, op sfvm.SFVMOpCode) {
	if !checkContain(t, code, op) {
		t.Fatalf("not found %v", op)
	}
}

func checkContain(t *testing.T, code string, op sfvm.SFVMOpCode) bool {
	match := false
	checkOpcode(t, code, op, func(s *sfvm.SFI) {
		match = true
	})
	return match
}
func checkOpcode(t *testing.T, code string, op sfvm.SFVMOpCode, handler func(*sfvm.SFI)) {
	{
		fmt.Printf("code: %v\n", code)
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

	vm.Show()
	for _, c := range frame.Codes {
		if op == c.OpCode {
			handler(c)
		}
	}
}

func TestOpcode(t *testing.T) {
	// comment
	t.Run("comment", func(t *testing.T) {
		checkNo(t, `// a `, sfvm.OpPushSearchExact)
	})
	t.Run("comment with keywords", func(t *testing.T) {
		checkNo(t, `// // // // a as $aaaa`, sfvm.OpUpdateRef)
		checkNo(t, `// // // a as $aaaa`, sfvm.OpNewRef)
		checkNo(t, `// a as $aaaa`, sfvm.OpUpdateRef)
		check(t, `
		// a as $aaaa
		a 
		`, sfvm.OpPushSearchExact)
		checkNo(t, `
		// a as $aaaa
		a 
		`, sfvm.OpUpdateRef)
		checkNo(t, `//check $aaae `, sfvm.OpCheckParams)
	})

	//  description
	// t.Run("description", func(t *testing.T) {
	// 	check(t, `desc(a: b)`, sfvm.OpAddDescription)
	// })
	// t.Run("description no :", func(t *testing.T) {
	// 	check(t, `desc("xxxx")`, sfvm.OpAddDescription)
	// })

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

	// file filter
	t.Run("file filter", func(t *testing.T) {
		check(t, `${application.properties}.re(/datasource.url=(.*)/) as $target`, sfvm.OpFileFilterReg)
		check(t, `${application.properties}.json("") as $target`, sfvm.OpFileFilterJsonPath)
		check(t, `${application.properties}.xpath("//path/a/b/@c") as $target`, sfvm.OpFileFilterXpath)
	})

	t.Run("file filter with variable", func(t *testing.T) {
		check(t, `${application.properties}.re(/datasource.url=(.*)/) as $target`, sfvm.OpUpdateRef)
		check(t, `${application.properties}.json("") as $target`, sfvm.OpUpdateRef)
		check(t, `${application.properties}.xpath("//path/a/b/@c") as $target`, sfvm.OpUpdateRef)
	})

	t.Run("file filter check for input(program)", func(t *testing.T) {
		check(t, `${application.properties}.re(/datasource.url=(.*)/) as $target`, sfvm.OpCheckStackTop)
		check(t, `${application.properties}.json("") as $target`, sfvm.OpCheckStackTop)
		check(t, `${application.properties}.xpath("//path/a/b/@c") as $target`, sfvm.OpCheckStackTop)
	})

	// variable
	t.Run("update ref", func(t *testing.T) {
		check(t, `a as $target`, sfvm.OpUpdateRef)
	})
	t.Run("get ref", func(t *testing.T) {
		check(t, `$a.f as $target`, sfvm.OpNewRef)
	})

	// check expr enter
	t.Run("enter expr with variable", func(t *testing.T) {
		check(t, `$a.b`, sfvm.OpEnterStatement)
	})
	t.Run("enter expr with expr", func(t *testing.T) {
		check(t, `a.b`, sfvm.OpExitStatement)
	})

	// function call
	t.Run("check function call", func(t *testing.T) {
		check(t, `a() as $target`, sfvm.OpGetCall)
	})
	t.Run("check all argument", func(t *testing.T) {
		check(t, `a(*  as $target)`, sfvm.OpGetCallArgs)
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
	t.Run("filter condition without iter loop", func(t *testing.T) {
		code := `a?{.b} as $target`
		checkOpcodeSequence(t, code,
			sfvm.OpDuplicate,
			sfvm.OpEnterStatement,
			sfvm.OpPushSearchExact,
			sfvm.OpExitStatement,
			sfvm.OpFilter,
			sfvm.OpCondition,
		)
		checkNo(t, code, sfvm.OpEmptyCompare)
	})
	t.Run("logical filter should build mask then condition", func(t *testing.T) {
		code := `a?{.b && .c} as $target`
		checkOpcodeSequence(t, code,
			sfvm.OpAnchorScopeStart,
			sfvm.OpDuplicate,
			sfvm.OpEnterStatement,
			sfvm.OpPushSearchExact,
			sfvm.OpExitStatement,
			sfvm.OpFilter,
			sfvm.OpAnchorScopeEnd,
			sfvm.OpAnchorScopeStart,
			sfvm.OpDuplicate,
			sfvm.OpEnterStatement,
			sfvm.OpPushSearchExact,
			sfvm.OpExitStatement,
			sfvm.OpFilter,
			sfvm.OpAnchorScopeEnd,
			sfvm.OpLogicAnd,
			sfvm.OpCondition,
		)
	})
	t.Run("mixed opcode-and-filter should keep stack balance", func(t *testing.T) {
		code := `a?{opcode:param && .b} as $target`
		checkOpcodeSequence(t, code,
			sfvm.OpAnchorScopeStart,
			sfvm.OpCompareOpcode,
			sfvm.OpAnchorScopeEnd,
			sfvm.OpAnchorScopeStart,
			sfvm.OpDuplicate,
			sfvm.OpEnterStatement,
			sfvm.OpPushSearchExact,
			sfvm.OpExitStatement,
			sfvm.OpFilter,
			sfvm.OpAnchorScopeEnd,
			sfvm.OpLogicAnd,
			sfvm.OpCondition,
		)
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

	t.Run("test OpFileFilterXpath 1", func(t *testing.T) {
		check(t, `
		${application.properties}.xpath(select: aaa)
		`, sfvm.OpFileFilterXpath)
	})

	t.Run("test OpFileFilterXpath 2", func(t *testing.T) {
		check(t, `
		${/xml$/}.xpath(select: aa)
		`, sfvm.OpFileFilterXpath)
	})

	t.Run("test OpFileFilterJsonPath 1 ", func(t *testing.T) {
		check(t, `
		${application.properties}.json(select: aaa)
		`, sfvm.OpFileFilterJsonPath)
	})

	t.Run("test OpFileFilterJsonPath 2 ", func(t *testing.T) {
		check(t, `
		${application.properties}.jsonpath(select: aaa)
		`, sfvm.OpFileFilterJsonPath)
	})

	t.Run("test OpFileFilterReg 1 ", func(t *testing.T) {
		check(t, `
		${application.properties}.re(select: aaa)
		`, sfvm.OpFileFilterReg)
	})

	t.Run("test OpFileFilterReg 2", func(t *testing.T) {
		check(t, `
		${application.properties}.regexp(select: aaa)
		`, sfvm.OpFileFilterReg)
	})

	t.Run("config heredoc", func(t *testing.T) {
		checkOpcode(t, `a #{
			hook: <<<HOOK
			*.a as $a
HOOK
		}->`, sfvm.OpGetTopDefs, func(s *sfvm.SFI) {
			require.Equal(t, 1, len(s.SyntaxFlowConfig))
			require.NotContains(t, s.SyntaxFlowConfig[0].Value, "HOOK")
			log.Infof("s: %v", s.SyntaxFlowConfig[0].Value)
		})
	})

	t.Run("config heredoc complex", func(t *testing.T) {
		checkOpcode(t, `a #{
			hook: <<<HOOK
			*.a as $a
			*-{
				until: <<<UNTIL
				*.b 
UNTIL
			}->
HOOK
		}->`, sfvm.OpGetTopDefs, func(s *sfvm.SFI) {
			require.Equal(t, 1, len(s.SyntaxFlowConfig))
			require.NotContains(t, s.SyntaxFlowConfig[0].Value, "HOOK")
			log.Infof("s: %v", s.SyntaxFlowConfig[0].Value)
		})
	})
}
