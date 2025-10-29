package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCheckStatement(t *testing.T) {

	t.Run("fast fail as default ", func(t *testing.T) {
		code := `
b = 1 
b2 = 2 
b3 = 3
	`

		ssatest.CheckSyntaxFlow(t, code, `
	a* as $a
	check $a 
	b* as $b 
	`, map[string][]string{
			"b": {},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

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
				`, ssaapi.QueryWithEnableDebug())
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
				`, ssaapi.QueryWithEnableDebug(), ssaapi.QueryWithFailFast(false))
				assert.NotNil(t, data.GetErrors())
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
				f(* as $b)
				`,
					ssaapi.QueryWithEnableDebug(), ssaapi.QueryWithFailFast())
				assert.Nil(t, err)
				log.Infof("err: %v", err)
				log.Infof("data: %v", data)
				assert.Equal(t, 0, len(data.GetAllValuesChain()))
				return nil
			})
	})
}
