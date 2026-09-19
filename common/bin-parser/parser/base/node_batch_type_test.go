package base

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNodeBatchImmutableTypeDescriptors(t *testing.T) {
	parent := NewEmptyConfig()
	parent.SetItems(ConfigItem{CfgEndian, "little"}, ConfigItem{"parser", "default"}, ConfigItem{"unit", "byte"})
	for _, typ := range append(configPrefixTypes[:], "Type...", "raw,3", "type:uint16;endian:big", "raw,invalid") {
		items := []ConfigItem{{CfgNodeResult, [2]uint64{3, 17}}, {CfgLength, uint64(14)}, {CfgParent, parent}}
		old, oldErr := NewNodeTreeWithConfigItems(parent, "field", typ, nil, items...)
		current, err := NewNodeBatch(1).NewNodeTreeWithTypeItems(parent, "field", typ, nil, items...)
		require.Equal(t, fmt.Sprint(oldErr), fmt.Sprint(err))
		if err == nil {
			require.Equal(t, rulePlanTestSnapshot(old), rulePlanTestSnapshot(current))
		}
	}
}
