package base

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCompactConfigVersioningAndExpansion(t *testing.T) {
	for i, key := range compactConfigKeys {
		if i > 0 {
			require.Equal(t, uint8(i), compactConfigKey(key))
		}
	}
	for _, trigger := range []string{"unknown", "delete", "callbacks", "iteration"} {
		cfg, old := NewEmptyConfig(), NewEmptyConfig()
		for _, entry := range []ConfigItem{{"endian", "big"}, {"length", uint64(1)}, {"length", uint64(2)}, {"parent", nil}, {"type", "raw"}} {
			cfg.SetItem(entry.Key, entry.Value)
			configLegacySet(old, entry.Key, entry.Value)
		}
		require.Nil(t, cfg.data.configStoreLegacy)
		require.Equal(t, 5, len(cfg.data.writes))
		require.Equal(t, uint64(2), cfg.GetItem("length"))
		require.True(t, cfg.Has("parent"))
		// A BaseKV write changes the current value but is not a replay entry.
		cfg.BaseKV.SetItem("length", uint64(9))
		old.BaseKV.SetItem("length", uint64(9))
		check := func() {
			x, y := AppendConfig(NewEmptyConfig(), cfg), configLegacyAppend(NewEmptyConfig(), old)
			require.Equal(t, configLookupSnapshot(y), configLookupSnapshot(x))
			require.Equal(t, uint64(2), x.GetItem("length"))
		}
		check()
		switch trigger {
		case "unknown":
			cfg.SetItem("extension", true)
			configLegacySet(old, "extension", true)
		case "delete":
			cfg.DeleteItem("length")
			old.DeleteItem("length")
		case "callbacks":
			cfg.GetItem(CfgOptionFuns)
			old.GetItem(CfgOptionFuns)
		case "iteration":
			cfg.data.ForEach(func(string, any) bool { return true })
			old.data.ForEach(func(string, any) bool { return true })
		}
		require.NotNil(t, cfg.data.configStoreLegacy)
		require.Nil(t, cfg.data.writes)
		check()
		require.Equal(t, configLookupSnapshot(old), configLookupSnapshot(cfg))
	}
}

func TestCompactConfigBoundedVersions(t *testing.T) {
	cfg := NewEmptyConfig()
	for i := 0; i < 65536; i++ {
		cfg.SetItem("length", i)
	}
	require.Equal(t, 65535, cfg.GetItem("length"))
	n, p, v := cfg.ReplayHistoryLen()
	require.Equal(t, 65536, n)
	require.True(t, p && v)
	require.NotNil(t, cfg.data.configStoreLegacy)
}
