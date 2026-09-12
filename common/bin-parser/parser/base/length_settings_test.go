package base

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLengthSettingsLiveValues(t *testing.T) {
	for _, expanded := range []bool{false, true} {
		cfg := NewEmptyConfig()
		if expanded {
			cfg.SetItem("extension", true)
		}
		for _, value := range []any{nil, false, uint64(0), uint64(32), int(-1), float64(3.5), "raw", "../Size", []int{1}} {
			cfg.SetItem("type", value)
			cfg.SetItem("length-from-field", value)
			got := cfg.LengthSettings()
			require.False(t, got.HasLength)
			require.True(t, got.HasType)
			require.True(t, got.HasField)
			require.Equal(t, cfg.GetString("type"), got.Type)
			require.Equal(t, cfg.GetString("length-from-field"), got.Field)
			cfg.SetItem("length", value)
			got = cfg.LengthSettings()
			require.True(t, got.HasLength)
			require.Equal(t, cfg.GetUint64("length"), got.Length)
			cfg.DeleteItem("length")
		}
		cfg.DeleteItem("type")
		cfg.DeleteItem("length-from-field")
		require.Equal(t, LengthSettings{}, cfg.LengthSettings())
	}
	var zero Config
	require.Equal(t, LengthSettings{}, zero.LengthSettings())
}
