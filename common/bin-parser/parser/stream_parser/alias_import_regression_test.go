package stream_parser_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	_ "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

const aliasToImportRule = `
endian: big
Package:
  Message:
    operator: |
      this.ProcessSubNode("Field")
    Field: EthernetAlias
EthernetAlias: "import:ethernet.yaml;node:Ethernet"
`

func TestAliasToImportResolutionDoesNotRestoreResolvedAlias(t *testing.T) {
	root := aliasImportRuleTree(t, aliasToImportRule)
	parser := &stream_parser.DefParser{}
	require.NoError(t, parser.OnRoot(root))

	field := base.GetNodeByPath(root, "@Message.Field")
	require.NotNil(t, field)
	require.Equal(t, "EthernetAlias", field.Cfg.GetString(stream_parser.CfgRefType))

	resolved, err := stream_parser.ParseRefNode(field)
	require.NoError(t, err)
	require.True(t, resolved.Cfg.Has(stream_parser.CfgImport))
	require.NoError(t, stream_parser.InitNode(resolved))
	require.False(t, resolved.Cfg.Has(stream_parser.CfgRefType))
	require.Empty(t, resolved.Cfg.GetString(stream_parser.CfgType))
}

func TestAliasToImportParsesAndGeneratesCompleteMessage(t *testing.T) {
	wire := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55,
		0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb,
		0x00, 0x03, 0x00, 0x00, 0x03,
	}

	parsed, _, reader := parseInlineRule(t, aliasToImportRule, wire)
	require.Zero(t, reader.Len())
	field := base.GetNodeByPath(parsed, "@Message.Field")
	require.NotNil(t, field)
	result, err := field.Result()
	require.NoError(t, err)
	typeValue := result.Child("Type")
	require.NotNil(t, typeValue)
	require.Equal(t, uint16(3), typeValue.Value)

	generated := aliasImportRuleTree(t, aliasToImportRule)
	require.NoError(t, generated.GenerateSubNode(map[string]any{
		"Field": map[string]any{
			"Destination": wire[:6],
			"Source":      wire[6:12],
			"Type":        3,
			"LLC":         map[string]any{"DSAP": 0, "SSAP": 0, "Control": 3},
		},
	}, "Message"))
	buffer, ok := generated.Ctx.GetItem("buffer").(*bytes.Buffer)
	require.True(t, ok)
	require.Equal(t, wire, buffer.Bytes())
}

func TestReferenceOverridesAndListGenerationSurviveResolution(t *testing.T) {
	const rule = `
endian: big
Package:
  Message:
    Records:
      list: true
      list-length: 2
      Record: RecordAlias
    Overridden:
      type: ScalarAlias
      endian: little
      length: 8
RecordAlias:
  Value: uint8
ScalarAlias: uint16
`
	root := aliasImportRuleTree(t, rule)
	parser := &stream_parser.DefParser{}
	require.NoError(t, parser.OnRoot(root))
	overridden := base.GetNodeByPath(root, "@Message.Overridden")
	require.NotNil(t, overridden)
	resolved, err := stream_parser.ParseRefNode(overridden)
	require.NoError(t, err)
	require.Equal(t, "uint16", resolved.Cfg.GetString(stream_parser.CfgType))
	require.Equal(t, "little", resolved.Cfg.GetString(stream_parser.CfgEndian))
	require.Equal(t, uint64(8), resolved.Cfg.GetUint64(stream_parser.CfgLength))

	generated := aliasImportRuleTree(t, rule)
	require.NoError(t, generated.GenerateSubNode(map[string]any{
		"Records": []map[string]any{
			{"Value": 0x11},
			{"Value": 0x22},
		},
		"Overridden": 0x33,
	}, "Message"))
	buffer, ok := generated.Ctx.GetItem("buffer").(*bytes.Buffer)
	require.True(t, ok)
	require.Equal(t, []byte{0x11, 0x22, 0x33}, buffer.Bytes())
}

func aliasImportRuleTree(t *testing.T, source string) *base.Node {
	t.Helper()
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	return root
}
