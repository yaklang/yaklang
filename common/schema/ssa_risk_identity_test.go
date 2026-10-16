package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestSSARiskHashUsesResultUUID(t *testing.T) {
	risk := &SSARisk{CodeSourceUrl: "file:///main.yak", CodeRange: "1:1-1:2", RuntimeId: "scan",
		ProgramName: "program", ResultID: 42, Variable: "alert", Index: 1, Title: "finding", RiskType: "custom"}
	empty := risk.CalcHash()
	require.Equal(t, utils.CalcSha1(risk.CodeSourceUrl, risk.CodeRange, risk.RuntimeId,
		risk.ProgramName, risk.ResultUUID, risk.Variable, risk.Index, risk.Title, risk.RiskType), empty)

	risk.ResultID = 99
	require.Equal(t, empty, risk.CalcHash(), "an empty result UUID is the identity; the numeric row id is not")
	risk.ResultUUID = "stable-result"
	hash := risk.CalcHash()
	risk.ResultID = 7
	require.Equal(t, hash, risk.CalcHash(), "binding the numeric audit row must not change a streamed hash")
	risk.ResultUUID = "other-result"
	require.NotEqual(t, hash, risk.CalcHash(), "different results remain distinguishable")
}

func TestSSARiskPrepareForSaveComputesFeatureHash(t *testing.T) {
	risk := &SSARisk{
		FunctionName: "main", FromRule: "rule", Variable: "target", Severity: "high",
		RiskType: "custom", Language: "yak", Title: "finding",
	}
	risk.PrepareForSave("println(123)")
	require.Equal(t, risk.CalcFeatureHash("println(123)"), risk.RiskFeatureHash)
	require.Equal(t, risk.CalcHash(), risk.Hash)
	require.Equal(t, "ssa", risk.ScanMode)

	kept := risk.RiskFeatureHash
	risk.PrepareForSave()
	require.Equal(t, kept, risk.RiskFeatureHash, "a blank feature text does not clear the cover key")
}
