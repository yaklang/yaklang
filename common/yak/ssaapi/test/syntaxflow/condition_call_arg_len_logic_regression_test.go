package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_CallArgLenLogicalRegression(t *testing.T) {
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
			name: "len_eq_2_should_keep_first_call",
			rule: `a?(*<len>?{==2}) as $result`,
			want: []string{`Undefined-a("param1","param2")`},
		},
		{
			name: "len_eq_3_should_keep_second_call",
			rule: `a?(*<len>?{==3}) as $result`,
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
