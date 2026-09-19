package base

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigAtomicWritePreservesConcurrentReplay(t *testing.T) {
	cfg := NewEmptyConfig()
	const workers, writes = 16, 100
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < writes; i++ {
				cfg.SetItem(fmt.Sprintf("worker-%d", worker), i)
			}
		}(worker)
	}
	wg.Wait()
	options := cfg.GetItem(CfgOptionFuns).([]NodeConfigFun)
	require.Len(t, options, workers*writes, "no replay entry may be lost")
	replayed := NewEmptyConfig()
	for _, option := range options {
		option(replayed)
	}
	require.Equal(t, configLookupSnapshot(cfg), configLookupSnapshot(replayed))
}

func TestBareTerminalConfigAndReplay(t *testing.T) {
	parent := NewEmptyConfig()
	parent.SetItem("endian", "little")
	parent.SetItem("parser", "default")
	parent.SetItem("unit", "bit")
	for _, typ := range []string{"uint8", "raw", "", "Record...", "...", " uint8 "} {
		t.Run(typ, func(t *testing.T) {
			cfg := configLegacyInherit(parent)
			configLegacySet(cfg, CfgIsTerminal, true)
			name := typ
			if len(typ) >= 3 && typ[len(typ)-3:] == "..." {
				name = typ[:len(typ)-3]
				configLegacySet(cfg, CfgIsList, true)
			}
			configLegacySet(cfg, CfgType, name)
			node, err := NewNodeTreeWithConfig(parent, "Field", typ, nil)
			require.NoError(t, err)
			require.Equal(t, typ, node.Origin)
			require.Equal(t, configLookupSnapshot(cfg), configLookupSnapshot(node.Cfg))
			require.Equal(t, configLookupSnapshot(configLegacyAppend(parent, cfg)), configLookupSnapshot(AppendConfig(parent, node.Cfg)))
		})
	}
	for _, typ := range []string{"raw,8", "uint8,3bit", "type:string;del:end", "endian:big;type:uint16"} {
		n, err := NewNodeTreeWithConfig(parent, "Field", typ, nil)
		require.NoError(t, err, typ)
		require.NotEqual(t, typ, n.Cfg.GetString(CfgType), "options must not take the bare type path")
	}
	_, err := NewNodeTreeWithConfig(parent, "Field", "raw,invalid", nil)
	require.ErrorContains(t, err, "parse length error")
}
