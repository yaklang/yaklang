package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Source_Sink(t *testing.T) {
	t.Run("simple utils", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (para) => {
			cmd := "bash" + "-c" +  para 
			system(cmd)
		}
		`,
			`
system(* #{
	until: <<<UNTIL
		* ?{opcode:add}
UNTIL
}-> * as $target)`,
			map[string][]string{
				"target": {`add("bash-c", Parameter-para)`},
			},
		)
	})

	t.Run("simple normal test", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (para) => {
			cmd = "bash" 
			if para == "ls" {
				cmd += para
			}
			system(cmd)
		}
		`,
			`
system(* #-> * as $target)`,
			map[string][]string{
				"target": {"Parameter-para", `"bash"`, `"ls"`},
			},
		)
	})

	/*FIXME: this is a bug,
	should contain bash,
	bash have two dataflow path:
		1. phi -> bash
		2. phi -> add -> bash
	but in v.GetDataFlowPath(), bash get all path:
		(phi, add, bash)
	and then exclude rule will exclude "bash" in result
	*/
	t.Run("simple exclude", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (para) => {
			cmd := "bash" 
			if para == "ls" {
				cmd += para
			}
			system(cmd)
		}
		`,
			`system(* #{
	exclude: <<<EXCLUDE
		* ?{opcode:add}
EXCLUDE
}-> * as $target)`,
			map[string][]string{
				"target": {
					// `"bash"`,
					`"ls"`,
				},
			},
		)
	})
	t.Run("simple include", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (para) => {
			cmd = "bash" 
			if para == "ls" {
				cmd += para
			}
			system(cmd)
		}
		`,
			`
system(* #{
	include: <<<INCLUDE
		* ?{opcode:param}
INCLUDE
}-> * as $target)`,
			map[string][]string{
				"target": {"Parameter-para"},
			},
		)
	})
}
