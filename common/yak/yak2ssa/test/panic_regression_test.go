package test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

func TestUnaryAddressInCallDoesNotPanic(t *testing.T) {
	t.Run("unsupported unary operator still reports error", func(t *testing.T) {
		test.CheckTestCase(t, test.TestCase{
			Code: `
json := {Unmarshal: func(_, _) {}}
req := {Body: "{}"}
bodyData := {}
err := json.Unmarshal(req.Body, &bodyData)
`,
			Check: func(prog *ssaapi.Program, _ []string) {
				msgs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string {
					return e.Message
				})
				require.Contains(t, msgs, yak2ssa.UnaryOperatorNotSupport("&"), "should report unsupported unary operator")
			},
		})
	})
}
