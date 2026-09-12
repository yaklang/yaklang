package bin_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type telnetQUICExactReader struct {
	*bytes.Reader
	bitLength uint64
}

func newTelnetQUICExactReader(input []byte) *telnetQUICExactReader {
	return &telnetQUICExactReader{
		Reader:    bytes.NewReader(input),
		bitLength: uint64(len(input)) * 8,
	}
}

func (r *telnetQUICExactReader) InputBitLength() uint64 {
	return r.bitLength
}

func parseTelnetQUICExact(t *testing.T, input []byte, rule, entry string) *base.NodeValue {
	t.Helper()
	reader := newTelnetQUICExactReader(input)
	node, err := parser.ParseBinary(reader, rule, entry)
	require.NoError(t, err)
	require.Zero(t, reader.Len(), "parser must consume the complete bounded input")
	value, err := node.Result()
	require.NoError(t, err)
	require.NotNil(t, value)
	return value
}

func TestTelnetLegacyRootAndCompleteSequence(t *testing.T) {
	sequence := []byte{
		0xff, 0xfd, 0x03,
		0xff, 0xfb, 0x18,
		0xff, 0xfb, 0x1f,
		0xff, 0xfb, 0x20,
	}
	value := parseTelnetQUICExact(t, sequence, "telnet", "Telnet")

	iac := mustChild(t, value, "IAC")
	require.Equal(t, uint64(0xfd), uintVal(t, iac.Child("Command")))
	require.Equal(t, uint64(3), uintVal(t, iac.Child("Option")))
	items := mustChild(t, value, "Items")
	require.Len(t, items.Children(), 3)
}

func TestTelnetLegacySubnegotiationRootPaths(t *testing.T) {
	t.Run("terminal type", func(t *testing.T) {
		input := append([]byte{0xff, 0xfa, 0x18, 0x00}, append([]byte("xterm"), 0xff, 0xf0)...)
		value := parseTelnetQUICExact(t, input, "telnet", "Telnet")
		iac := mustChild(t, value, "IAC")
		require.Equal(t, "xterm", joinUint8(t, iac.Child("Terminal Type")))
	})

	t.Run("window size", func(t *testing.T) {
		input := []byte{0xff, 0xfa, 0x1f, 0x00, 0x50, 0x00, 0x18, 0xff, 0xf0}
		value := parseTelnetQUICExact(t, input, "telnet", "Telnet")
		iac := mustChild(t, value, "IAC")
		require.Equal(t, uint64(80), uintVal(t, iac.Child("Width")))
		require.Equal(t, uint64(24), uintVal(t, iac.Child("Height")))
	})
}

func TestQUICOptionalProtectedPayload(t *testing.T) {
	t.Run("long header without remaining payload", func(t *testing.T) {
		value := parseTelnetQUICExact(t, []byte{0x80, 0, 0, 0, 0, 0, 0}, "application-layer.quic", "QUIC")
		require.Nil(t, value.Child("Protected Payload"))
	})

	t.Run("short header without remaining payload", func(t *testing.T) {
		value := parseTelnetQUICExact(t, []byte{0x40}, "application-layer.quic", "QUIC")
		require.Nil(t, value.Child("Protected Payload"))
	})

	t.Run("short header with protected payload", func(t *testing.T) {
		value := parseTelnetQUICExact(t, []byte{0x40, 0x01, 0x02}, "application-layer.quic", "QUIC")
		payload := mustChild(t, value, "Protected Payload")
		require.Len(t, payload.Children(), 2)
		require.Equal(t, uint64(1), uintVal(t, payload.Children()[0]))
		require.Equal(t, uint64(2), uintVal(t, payload.Children()[1]))
	})
}
