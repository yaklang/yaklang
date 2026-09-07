package base

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResultSettingsReadsCurrentPublicValues(t *testing.T) {
	for _, expand := range []bool{false, true} {
		cfg := NewEmptyConfig()
		if expand {
			cfg.SetItem("extension", true)
		}
		for _, v := range []any{nil, false, "raw", "little", uint64(1), [2]uint64{1, 9}, []int{1}} {
			for _, key := range []string{CfgNodeResult, CfgType, CfgEndian, CfgIsTerminal, "list"} {
				cfg.SetItem(key, v)
			}
			got := cfg.ResultSettings()
			require.Equal(t, cfg.GetItem(CfgNodeResult), got.Position)
			require.Equal(t, cfg.Has(CfgNodeResult), got.Present)
			require.Equal(t, cfg.GetString(CfgType), got.Type)
			require.Equal(t, cfg.GetString(CfgEndian), got.Endian)
			require.Equal(t, cfg.GetBool(CfgIsTerminal), got.Terminal)
			require.Equal(t, cfg.GetBool("list"), got.List)
		}
		cfg.DeleteItem(CfgNodeResult)
		require.False(t, cfg.ResultSettings().Present)
		cfg.SetItem(CfgNodeResult, nil)
		require.True(t, cfg.ResultSettings().Present)
	}
	var zero Config
	require.Equal(t, ResultSettings{}, zero.ResultSettings())
}
