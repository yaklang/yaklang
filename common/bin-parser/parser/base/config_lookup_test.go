package base

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Keep the former two-lookup operations as an independent compatibility and
// performance control. The optimization changes lookups, not option replay,
// shallow value sharing, ordering, or the distinction between nil and absent.
func configLegacySet(c *Config, key string, value any) {
	c.BaseKV.SetItem(key, value)
	option := func(target *Config) { configLegacySet(target, key, value) }
	if c.Has(CfgOptionFuns) {
		c.data.Set(CfgOptionFuns, append(c.GetItem(CfgOptionFuns).([]NodeConfigFun), option))
	} else {
		c.data.Set(CfgOptionFuns, []NodeConfigFun{option})
	}
}

func configLegacyInherit(parent *Config) *Config {
	result := NewEmptyConfig()
	for _, key := range []string{"endian", "parser", "unit"} {
		if parent.Has(key) {
			configLegacySet(result, key, parent.GetItem(key))
		}
	}
	return result
}

func configLegacyCopy(source *Config) *Config {
	result := NewEmptyConfig()
	source.data.ForEach(func(key string, value any) bool {
		configLegacySet(result, key, value)
		return true
	})
	return result
}

func configLegacyAppend(parent, child *Config) *Config {
	result := configLegacyCopy(parent)
	if child.Has(CfgOptionFuns) {
		for _, option := range child.GetItem(CfgOptionFuns).([]NodeConfigFun) {
			option(result)
		}
	}
	return result
}

func configLookupSnapshot(c *Config) []any {
	var result []any
	c.data.ForEach(func(key string, value any) bool {
		if key == CfgOptionFuns {
			value = len(value.([]NodeConfigFun))
		}
		result = append(result, key, value)
		return true
	})
	return result
}

func TestConfigSingleLookupMatchesLegacyOperations(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		t.Run(fmt.Sprint(mask), func(t *testing.T) {
			oldParent, newParent := NewEmptyConfig(), NewEmptyConfig()
			shared := map[string]any{"shared": 1}
			for index, key := range []string{"endian", "parser", "unit"} {
				if mask&(1<<index) != 0 {
					var value any
					if index == 0 {
						value = "little"
					} else if index == 2 {
						value = uint64(8)
					}
					configLegacySet(oldParent, key, value)
					newParent.SetItem(key, value)
				}
			}
			configLegacySet(oldParent, "not-inherited", shared)
			newParent.SetItem("not-inherited", shared)
			oldChild, newChild := configLegacyInherit(oldParent), NewConfig(newParent)
			require.Equal(t, configLookupSnapshot(oldChild), configLookupSnapshot(newChild))
			require.False(t, newChild.Has("not-inherited"))
			for index, value := range []any{nil, "big", uint64(0), shared, false, "little"} {
				key := []string{"endian", "value", "other"}[index%3]
				configLegacySet(oldChild, key, value)
				newChild.SetItem(key, value)
				require.Equal(t, configLookupSnapshot(oldChild), configLookupSnapshot(newChild))
			}
			oldChild.DeleteItem("other")
			newChild.DeleteItem("other")
			oldCopy, newCopy := configLegacyCopy(oldChild), CopyConfig(newChild)
			require.Equal(t, configLookupSnapshot(oldCopy), configLookupSnapshot(newCopy))
			oldMerged, newMerged := configLegacyAppend(oldParent, oldChild), AppendConfig(newParent, newChild)
			require.Equal(t, configLookupSnapshot(oldMerged), configLookupSnapshot(newMerged))
			// Replay preserves write history, including the key deleted above.
			require.True(t, newMerged.Has("other"))
			shared["shared"] = 2
			require.Equal(t, shared, newMerged.GetItem("not-inherited"))
			require.Equal(t, configLookupSnapshot(oldMerged), configLookupSnapshot(newMerged))
		})
	}
}

func TestConfigSingleLookupPreservesInvalidOptionBehavior(t *testing.T) {
	for _, value := range []any{nil, "invalid", uint64(1)} {
		old, current := NewEmptyConfig(), NewEmptyConfig()
		old.BaseKV.SetItem(CfgOptionFuns, value)
		current.BaseKV.SetItem(CfgOptionFuns, value)
		require.Panics(t, func() { configLegacySet(old, "written", true) })
		require.Panics(t, func() { current.SetItem("written", true) })
		require.Equal(t, true, current.GetItem("written"))
		require.Panics(t, func() { configLegacyAppend(NewEmptyConfig(), old) })
		require.Panics(t, func() { AppendConfig(NewEmptyConfig(), current) })
	}
}

func TestConfigSingleLookupPreservesCustomOptions(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		set, merge := (*Config).SetItem, AppendConfig
		if legacy {
			set, merge = configLegacySet, configLegacyAppend
		}
		child := NewEmptyConfig()
		child.BaseKV.SetItem(CfgOptionFuns, []NodeConfigFun(nil))
		set(child, "first", nil)
		require.Len(t, child.GetItem(CfgOptionFuns), 1)
		var trace []string
		child.BaseKV.SetItem(CfgOptionFuns, []NodeConfigFun{
			func(target *Config) { trace = append(trace, "one"); set(target, "ordered", 1) },
			func(target *Config) { trace = append(trace, "two"); set(target, "ordered", 2) },
		})
		set(child, "after-custom", true)
		merged := merge(NewEmptyConfig(), child)
		require.Equal(t, []string{"one", "two"}, trace)
		require.Equal(t, 2, merged.GetItem("ordered"))
		require.Equal(t, true, merged.GetItem("after-custom"))
	}
}

var configLookupBenchmarkSink *Config

func BenchmarkConfigInheritanceAndWrites(b *testing.B) {
	for _, legacy := range []bool{true, false} {
		name := "single-lookup"
		set, inherit, merge := (*Config).SetItem, NewConfig, AppendConfig
		if legacy {
			name, set, inherit, merge = "legacy-two-lookups", configLegacySet, configLegacyInherit, configLegacyAppend
		}
		b.Run(name, func(b *testing.B) {
			parent := NewEmptyConfig()
			set(parent, "endian", "big")
			set(parent, "parser", "default")
			set(parent, "unit", uint64(8))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				child := inherit(parent)
				set(child, CfgLength, uint64(48))
				set(child, CfgParent, parent)
				set(child, CfgNodeResult, [2]uint64{0, 48})
				configLookupBenchmarkSink = merge(parent, child)
			}
		})
	}
}
