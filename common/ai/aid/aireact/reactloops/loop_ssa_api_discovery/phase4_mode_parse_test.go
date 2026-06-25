package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizePhase4Mode_DefaultDeepMining(t *testing.T) {
	require.Equal(t, Phase4ModeDeepMining, NormalizePhase4Mode(""))
	require.Equal(t, Phase4ModeDeepMining, NormalizePhase4Mode("深度挖掘"))
	require.Equal(t, Phase4ModeDeepMining, NormalizePhase4Mode("deep_mining"))
}

func TestNormalizePhase4Mode_BatchScan(t *testing.T) {
	require.Equal(t, Phase4ModeBatchScan, NormalizePhase4Mode("batch_scan"))
	require.Equal(t, Phase4ModeBatchScan, NormalizePhase4Mode("灰盒批量"))
	require.Equal(t, Phase4ModeBatchScan, NormalizePhase4Mode("congin"))
}

func TestParseUserInput_Phase4Mode(t *testing.T) {
	base := "Code path: /tmp/p\nTarget: http://127.0.0.1:8080\n"
	p, err := ParseUserInput(base + "phase4_mode: batch_scan\n")
	require.NoError(t, err)
	require.Equal(t, Phase4ModeBatchScan, NormalizePhase4Mode(p.Phase4Mode))

	p2, err := ParseUserInput(base + "phase4-mode: 深度挖掘\n")
	require.NoError(t, err)
	require.Equal(t, Phase4ModeDeepMining, NormalizePhase4Mode(p2.Phase4Mode))
}
