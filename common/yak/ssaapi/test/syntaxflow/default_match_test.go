package syntaxflow

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func WithSyntaxFlowResult(expected string, handler func(*ssaapi.Value) error) sfvm.Option {
	return sfvm.WithResultCaptured(func(name string, results sfvm.ValueOperator) error {
		if name != expected {
			return nil
		}
		return results.Recursive(func(operator sfvm.ValueOperator) error {
			result, ok := operator.(*ssaapi.Value)
			if !ok {
				return nil
			}
			err := handler(result)
			if err != nil {
				return err
			}
			return nil
		})
	})
}

func TestDefaultMatch(t *testing.T) {
	prog, _ := ssaapi.Parse(`
a = b => {
	return b + 4
}

dump(a(2))

`)
	var count = 0
	var ssaValueCount = 0
	var check2 = false
	var check4 = false
	prog.SyntaxFlowChain(`dump(* #-> * as $abc)`, ssaapi.QueryWithEnableDebug(true), ssaapi.QueryWithResultCaptured(func(name string, op sfvm.ValueOperator) error {
		count++
		return nil
	}), ssaapi.QueryWithSyntaxFlowResult("abc", func(value *ssaapi.Value) error {
		ssaValueCount++
		spew.Dump(value.String())
		switch value.String() {
		case "2":
			check2 = true
		case "4":
			check4 = true
		}
		return nil
	}))
	assert.Equal(t, 2, count, `Default Params`)
	assert.Equal(t, 2, ssaValueCount, `Default SSA Value`)
	assert.True(t, check2, "check 2 is failed")
	assert.True(t, check4, "check 4 is failed")
}
