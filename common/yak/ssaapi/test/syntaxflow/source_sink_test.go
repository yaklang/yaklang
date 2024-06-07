package syntaxflow

import (
	"fmt"
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
			fmt.Sprintf(`system(* #{
				until: %s
			}-> * as $target)`, "`para`"),
			map[string][]string{
				"target": {"Parameter-para"},
			},
		)
	})

	t.Run("simple exclude", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (para) => {
			cmd := "bash" +  para 
			system(cmd)
		}
		`,
			fmt.Sprintf(`system(* #{
				exclude: %s
			}-> * as $target)`, "`para`"),
			map[string][]string{
				"target": {`"bash"`},
			},
		)
	})

	t.Run("with branch", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (para) => {
			cmd = "bash" 
			if para == "ls" {
				cmd += para
			}
			system(cmd)
		}
		`,
			"system(* #{until:`para`}-> * as $target)",
			map[string][]string{
				"target": {"Parameter-para"},
			},
		)
	})
}
