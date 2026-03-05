package syntaxflow

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_BinaryCompareShorthand_ObjectMemberLen(t *testing.T) {
	code := `
x1 = { b: 1, c: 2 }
x2 = { b: 1 }
`
	ssatest.CheckSyntaxFlow(t, code, `x*?{.*<len>==2} as $target`, map[string][]string{
		"target": {"x1"},
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestCondition_CallArg_ShorthandMatchesExplicitOptionalFilter(t *testing.T) {
	code := `
f = () => { return 1 }
a(f)
a(1)
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		explicit, err := prog.SyntaxFlowWithError(`a?(*?{opcode:function}) as $result`)
		require.NoError(t, err)
		shorthand, err := prog.SyntaxFlowWithError(`a?(opcode:function) as $result`)
		require.NoError(t, err)

		gotExplicit := explicit.GetValues("result")
		gotShorthand := shorthand.GetValues("result")
		explicitStr := make([]string, 0, gotExplicit.Len())
		shorthandStr := make([]string, 0, gotShorthand.Len())
		for _, v := range gotExplicit {
			explicitStr = append(explicitStr, v.String())
		}
		for _, v := range gotShorthand {
			shorthandStr = append(shorthandStr, v.String())
		}
		sort.Strings(explicitStr)
		sort.Strings(shorthandStr)
		require.Equal(t, explicitStr, shorthandStr)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestCondition_CallArg_BinaryCompareShorthand(t *testing.T) {
	code := `
a("param1", "param2")
a("param1", "param2", "param3")
`
	tests := []struct {
		name string
		rule string
		want []string
	}{
		{
			name: "len_eq_2",
			rule: `a?(*<len>==2) as $result`,
			want: []string{`Undefined-a("param1","param2")`},
		},
		{
			name: "len_eq_3",
			rule: `a?(*<len>==3) as $result`,
			want: []string{`Undefined-a("param1","param2","param3")`},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlow(t, code, tt.rule, map[string][]string{
				"result": tt.want,
			}, ssaapi.WithLanguage(ssaconfig.Yak))
		})
	}
}

