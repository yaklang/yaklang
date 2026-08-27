package ssaapi

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestSaveRiskKeepsMemoryRiskWhenDBPersistFails verifies that a stale/read-only
// SSA IR DB (e.g. missing a newly added risk column) does NOT silently drop
// findings: the risk stays in memory so the streaming path (EmitSSAResult →
// ssa-stream → platform artifact import) still delivers it.
func TestSaveRiskKeepsMemoryRiskWhenDBPersistFails(t *testing.T) {
	code := `print(f())`
	prog, err := Parse(code,
		WithLanguage(ssaconfig.Yak),
		WithProgramName(uuid.NewString()),
	)
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(`
desc(title: "t")
print(* as $para)
alert $para for { "msg": "x", "level": "warning" }
`)
	require.NoError(t, err)

	// Simulate a stale schema: fresh SSA DB with the ssa_risks table dropped,
	// so CreateSSARisk fails exactly like the missing scan_mode column did.
	broken, err := consts.GetTempSSADataBase()
	require.NoError(t, err)
	require.NoError(t, broken.Exec("DROP TABLE IF EXISTS ssa_risks").Error)

	old := consts.GetGormSSAProjectDataBase()
	consts.SetGormSSAProjectDatabase(broken)
	defer consts.SetGormSSAProjectDatabase(old)

	// save=true path hits the broken DB; Save itself must not fail.
	_, err = res.Save(schema.SFResultKindScan, uuid.NewString())
	require.NoError(t, err)

	// The risk must still be present in memory so streaming delivers it.
	count := 0
	for range res.YieldRisk() {
		count++
	}
	require.Greater(t, count, 0, "risk must survive a failed DB persist (streaming path)")
}
