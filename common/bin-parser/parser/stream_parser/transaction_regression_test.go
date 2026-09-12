package stream_parser_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	_ "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func parseInlineRule(t *testing.T, source string, input []byte) (*base.Node, *base.BitReader, *bytes.Reader) {
	t.Helper()

	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(len(input)*8))

	reader := bytes.NewReader(input)
	bitReader := base.NewBitReader(reader)
	require.NoError(t, root.Parse(bitReader))
	return root, bitReader, reader
}

func TestListCommitsEveryElementBackup(t *testing.T) {
	_, bitReader, reader := parseInlineRule(t, `
Package:
  Bytes:
    list: true
    list-length: 3
    Byte: uint8
`, []byte{0x11, 0x22, 0x33})

	require.Zero(t, reader.Len())
	_, err := bitReader.ReadBits(8)
	require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
	require.ErrorContains(t, bitReader.Recovery(), "no backup")
}

func TestFixedLengthListRejectsMissingElement(t *testing.T) {
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(`
Package:
  Bytes:
    list: true
    list-length: 2
    Item:
      Byte: uint8
`), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(8))

	err = root.Parse(base.NewBitReader(bytes.NewReader([]byte{0x11})))
	require.Error(t, err)
	require.ErrorContains(t, err, "list ended after 1 of 2 elements")
}

func TestNestedListStopStateDoesNotEndOuterFixedList(t *testing.T) {
	const rule = `
Package:
  Outer:
    list: true
    list-length: 2
    Item:
      Inner:
        list: true
        Entry:
          operator: |
            marker = this.ProcessSubNode("Marker").Value
            if marker == 0 {
              setCtx("inList", false)
            }
          Marker: uint8
      Value: uint16
`

	t.Run("two complete outer elements", func(t *testing.T) {
		_, bitReader, reader := parseInlineRule(t, rule, []byte{
			0x00, 0x12, 0x34,
			0x00, 0x56, 0x78,
		})
		require.Zero(t, reader.Len())
		require.ErrorContains(t, bitReader.Recovery(), "no backup")
	})

	t.Run("one trailing byte cannot stand in for second element", func(t *testing.T) {
		var document yaml.MapSlice
		require.NoError(t, yaml.Unmarshal([]byte(rule), &document))
		root, err := base.NewNodeTree(document)
		require.NoError(t, err)
		input := []byte{0x00, 0x12, 0x34, 0xff}
		root.Cfg.SetItem(base.CfgLength, uint64(len(input)*8))

		err = root.Parse(base.NewBitReader(bytes.NewReader(input)))
		require.Error(t, err)
	})
}

func TestVRRPv2AuthenticationProbeCommitsBackup(t *testing.T) {
	input := []byte{
		0x21, 0x01, 0x64, 0x01,
		0x01, 0x01, 0x00, 0x00,
		0xc0, 0xa8, 0x00, 0x01,
		0x11, 0x22, 0x33, 0x44,
		0x55, 0x66, 0x77, 0x88,
	}
	root, err := base.ParseRule("vrrp.yaml")
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(len(input)*8))

	reader := bytes.NewReader(input)
	bitReader := base.NewBitReader(reader)
	require.NoError(t, root.Parse(bitReader))
	require.Zero(t, reader.Len())

	dataOne := base.GetNodeByPath(root, "@VRRP.Authentication Data.Data One")
	require.NotNil(t, dataOne)
	value, err := dataOne.Result()
	require.NoError(t, err)
	require.Equal(t, uint32(0x11223344), value.Value)

	dataTwo := base.GetNodeByPath(root, "@VRRP.Authentication Data.Data Two")
	require.NotNil(t, dataTwo)
	value, err = dataTwo.Result()
	require.NoError(t, err)
	require.Equal(t, uint32(0x55667788), value.Value)
	require.ErrorContains(t, bitReader.Recovery(), "no backup")
}

func TestVRRPv2TruncatedDeclaredAuthenticationProbeRecoversBackup(t *testing.T) {
	input := []byte{
		0x21, 0x01, 0x64, 0x01,
		0x00, 0x01, 0xba, 0x52,
		0xc0, 0xa8, 0x00, 0x01,
	}
	root, err := base.ParseRule("vrrp.yaml")
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(20*8))

	reader := bytes.NewReader(input)
	bitReader := base.NewBitReader(reader)
	require.NoError(t, root.Parse(bitReader))
	require.Zero(t, reader.Len())
	require.ErrorContains(t, bitReader.Recovery(), "no backup")
}

