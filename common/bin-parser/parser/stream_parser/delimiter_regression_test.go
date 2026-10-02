package stream_parser_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestDelimitedFieldWireLengthIncludesOnlyConsumedFraming(t *testing.T) {
	for _, test := range []struct{ input, delimiter, value string }{
		{"ab\r\nXY", "\r\n", "ab"},
		{"aaabXY", "aab", "a"},
		{"abababacXY", "ababac", "ab"},
		{"\nXY", "\n", ""},
	} {
		t.Run(fmt.Sprintf("%q", test.delimiter), func(t *testing.T) {
			consumed := len(test.input) - 2
			source := fmt.Sprintf(`
Package:
  Message:
    operator: |
      this.ProcessSubNode("Line")
      if this.Length() != %d { panic("wire length excludes delimiter") }
      if this.GetSubNode("Tail").GetMaxLength() != 2 { panic("incorrect remaining boundary") }
      this.ProcessSubNode("Tail")
    Line:
      type: string
      del: %q
    Tail: raw
`, consumed, test.delimiter)
			root, _, reader := parseInlineRule(t, source, []byte(test.input))
			require.Zero(t, reader.Len())
			line := base.GetNodeByPath(root, "@Message.Line")
			value, err := line.Result()
			require.NoError(t, err)
			require.Equal(t, test.value, value.Value)
			require.EqualValues(t, consumed*8, stream_parser.CalcNodeConsumedLength(line))
			require.EqualValues(t, len(test.value)*8, stream_parser.CalcNodeResultLength(line), "wire length must not alter the public value")
			tail, err := base.GetNodeByPath(root, "@Message.Tail").Result()
			require.NoError(t, err)
			require.Equal(t, []byte("XY"), tail.Value)
		})
	}
}

func TestRequiredDelimiterCannotReadOutsideDeclaredField(t *testing.T) {
	const source = `
Package:
  Message:
    Line:
      type: string
      length: 24
      del: "\r\n"
    Tail: raw
`
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(40))
	reader := bytes.NewReader([]byte("ab\r\nZ"))
	err = root.Parse(base.NewBitReader(reader))
	require.ErrorContains(t, err, "delimiter not found within field boundary")
	require.Equal(t, 2, reader.Len(), "the following field must not be read while seeking a delimiter")
}

func TestOptionalDelimiterAtFieldBoundaryDoesNotConsumeNextField(t *testing.T) {
	const source = `
Package:
  Message:
    Line:
      type: string
      length: 16
      del: "\n"
      delimiter-optional: true
    Tail: uint8
`
	for _, input := range []string{"AB\n", "A\nZ"} {
		root, _, reader := parseInlineRule(t, source, []byte(input))
		require.Zero(t, reader.Len())
		line := base.GetNodeByPath(root, "@Message.Line")
		require.EqualValues(t, 16, stream_parser.CalcNodeConsumedLength(line))
		value, err := line.Result()
		require.NoError(t, err)
		require.Equal(t, string(bytes.TrimSuffix([]byte(input[:2]), []byte{'\n'})), value.Value)
		tail, err := base.GetNodeByPath(root, "@Message.Tail").Result()
		require.NoError(t, err)
		require.Equal(t, input[2], tail.Value)
	}
}
