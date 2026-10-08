package base

import (
	"testing"

	"github.com/stretchr/testify/require"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestNodeCopyPreservesOriginalParents(t *testing.T) {
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte("Package:\n  Message:\n    Byte: uint8\n"), &document))
	original, err := NewNodeTree(document)
	require.NoError(t, err)
	var linkParents func(*Node)
	linkParents = func(node *Node) {
		for _, child := range node.Children {
			child.Cfg.SetItem(CfgParent, node)
			linkParents(child)
		}
	}
	linkParents(original)
	for repeat := 0; repeat < 3; repeat++ {
		copied := original.Copy()
		var check func(*Node, *Node)
		check = func(source, clone *Node) {
			require.NotSame(t, source, clone)
			require.Len(t, clone.Children, len(source.Children))
			for i, child := range source.Children {
				require.Same(t, source, child.Cfg.GetItem(CfgParent))
				require.Same(t, clone, clone.Children[i].Cfg.GetItem(CfgParent))
				check(child, clone.Children[i])
			}
		}
		check(original, copied)
	}
}
