package base

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireCompactReplay(t *testing.T, cfg *Config, count int) {
	t.Helper()
	i, ok := cfg.data.findLocked(CfgOptionFuns)
	require.True(t, ok)
	journal, ok := cfg.data.entries[i].value.(*configReplay)
	require.True(t, ok, "ordinary operations must not expose function history")
	require.Len(t, journal.writes, count)
}

func TestCompactReplayDeferredExposureAndCapacity(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 8, 9, 32, 257, 1025} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			current, legacy := NewEmptyConfig(), NewEmptyConfig()
			for i := 0; i < count; i++ {
				key := fmt.Sprint(i % 17)
				current.SetItem(key, i)
				configLegacySet(legacy, key, i)
			}
			require.True(t, current.Has(CfgOptionFuns))
			n, present, valid := current.ReplayHistoryLen()
			require.Equal(t, count, n)
			require.True(t, present && valid)
			requireCompactReplay(t, current, count)
			current.DeleteItem("0") // deletion changes values, not assignment history
			legacy.DeleteItem("0")
			merged := AppendConfig(NewEmptyConfig(), current)
			requireCompactReplay(t, current, count)
			require.Equal(t, configLookupSnapshot(configLegacyAppend(NewEmptyConfig(), legacy)), configLookupSnapshot(merged))
			got, old := current.GetItem(CfgOptionFuns).([]NodeConfigFun), legacy.GetItem(CfgOptionFuns).([]NodeConfigFun)
			require.Equal(t, len(old), len(got))
			require.Equal(t, cap(old), cap(got))
			// Exposure must be stable, not a newly generated slice on each read.
			got[0] = func(c *Config) { c.SetItem("public edit", true) }
			current.SetItem("after exposure", true)
			replayed := AppendConfig(NewEmptyConfig(), current)
			require.Equal(t, true, replayed.GetItem("public edit"))
			require.Equal(t, true, replayed.GetItem("after exposure"))
		})
	}
}

func TestCompactReplayResetAndInvalidHistory(t *testing.T) {
	for _, installed := range []any{nil, "invalid", []NodeConfigFun(nil)} {
		cfg := NewEmptyConfig()
		cfg.SetItem("before", 1)
		cfg.BaseKV.SetItem(CfgOptionFuns, installed)
		_, present, valid := cfg.ReplayHistoryLen()
		require.True(t, present)
		_, expectedValid := installed.([]NodeConfigFun)
		require.Equal(t, expectedValid, valid)
		if !valid {
			require.Panics(t, func() { cfg.SetItem("written before panic", true) })
			require.Equal(t, true, cfg.GetItem("written before panic"))
			require.Panics(t, func() { AppendConfig(NewEmptyConfig(), cfg) })
		}
		cfg.DeleteItem(CfgOptionFuns)
		n, present, valid := cfg.ReplayHistoryLen()
		require.Equal(t, 0, n)
		require.False(t, present)
		require.True(t, valid)
		cfg.SetItem("after reset", nil)
		requireCompactReplay(t, cfg, 1)
		replayed := AppendConfig(NewEmptyConfig(), cfg)
		require.False(t, replayed.Has("before"))
		require.True(t, replayed.Has("after reset"))
	}
}

func TestCompactReplayCustomReentrantAliasing(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		set, copyConfig, merge := (*Config).SetItem, CopyConfig, AppendConfig
		if legacy {
			set, copyConfig, merge = configLegacySet, configLegacyCopy, configLegacyAppend
		}
		cfg := NewEmptyConfig()
		set(cfg, "a", 1)
		set(cfg, "b", 2)
		options := cfg.GetItem(CfgOptionFuns).([]NodeConfigFun)
		options[0] = func(target *Config) {
			cfg.BaseKV.SetItem("reentered", true)
			options[1] = func(target *Config) { set(target, "replacement", true) }
		}
		got := merge(NewEmptyConfig(), cfg)
		require.True(t, cfg.GetBool("reentered"))
		require.True(t, got.GetBool("replacement"), "callback edits to later functions must be visible")
		require.False(t, got.Has("b"))

		// Explicitly supplied spare capacity remains shallow-shared by Copy.
		shared := make([]NodeConfigFun, 2, 16)
		shared[0], shared[1] = func(*Config) {}, func(*Config) {}
		source := NewEmptyConfig()
		source.BaseKV.SetItem(CfgOptionFuns, shared)
		copied := copyConfig(source)
		shared[0] = func(target *Config) { set(target, "shared callback", true) }
		require.True(t, merge(NewEmptyConfig(), copied).GetBool("shared callback"))
	}
}

func TestCompactReplayConcurrentExposureAndAppend(t *testing.T) {
	cfg := NewEmptyConfig()
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 80; i++ {
				cfg.SetItem(fmt.Sprint(worker), i)
				cfg.Has(CfgOptionFuns)
				cfg.ReplayHistoryLen()
				if i%17 == 0 {
					cfg.GetItem(CfgOptionFuns)
				}
			}
		}(worker)
	}
	wg.Wait()
	n, present, valid := cfg.ReplayHistoryLen()
	require.Equal(t, 8*80, n)
	require.True(t, present && valid)
	replayed := AppendConfig(NewEmptyConfig(), cfg)
	for worker := 0; worker < 8; worker++ {
		require.Equal(t, 79, replayed.GetItem(fmt.Sprint(worker)))
	}
}
