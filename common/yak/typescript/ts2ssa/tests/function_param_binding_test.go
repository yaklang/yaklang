package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestFunctionParamBindingPatternCompile(t *testing.T) {
	cases := map[string]string{
		"array destructuring": `var f = ([a, b]) => a + b`,
		"object destructuring": `var g = ({x, y}) => x + y`,
		"rest identifier":      `var h = (...args) => args`,
		"rest array binding":   `var i = (...[a, b]) => a + b`,
		"mixed params":         `var j = (x, [a, b], {c}) => x + a + b + c`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.TS))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
		})
	}
}
