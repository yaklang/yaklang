package stream_parser_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

const nestedReferenceRule = `
endian: big
Package:
  Message:
    First: Record
    Second: Record
Record:
  Length: LengthField
  Body:
    list: true
    list-length-from-field: "../Length/Value"
    Byte: ByteField
LengthField:
  Value: uint8
ByteField:
  Value: uint8
`

func TestNestedReferencesInitializeEveryInstance(t *testing.T) {
	for repeat := 0; repeat < 3; repeat++ {
		root, _, reader := parseInlineRule(t, nestedReferenceRule, []byte{2, 0x11, 0x22, 1, 0x33})
		require.Zero(t, reader.Len())
		for path, want := range map[string]uint8{"@Message.First.Length.Value": 2, "@Message.Second.Length.Value": 1} {
			node := base.GetNodeByPath(root, path)
			require.NotNil(t, node)
			value, err := node.Result()
			require.NoError(t, err)
			require.Equal(t, want, value.Value)
		}
		for _, name := range []string{"Record", "LengthField", "ByteField"} {
			for _, template := range root.Children {
				if template.Name == name {
					for _, child := range template.Children {
						require.False(t, child.Cfg.Has(base.CfgNodeResult), "%s template was mutated", name)
					}
				}
			}
		}
	}
}

func TestNestedReferenceTruncationIsNotAnUnknownType(t *testing.T) {
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(nestedReferenceRule), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(16))
	err = root.Parse(base.NewBitReader(bytes.NewReader([]byte{2, 0x11})))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "not support type")
	require.ErrorContains(t, err, "list ended after 1 of 2 elements")
}
