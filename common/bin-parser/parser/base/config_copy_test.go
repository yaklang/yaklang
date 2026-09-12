package base

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigCopyKeepsHistoryCapacityAndAliases(t *testing.T) {
	for _, count := range []int{0, 1, 2, 5, 8, 9, 16, 32, 257} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			var sources, copies [2]*Config
			for mode := 0; mode < 2; mode++ {
				source := NewEmptyConfig()
				sources[mode] = source
				for i := 0; i < count; i++ {
					source.SetItem(fmt.Sprint(i%19), i)
				}
				source.SetItem("shared", []byte{1, 2, 3})
				if mode == 0 {
					copies[mode] = NewEmptyConfig()
					source.data.ForEach(func(k string, v any) bool { copies[mode].SetItem(k, v); return true })
				} else {
					copies[mode] = CopyConfig(source)
				}
				source.GetItem("shared").([]byte)[0] = 9
				journal := source.GetItem(CfgOptionFuns).([]NodeConfigFun)
				journal[0] = func(c *Config) { c.SetItem("edited callback", true) }
				source.SetItem("after-copy", 42)
			}
			for _, pair := range [][2]*Config{sources, copies} {
				a, b := pair[0].GetItem(CfgOptionFuns).([]NodeConfigFun), pair[1].GetItem(CfgOptionFuns).([]NodeConfigFun)
				require.Equal(t, len(a), len(b))
				require.Equal(t, cap(a), cap(b))
				require.Equal(t, configLookupSnapshot(pair[0]), configLookupSnapshot(pair[1]))
				require.Equal(t, configLookupSnapshot(AppendConfig(NewEmptyConfig(), pair[0])), configLookupSnapshot(AppendConfig(NewEmptyConfig(), pair[1])))
			}
		})
	}
}
