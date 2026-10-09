package reactloops

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Exercise actual Yak literals through action registration and validation, not
// Go float64 fixtures that hide the runtime's integer representation.
func TestFocusActionSchemaYakNumericBounds(t *testing.T) {
	for _, tc := range []struct {
		name, kind, bounds string
		accepted, rejected []any
	}{
		{"log_read_integer", "integer", `"min":1,"max":65536`, []any{468, 4096, 1, 65536}, []any{0, 65537, 1.5}},
		{"integer_bounds_on_number", "number", `"min":0,"max":1`, []any{0, 0.5, 1}, []any{-0.1, 1.1}},
		{"fractional", "number", `"min":0.25,"max":1.75`, []any{0.25, 0.5, 1.75}, []any{0, 2}},
		{"zero_minimum", "number", `"min":0`, []any{0, 0.5}, []any{-0.5}},
		{"zero_maximum", "number", `"max":0`, []any{-0.5, 0}, []any{0.5}},
		{"negative_integer", "integer", `"min":-10,"max":-1`, []any{-10, -5, -1}, []any{-11, 0}},
		{"absent_bounds", "number", `"description":"unbounded"`, []any{-100, 100}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loop := newMinimalReActLoopForOptionTest()
			code := fmt.Sprintf(`__ACTIONS__ = [{"type":"read_slice","options":[{"name":"max_bytes","type":%q,"required":true,%s}],"handler":func(loop,action,op){}}]`, tc.kind, tc.bounds)
			applyFocusActionValidationScript(t, loop, code)
			action, ok := loop.actions.Get("read_slice")
			require.True(t, ok)
			for _, value := range tc.accepted {
				require.NoError(t, action.ActionVerifier(loop, aicommon.NewSimpleAction("read_slice", aitool.InvokeParams{"max_bytes": value})), "accepted value %v", value)
			}
			for _, value := range tc.rejected {
				require.ErrorContains(t, action.ActionVerifier(loop, aicommon.NewSimpleAction("read_slice", aitool.InvokeParams{"max_bytes": value})), "max_bytes", "rejected value %v", value)
			}
		})
	}
}
