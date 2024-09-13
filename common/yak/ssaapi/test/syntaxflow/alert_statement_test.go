package syntaxflow

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestAlertStatement(t *testing.T) {
	code := `f = (i) => {
			return i
		}
		f(1)`

	t.Run("alert statement without extra message", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			result, err := prog.SyntaxFlowWithError(`
				f( * as $i )
				alert $i

`, sfvm.WithEnableDebug(true))
			if err != nil {
				t.Fatal(err)
			}
			report, err := ssaapi.ConvertSyntaxFlowResultToSarif(result)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			err = report.PrettyWrite(&buf)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println(string(buf.String()))
			return nil
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})

	t.Run("alert statement with extra message", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			result, err := prog.SyntaxFlowWithError(`
				f( * as $i )
				alert $i for "this is an alert message"

`, sfvm.WithEnableDebug(true))
			if err != nil {
				t.Fatal(err)
			}
			report, err := ssaapi.ConvertSyntaxFlowResultToSarif(result)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			err = report.PrettyWrite(&buf)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println(string(buf.String()))
			return nil
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
}
