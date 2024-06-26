package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCheckStatement(t *testing.T) {
	t.Run("simple, positive", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return i
		}
		f(1)
		`,
			func(prog *ssaapi.Program) error {
				data, err := prog.SyntaxFlowWithError(`
				f( * as $i )
				check $i then fine else fail
				`, sfvm.WithEnableDebug())
				assert.Nil(t, err)
				log.Infof("data: %v", data)
				return nil
			})
	})

	t.Run("simple, negative, just check ", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return i
		}
		f(1)
		`,
			func(prog *ssaapi.Program) error {
				data, err := prog.SyntaxFlowWithError(`
				bbbb( * as $i )
				check $i then fine else fail
				f( * as $b)
				`, sfvm.WithEnableDebug())
				assert.NotNil(t, data.Errors)
				log.Infof("err: %v", err)
				log.Infof("data: %v", data)
				res := data.GetValues("b")
				assert.Greater(t, len(res), 0, "b should have values")
				return nil
			})
	})

	t.Run("simple, negative, fail fast", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return i
		}
		f(1)
		`,
			func(prog *ssaapi.Program) error {
				data, err := prog.SyntaxFlowWithError(`
				bbbb( * as $i )
				check $i then fine else fail
				f( * as $b)
				`,
					sfvm.WithEnableDebug(), sfvm.WithFailFast())
				assert.NotNil(t, err)
				log.Infof("err: %v", err)
				log.Infof("data: %v", data)
				assert.Equal(t, 1, data.SymbolTable.Len()) // just "_"
				return nil
			})
	})
}
