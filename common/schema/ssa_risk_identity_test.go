package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestSSARiskHashKeepsLegacyIdentity(t *testing.T) {
	risk := &SSARisk{CodeSourceUrl: "file:///main.yak", CodeRange: "1:1-1:2", RuntimeId: "scan",
		ProgramName: "program", ResultID: 42, Variable: "alert", Index: 1, Title: "finding", RiskType: "custom"}
	require.Equal(t, utils.CalcSha1(risk.CodeSourceUrl, risk.CodeRange, risk.RuntimeId,
		risk.ProgramName, risk.ResultID, risk.Variable, risk.Index, risk.Title, risk.RiskType), risk.CalcHash())

	risk.ResultUUID = "stable-result"
	hash := risk.CalcHash()
	risk.ResultID = 99
	require.Equal(t, hash, risk.CalcHash(), "binding the numeric audit row must not change a streamed hash")
	risk.ResultUUID = "other-result"
	require.NotEqual(t, hash, risk.CalcHash(), "different results remain distinguishable")
}
