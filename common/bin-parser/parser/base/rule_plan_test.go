package base

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/rules"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func rulePlanTestSnapshot(n *Node) any {
	config := []configEntry{}
	n.Cfg.data.ForEach(func(k string, v any) bool {
		if k != CfgOptionFuns {
			config = append(config, configEntry{k, v})
		}
		return true
	})
	history := []any{}
	if callbacks, ok := n.Cfg.GetItem(CfgOptionFuns).([]NodeConfigFun); ok {
		for _, cb := range callbacks {
			target := NewEmptyConfig()
			cb(target)
			values := []configEntry{}
			target.data.ForEach(func(k string, v any) bool {
				if k != CfgOptionFuns {
					values = append(values, configEntry{k, v})
				}
				return true
			})
			history = append(history, values)
		}
	}
	var children []any
	for _, child := range n.Children {
		children = append(children, rulePlanTestSnapshot(child))
	}
	return []any{n.Name, n.Origin, config, history, children, n.Children == nil}
}

func TestRulePlanAllEmbeddedConstruction(t *testing.T) {
	count := 0
	err := fs.WalkDir(rules.RuleFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		data, err := rules.RuleFS.ReadFile(path)
		require.NoError(t, err)
		var doc yaml.MapSlice
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil
		} // ParseRule rejects malformed YAML before compilation.
		t.Run(path, func(t *testing.T) {
			old, oldErr := NewNodeTree(cloneRuleDocumentValue(doc).(yaml.MapSlice))
			current, err := instantiateRuleDocument(path, doc)
			require.Equal(t, fmt.Sprint(oldErr), fmt.Sprint(err))
			if err == nil {
				require.Equal(t, rulePlanTestSnapshot(old), rulePlanTestSnapshot(current))
			}
		})
		count++
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, count, 100)
}

func TestRulePlanConfigOrderAndCacheReplacement(t *testing.T) {
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte("Package:\n  Before: uint8\n  endian: little\n  After: 'type:uint16;length:16;type:uint32;length:7'\n  metadata:\n    tags: [original]\n"), &doc))
	n, err := instantiateRuleDocument(t.Name(), doc)
	require.NoError(t, err)
	require.Equal(t, "big", n.Children[0].Children[0].Cfg.GetString("endian"))
	require.Equal(t, "little", n.Children[0].Children[1].Cfg.GetString("endian"))
	metadata := n.Children[0].Cfg.GetItem("metadata").(yaml.MapSlice)
	metadata[0].Value.([]any)[0] = "edited"
	origin := n.Children[0].Origin.(yaml.MapSlice)
	require.Equal(t, "edited", origin[3].Value.(yaml.MapSlice)[0].Value.([]any)[0])
	next, err := instantiateRuleDocument(t.Name(), doc)
	require.NoError(t, err)
	require.Equal(t, "original", next.Children[0].Cfg.GetItem("metadata").(yaml.MapSlice)[0].Value.([]any)[0])
	var replaced yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte("Package:\n  Changed: raw\n"), &replaced))
	next, err = instantiateRuleDocument(t.Name(), replaced)
	require.NoError(t, err)
	require.Equal(t, "Changed", next.Children[0].Children[0].Name)
}
