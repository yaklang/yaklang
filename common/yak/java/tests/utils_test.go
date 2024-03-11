package tests

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
	"testing"
)

func init() {
	test.SetLanguage("java", java2ssa.Build)
}

func createJavaProgram(s string) string {
	template := `package org.example;

public class Main {
    public static void main(String[] args) {
        %s
    }
}`

	program := fmt.Sprintf(template, s)
	return program
}

func CheckJavaTestCase(t *testing.T, tc test.TestCase) {
	prog, err := ssaapi.Parse(tc.Code, ssaapi.WithLanguage("java"))
	require.Nil(t, err, "parse error")
	prog.Show()
	fmt.Println(prog.GetErrors().String())
	if tc.Want == nil {
		tc.Want = make([]string, 0)
	}
	if tc.Check != nil {
		tc.Check(prog, tc.Want)
	}

}

func CheckJavaPrintlnValue(code string, want []string, t *testing.T) {
	CheckJavaPrintf(t, test.TestCase{
		Code: code,
		Want: want,
	})
}

func CheckJavaPrintf(t *testing.T, tc test.TestCase) {
	tc.Check = func(prog *ssaapi.Program, want []string) {
		println := prog.Ref("packages").ShowWithSource()
		got := lo.Map(
			println.GetUsers().ShowWithSource().Flat(func(v *ssaapi.Value) ssaapi.Values {
				return ssaapi.Values{v.GetOperand(1)}
			}),
			func(v *ssaapi.Value, _ int) string {
				return v.String()
			},
		)

		log.Info("got :", got)

		log.Info("want :", want)

		require.Equal(t, want, got)
	}
	CheckJavaTestCase(t, tc)
}

func CheckJavaCode(code string, t *testing.T) {
	CheckJavaTestCase(t, test.TestCase{Code: code})
}
