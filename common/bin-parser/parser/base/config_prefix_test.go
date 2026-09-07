package base

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigPrefixPrivateHistoryAndExposure(t *testing.T) {
	for _, endian := range []string{"big", "little"} {
		for _, unit := range []string{"", "byte"} {
			for _, typ := range configPrefixTypes {
				for _, expose := range []string{"replay", "overwrite", "delete", "callbacks", "unknown", "invalid"} {
					t.Run(fmt.Sprint(endian, "/", unit, "/", typ, "/", expose), func(t *testing.T) {
						parent := NewEmptyConfig()
						parent.SetItems(ConfigItem{CfgEndian, endian}, ConfigItem{"parser", "default"})
						if unit != "" {
							parent.SetItem("unit", unit)
						}
						var items []ConfigItem
						if typ != "" {
							items = []ConfigItem{{CfgIsTerminal, true}, {CfgType, typ}}
						}
						cfg := NewConfigWithItems(parent, items...)
						sibling := NewNodeBatch(1).NewNode("Field", nil, parent, nil, items...).Cfg
						old := configLegacyInherit(parent)
						for _, item := range items {
							configLegacySet(old, item.Key, item.Value)
						}
						require.NotNil(t, cfg.data.prefix)
						require.Same(t, cfg.data.prefix, sibling.data.prefix)
						checkReplay := func() {
							require.Equal(t, configLookupSnapshot(configLegacyAppend(NewEmptyConfig(), old)), configLookupSnapshot(AppendConfig(NewEmptyConfig(), cfg)))
						}
						checkReplay()
						switch expose {
						case "overwrite":
							cfg.SetItem(CfgEndian, nil)
							configLegacySet(old, CfgEndian, nil)
							cfg.BaseKV.SetItem(CfgEndian, "current-only")
							old.BaseKV.SetItem(CfgEndian, "current-only")
						case "delete":
							cfg.DeleteItem(CfgEndian)
							old.DeleteItem(CfgEndian)
						case "callbacks":
							for _, c := range []*Config{cfg, old} {
								c.GetItem(CfgOptionFuns).([]NodeConfigFun)[0] = func(target *Config) { target.SetItem(CfgEndian, "edited") }
							}
						case "unknown":
							value := map[string]any{"mutable": []int{1}}
							cfg.SetItem("extension", value)
							configLegacySet(old, "extension", value)
						case "invalid":
							require.Panics(t, func() { cfg.SetItems(ConfigItem{CfgOptionFuns, nil}, ConfigItem{CfgLength, 4}) })
							require.Panics(t, func() { configLegacySet(old, CfgOptionFuns, nil) })
							require.True(t, cfg.Has(CfgOptionFuns))
							require.Nil(t, cfg.GetItem(CfgOptionFuns))
							require.False(t, cfg.Has(CfgLength))
							cfg.DeleteItem(CfgOptionFuns)
							old.DeleteItem(CfgOptionFuns)
						}
						checkReplay()
						require.Equal(t, configLookupSnapshot(old), configLookupSnapshot(cfg))
						require.Equal(t, endian, sibling.GetItem(CfgEndian))
						require.Equal(t, configLookupSnapshot(NewConfigWithItems(parent, items...)), configLookupSnapshot(sibling))
					})
				}
			}
		}
	}
}
