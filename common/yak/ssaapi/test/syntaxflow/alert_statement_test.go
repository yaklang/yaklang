package syntaxflow

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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

`, ssaapi.QueryWithEnableDebug(true))
			if err != nil {
				t.Fatal(err)
			}
			report, err := sfreport.ConvertSyntaxFlowResultsToSarif(result)
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
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("alert statement with extra message", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			result, err := prog.SyntaxFlowWithError(`
				f( * as $i )
				alert $i for {msg: "this is an alert message"}

`, ssaapi.QueryWithEnableDebug(true))
			if err != nil {
				t.Fatal(err)
			}
			report, err := sfreport.ConvertSyntaxFlowResultsToSarif(result)
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
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test alert get exInfo", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `f( * as $i )
alert $i for{
info: "info",
level: 'level'
}
`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
