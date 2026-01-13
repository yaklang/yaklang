package syntaxflow

import (
	"fmt"
	"strings"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func checkNo(t *testing.T, code string, visit func(*sfvm.SyntaxFlowVisitor)) {
	visitor := sfvm.NewSyntaxFlowVisitor()
	if visit != nil {
		visit(visitor)
	}
	checkEx(t, code, visitor.GetCodes(), false)
}

func check(t *testing.T, code string, visit func(*sfvm.SyntaxFlowVisitor)) {
	visitor := sfvm.NewSyntaxFlowVisitor()
	visit(visitor)
	checkEx(t, code, visitor.GetCodes(), true)
}

func checkEx(t *testing.T, code string, insts []*sfvm.SFI, wantMatch bool) {
	require.Greater(t, len(insts), 0)
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
	require.NoError(t, err)
	codes := frame.Codes

	require.Greater(t, len(codes), 0)
	require.GreaterOrEqual(t, len(codes), len(insts))
	// match continue sequence of op in codes

	code_string := ""
	for _, c := range frame.Codes {
		code_string += c.String()
	}

	log.Infof("code_string: %s", code_string)
	// check if contain all insts
	match := true
	for _, inst := range insts {
		index := strings.Index(code_string, inst.String())
		if index == -1 {
			log.Infof("inst not found: %s in %s at index %d", inst.String(), code_string, index)
			match = false
			break
		}
		code_string = code_string[index+len(inst.String()):]
	}

	if wantMatch {
		require.True(t, match)
	} else {
		require.False(t, match)
	}
}

func TestOpcode(t *testing.T) {
	// comment
	t.Run("comment", func(t *testing.T) {
		checkNo(t, `// a `, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
		})
	})
	t.Run("comment with keywords", func(t *testing.T) {
		checkNo(t, `// // // // a as $aaaa`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitUpdate("aaaa")
		})
		checkNo(t, `// // // a as $aaaa`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitUpdate("aaaa")
		})
		checkNo(t, `// a as $aaaa`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitUpdate("aaaa")
		})
		check(t, `
		// a as $aaaa
		a 
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
		})
		checkNo(t, `
		// a as $aaaa
		a 
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitUpdate("a")
		})
		checkNo(t, `//check $aaae `, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckParam("aaae", "", "")
		})
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
		check(t, `aaa as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "aaa")
			sfv.EmitUpdate("target")
		})
		// check(t, `aaa as $target`, )
	})
	t.Run("search glob", func(t *testing.T) {
		check(t, `a* as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchGlob(sfvm.NameMatch, "a*")
			sfv.EmitUpdate("target")
		})
	})
	t.Run("search regex", func(t *testing.T) {
		check(t, `/(a1|b1)/ as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchRegexp(sfvm.NameMatch, "(a1|b1)")
			sfv.EmitUpdate("target")
		})
	})
	t.Run("get ref", func(t *testing.T) {
		check(t, `$a.f as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitNewRef("a")
			sfv.EmitSearchExact(sfvm.KeyMatch, "f")
			sfv.EmitUpdate("target")
		})
	})

	// check

	t.Run("check statement only", func(t *testing.T) {
		check(t, `check $a`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckParam("a", "", "")
		})
	})
	t.Run("check statement with then", func(t *testing.T) {
		check(t, `check $a then "pass"`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckParam("a", "pass", "")
		})
	})
	t.Run("check statement with else", func(t *testing.T) {
		check(t, `check $a else "fail"`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckParam("a", "", "fail")
		})
	})
	t.Run("check statement full", func(t *testing.T) {
		check(t, `check $a then "pass" else "fail"`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckParam("a", "pass", "fail")
		})
	})
	// alert
	t.Run("echo statement", func(t *testing.T) {
		check(t, `alert $a`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitAlert("a")
		})
	})

	// file filter
	t.Run("file filter", func(t *testing.T) {
		check(t, `${application.properties}.re(/datasource.url=(.*)/) as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterReg("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
		check(t, `${application.properties}.json("") as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterJsonPath("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
		check(t, `${application.properties}.xpath("//path/a/b/@c") as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterXpath("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
	})

	t.Run("file filter with variable", func(t *testing.T) {
		check(t, `${application.properties}.re(/datasource.url=(.*)/) as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterReg("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
		check(t, `${application.properties}.json("") as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterJsonPath("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
		check(t, `${application.properties}.xpath("//path/a/b/@c") as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterXpath("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
	})

	t.Run("file filter check for input(program)", func(t *testing.T) {
		check(t, `${application.properties}.re(/datasource.url=(.*)/) as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterReg("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
		check(t, `${application.properties}.json("") as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterJsonPath("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
		check(t, `${application.properties}.xpath("//path/a/b/@c") as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterXpath("application.properties", nil, nil)
			sfv.EmitUpdate("target")
		})
	})

	// variable
	t.Run("update ref", func(t *testing.T) {
		check(t, `a as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitUpdate("target")
		})
	})
	t.Run("get ref", func(t *testing.T) {
		check(t, `$a.f as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitNewRef("a")
			sfv.EmitSearchExact(sfvm.KeyMatch, "f")
			sfv.EmitUpdate("target")
		})
	})

	// check expr enter
	t.Run("enter expr with variable", func(t *testing.T) {
		check(t, `$a.b`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitNewRef("a")
			sfv.EmitEnterStatement()
			sfv.EmitSearchExact(sfvm.KeyMatch, "b")
		})
	})
	t.Run("enter expr with expr", func(t *testing.T) {
		check(t, `a.b`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			enter := sfv.EmitEnterStatement()
			sfv.EmitSearchExact(sfvm.KeyMatch, "b")
			sfv.EmitExitStatement(enter)
		})
	})

	// function call
	t.Run("check function call", func(t *testing.T) {
		check(t, `a() as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetCall()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("check all argument", func(t *testing.T) {
		check(t, `a(*  as $target)`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetCall()
			sfv.EmitPushCallArgs(0, true)
			sfv.EmitUpdate("target")
		})
	})
	t.Run("check single argument", func(t *testing.T) {
		check(t, `a(*  as $target, )`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetCall()
			sfv.EmitPushCallArgs(0, false)
			sfv.EmitUpdate("target")
		})
	})
}
func TestCondition(t *testing.T) {
	// condition
	t.Run("opcode condition", func(t *testing.T) {
		check(t, `a?{opcode: const} as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitCompareOpcode([]string{"const"})
			sfv.EmitOpToBool()
			sfv.EmitCondition()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("string condition", func(t *testing.T) {
		check(t, `a?{have: const} as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			res := []sfvm.ConditionItem{
				{
					Text:       "const",
					FilterMode: sfvm.ExactConditionFilter,
				},
			}
			sfv.EmitCompareString(res, sfvm.MatchHave)
			sfv.EmitCondition()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("bang condition", func(t *testing.T) {
		check(t, `a?{!(have: const)} as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitCompareString([]sfvm.ConditionItem{
				{
					Text:       "const",
					FilterMode: sfvm.ExactConditionFilter,
				},
			},
				sfvm.MatchHave)
			sfv.EmitOperator("!")
			sfv.EmitCondition()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("logical condition", func(t *testing.T) {
		check(t, `a?{(have: const) || (opcode: const)} as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitCompareString([]sfvm.ConditionItem{
				{
					Text:       "const",
					FilterMode: sfvm.ExactConditionFilter,
				},
			},
				sfvm.MatchHave)
			sfv.EmitCompareOpcode([]string{"const"})
			sfv.EmitOperator("||")
			sfv.EmitCondition()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("filter condition", func(t *testing.T) {
		check(t, `a?{.b()} as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			// {{{
			sfv.EmitDuplicate()
			sfv.EmitSearchExact(sfvm.KeyMatch, "b")
			sfv.EmitGetCall()
			sfv.EmitOpToBool()  // got condition stack result
			sfv.EmitCondition() // get result
			// }}}
			sfv.EmitUpdate("target")
		})
	})
	t.Run("filter with logical", func(t *testing.T) {
		check(t, `a?{.b() && .c()} as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			// {{{
			sfv.EmitDuplicate()
			sfv.EmitSearchExact(sfvm.KeyMatch, "b")
			sfv.EmitGetCall()
			sfv.EmitOpToBool()
			// }}}

			// {{{
			sfv.EmitDuplicate()
			sfv.EmitSearchExact(sfvm.KeyMatch, "c")
			sfv.EmitGetCall()
			sfv.EmitOpToBool()
			// }}}
			sfv.EmitOperator("&&")
			sfv.EmitCondition()

			sfv.EmitUpdate("target")
		})
	})
}

func TestUseDef(t *testing.T) {

	// use def
	t.Run("get users", func(t *testing.T) {
		check(t, `a -> * as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetUsers()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("get users empty", func(t *testing.T) {
		check(t, `a ->  as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetUsers()
			sfv.EmitUpdate("target")
		})
	})

	t.Run("get def", func(t *testing.T) {
		check(t, `a #> * as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetDefs()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("get def empty", func(t *testing.T) {
		check(t, `a #>  as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetDefs()
			sfv.EmitUpdate("target")
		})
	})

	t.Run("get users with config", func(t *testing.T) {
		check(t, `a -{depth: 1}-> * as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetBottomUsers()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("get users empty with config", func(t *testing.T) {
		check(t, `a -{depth: 1}->  as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetBottomUsers()
			sfv.EmitUpdate("target")
		})
	})

	t.Run("get def with config", func(t *testing.T) {
		check(t, `a #{depth: 1}-> * as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetTopDefs()
			sfv.EmitUpdate("target")
		})
	})
	t.Run("get def empty with config", func(t *testing.T) {
		check(t, `a #{depth: 1}->  as $target`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "a")
			sfv.EmitGetTopDefs()
			sfv.EmitUpdate("target")
		})
	})
}

func TestExample(t *testing.T) {
	// example
	t.Run("example 1", func(t *testing.T) {
		check(t, `
		a* as $target1
		$target1?{opcode: const} as $target2
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchGlob(sfvm.NameMatch, "a*")
			sfv.EmitUpdate("target1")
			sfv.EmitNewRef("target1")
			sfv.EmitCompareOpcode([]string{"const"})
			sfv.EmitCondition()
			sfv.EmitUpdate("target2")
		})
	})

	t.Run("example 2 ", func(t *testing.T) {
		check(t, `
		f(* as $obj)
		$obj.a as $a
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitSearchExact(sfvm.NameMatch, "f")
			sfv.EmitGetCall()
			sfv.EmitPushCallArgs(0, true)
			sfv.EmitUpdate("obj")
			sfv.EmitNewRef("obj")
			sfv.EmitSearchExact(sfvm.KeyMatch, "a")
			sfv.EmitUpdate("a")
		})
	})

	t.Run("example 3", func(t *testing.T) {
		check(t, `
		$a.f 
		f.b()
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitNewRef("a")
			sfv.EmitSearchExact(sfvm.KeyMatch, "f")
			sfv.EmitCheckStackTop()
			sfv.EmitSearchExact(sfvm.NameMatch, "f")
			sfv.EmitEnterStatement()
			sfv.EmitSearchExact(sfvm.KeyMatch, "b")
			sfv.EmitGetCall()
		})
	})

	t.Run("test OpFileFilterXpath 1", func(t *testing.T) {
		check(t, `
		${application.properties}.xpath(select: aaa)
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterXpath("application.properties", nil, nil)
		})
	})

	t.Run("test OpFileFilterXpath 2", func(t *testing.T) {
		check(t, `
		${/xml$/}.xpath(select: aa)
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterXpath("/xml$/", nil, nil)
		})
	})

	t.Run("test OpFileFilterJsonPath 1 ", func(t *testing.T) {
		check(t, `
		${application.properties}.json(select: aaa)
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterJsonPath("application.properties", nil, nil)
		})
	})

	t.Run("test OpFileFilterJsonPath 2 ", func(t *testing.T) {
		check(t, `
		${application.properties}.jsonpath(select: aaa)
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterJsonPath("application.properties", nil, nil)
		})
	})

	t.Run("test OpFileFilterReg 1 ", func(t *testing.T) {
		check(t, `
		${application.properties}.re(select: aaa)
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterReg("application.properties", nil, nil)
		})
	})

	t.Run("test OpFileFilterReg 2", func(t *testing.T) {
		check(t, `
		${application.properties}.regexp(select: aaa)
		`, func(sfv *sfvm.SyntaxFlowVisitor) {
			sfv.EmitCheckStackTop()
			sfv.EmitFileFilterReg("application.properties", nil, nil)
		})
	})

	t.Run("config heredoc", func(t *testing.T) {
		check(t, `a #{
			hook: <<<HOOK
			*.a as $a
HOOK
}->`, func(sfv *sfvm.SyntaxFlowVisitor) {
			config := []*sfvm.RecursiveConfigItem{}
			config = append(config, &sfvm.RecursiveConfigItem{
				Key:   sfvm.RecursiveConfig_Hook,
				Value: `*.a as $a`,
			})
			sfv.EmitGetTopDefs(config...)
		})
	})

	t.Run("config heredoc complex", func(t *testing.T) {
		check(t, `a #{
			hook: <<<HOOK
			*.a as $a
			*-{
				until: <<<UNTIL
				*.b 
UNTIL
			}->
HOOK
		}->`, func(sfv *sfvm.SyntaxFlowVisitor) {
			config := []*sfvm.RecursiveConfigItem{}
			config = append(config, &sfvm.RecursiveConfigItem{
				Key:   sfvm.RecursiveConfig_Hook,
				Value: `*.a as $a`,
			})
			sfv.EmitGetTopDefs(config...)
		})
	})
}