func TestLargeDeclaredOptionalProbeFailureRecoversBackup(t *testing.T) {
	tests := []struct {
		name          string
		rule          string
		input         []byte
		declaredBytes uint64
	}{
		{
			name:          "DHCPv6 option",
			rule:          "dhcpv6.yaml",
			input:         []byte{0x01, 0x00, 0x00, 0x01},
			declaredBytes: 4 + 1048576,
		},
		{
			name:          "loopback function",
			rule:          "loopback.yaml",
			input:         []byte{0x00, 0x00},
			declaredBytes: 2 + 1048576,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, err := base.ParseRule(tt.rule)
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, tt.declaredBytes*8)

			reader := bytes.NewReader(tt.input)
			bitReader := base.NewBitReader(reader)
			require.NoError(t, root.Parse(bitReader))
			require.Zero(t, reader.Len())
			require.ErrorContains(t, bitReader.Recovery(), "no backup")
		})
	}
}

func TestDNSRecordNameProbeFailureRecoversBeforePanic(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "answer", path: "@DNS.Answers.Answer.Name"},
		{name: "authority", path: "@DNS.Authority.Record.Name"},
		{name: "additional", path: "@DNS.Additional.Record.Name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, err := base.ParseRule("application-layer/dns.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(8))

			reader := bytes.NewReader(nil)
			bitReader := base.NewBitReader(reader)
			require.Error(t, root.ParseSubNode(bitReader, tt.path))
			require.Zero(t, reader.Len())
			require.ErrorContains(t, bitReader.Recovery(), "no backup")
		})
	}
}

func TestStopListProbeRecoveryRestoresCompleteInput(t *testing.T) {
	input := []byte{0x11, 0x22, 0x33, 0x44}
	root, bitReader, reader := parseInlineRule(t, `
Package:
  Message:
    operator: |
      result, probe = this.TryProcessByType("PartialList")
      probe.Recovery()
      this.ProcessSubNode("Payload")
    Payload: raw
PartialList:
  list: true
  exception-plan: stopList
  Item: PartialItem
PartialItem:
  operator: |
    this.ProcessSubNode("Prefix")
    this.ProcessSubNode("Oversized")
  Prefix: uint8
  Oversized: raw,8
`, input)

	payload := base.GetNodeByPath(root, "@Message.Payload")
	require.NotNil(t, payload)
	value, err := payload.Result()
	require.NoError(t, err)
	require.Equal(t, input, value.Value)
	require.Zero(t, reader.Len())
	_, err = bitReader.ReadBits(8)
	require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF), "probe recovery left duplicated input readable")
	require.ErrorContains(t, bitReader.Recovery(), "no backup")
}

func TestTryProcessSubNodeResultFailureKeepsRecovery(t *testing.T) {
	input := []byte{0x11, 0x22, 0x33, 0x44}
	root, bitReader, reader := parseInlineRule(t, `
Package:
  Message:
    operator: |
      result, probe = this.TryProcessSubNode("PartialList")
      probe.Recovery()
      this.ProcessSubNode("Payload")
    PartialList:
      list: true
      exception-plan: stopList
      Item: PartialItem
    Payload: raw
PartialItem:
  operator: |
    this.ProcessSubNode("Prefix")
    this.ProcessSubNode("Oversized")
  Prefix: uint8
  Oversized: raw,8
`, input)

	payload := base.GetNodeByPath(root, "@Message.Payload")
	require.NotNil(t, payload)
	value, err := payload.Result()
	require.NoError(t, err)
	require.Equal(t, input, value.Value)
	require.Zero(t, reader.Len())
	_, err = bitReader.ReadBits(8)
	require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
	require.ErrorContains(t, bitReader.Recovery(), "no backup")
}

func TestCurrentPositionIncludesDelimiterForTailLength(t *testing.T) {
	root, bitReader, reader := parseInlineRule(t, `
Package:
  Message:
    operator: |
      start = getCurrentPosition()
      this.ProcessSubNode("Line")
      consumed = getCurrentPosition() - start
      remaining = this.GetMaxLength() - consumed
      if remaining > 0 {
        this.GetSubNode("Tail").SetMaxLength(remaining)
        this.ProcessSubNode("Tail")
      }
    Line: "type:string;del:\r\n"
    Tail: raw
`, []byte("A\r\nZ"))

	line := base.GetNodeByPath(root, "@Message.Line")
	require.NotNil(t, line)
	lineValue, err := line.Result()
	require.NoError(t, err)
	require.Equal(t, "A", lineValue.Value)

	tail := base.GetNodeByPath(root, "@Message.Tail")
	require.NotNil(t, tail)
	tailValue, err := tail.Result()
	require.NoError(t, err)
	require.Equal(t, []byte("Z"), tailValue.Value)
	require.Zero(t, reader.Len())
	_, err = bitReader.ReadBits(8)
	require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
	require.ErrorContains(t, bitReader.Recovery(), "no backup")
}
