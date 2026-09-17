package base

import (
	"testing"

	"github.com/stretchr/testify/require"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestConfigBatchMatchesOrderedAssignments(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		parent := NewEmptyConfig()
		for i, key := range []string{"endian", "parser", "unit"} {
			if mask&(1<<i) != 0 {
				parent.SetItem(key, []any{"little", nil, "bit"}[i])
			}
		}
		shared := map[string]any{"value": 1}
		items := []ConfigItem{{"nil", nil}, {CfgEndian, "big"}, {CfgLength, uint64(8)}, {"shared", shared}, {CfgLength, uint64(0)}}
		old := configLegacyInherit(parent)
		for _, item := range items {
			configLegacySet(old, item.Key, item.Value)
		}
		current := NewConfigWithItems(parent, items...)
		separate := NewConfig(parent)
		separate.SetItems(items...)
		require.Equal(t, configLookupSnapshot(old), configLookupSnapshot(current))
		require.Equal(t, configLookupSnapshot(old), configLookupSnapshot(separate))
		require.Equal(t, configLookupSnapshot(configLegacyAppend(parent, old)), configLookupSnapshot(AppendConfig(parent, current)))
		shared["value"] = 2
		require.Equal(t, 2, current.GetItem("shared").(map[string]any)["value"])
	}
}

func TestConfigBatchExplicitHistoryAndPartialFailure(t *testing.T) {
	for _, value := range []any{nil, "invalid", []NodeConfigFun(nil), []NodeConfigFun{func(c *Config) { c.SetItem("callback", 1) }}} {
		items := []ConfigItem{{"first", 1}, {CfgOptionFuns, value}, {"last", 2}}
		old, current := NewEmptyConfig(), NewEmptyConfig()
		_, valid := value.([]NodeConfigFun)
		legacyWrite := func() {
			for _, item := range items {
				configLegacySet(old, item.Key, item.Value)
			}
		}
		if valid {
			require.NotPanics(t, legacyWrite)
			require.NotPanics(t, func() { current.SetItems(items...) })
			require.Equal(t, configLookupSnapshot(old), configLookupSnapshot(current))
		} else {
			require.Panics(t, legacyWrite)
			require.Panics(t, func() { current.SetItems(items...) })
			require.Equal(t, 1, current.GetItem("first"))
			require.Equal(t, value, current.GetItem(CfgOptionFuns))
			require.False(t, current.Has("last"))
		}
	}
	var zero Config
	require.NotPanics(t, func() { zero.SetItems(ConfigItem{"ignored", true}) })
	require.False(t, zero.Has("ignored"))
}

func TestConfigBatchNodeDescriptionsAndReplay(t *testing.T) {
	parent := NewEmptyConfig()
	parent.SetItem(CfgEndian, "little")
	parent.SetItem("parser", "default")
	parent.SetItem("unit", "byte")
	items := []ConfigItem{{CfgNodeResult, [2]uint64{3, 27}}, {CfgEndian, "big"}, {CfgParent, parent}, {CfgLength, uint64(24)}, {CfgIsList, false}, {"index", 1}}
	for _, data := range []any{"", "...", "raw", "uint16", "Type...", " raw ", "raw,3", "raw,invalid", "type:uint16;endian:little", yaml.MapSlice{}, yaml.MapSlice{{Key: "Child", Value: "uint8"}}, 42} {
		old, oldErr := NewNodeTreeWithConfig(parent, "Field", data, nil)
		current, err := NewNodeTreeWithConfigItems(parent, "Field", data, nil, items...)
		if oldErr != nil {
			require.EqualError(t, err, oldErr.Error())
			require.Nil(t, current)
			continue
		}
		require.NoError(t, err)
		for _, item := range items {
			old.Cfg.SetItem(item.Key, item.Value)
		}
		require.Equal(t, old.Origin, current.Origin)
		require.Equal(t, old.Name, current.Name)
		require.Equal(t, len(old.Children), len(current.Children))
		require.Equal(t, old.Children == nil, current.Children == nil)
		require.Equal(t, configLookupSnapshot(old.Cfg), configLookupSnapshot(current.Cfg))
		require.Equal(t, configLookupSnapshot(AppendConfig(parent, old.Cfg)), configLookupSnapshot(AppendConfig(parent, current.Cfg)))
	}
}
