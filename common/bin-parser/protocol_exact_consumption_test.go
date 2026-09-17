package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

type exactMessageReader struct {
	*bytes.Reader
	bitLength uint64
}

func newExactMessageReader(input []byte) *exactMessageReader {
	return &exactMessageReader{
		Reader:    bytes.NewReader(input),
		bitLength: uint64(len(input)) * 8,
	}
}

func (r *exactMessageReader) InputBitLength() uint64 {
	return r.bitLength
}

func requireExactMessageParse(t *testing.T, input []byte, rule, entry string) {
	t.Helper()
	reader := newExactMessageReader(input)
	_, err := parser.ParseBinary(reader, rule, entry)
	require.NoError(t, err)
	require.Zero(t, reader.Len(), "parser must consume the complete bounded input")
}

func requireExactMessageReject(t *testing.T, input []byte, rule, entry string) {
	t.Helper()
	_, err := parser.ParseBinary(newExactMessageReader(input), rule, entry)
	require.Error(t, err)
}

func TestC1222MessageExactLength(t *testing.T) {
	message, err := hex.DecodeString("6030a20f060d607c86f754011600010140ce11a60e060c607c86f75401160001014021a80402020f89be0728058103800120")
	require.NoError(t, err)

	requireExactMessageParse(t, message, "application-layer.extended_protocols", "C1222Message")

	t.Run("truncated message", func(t *testing.T) {
		requireExactMessageReject(t, message[:len(message)-1], "application-layer.extended_protocols", "C1222Message")
	})
	t.Run("outer length mismatch", func(t *testing.T) {
		malformed := append([]byte(nil), message...)
		malformed[1]--
		requireExactMessageReject(t, malformed, "application-layer.extended_protocols", "C1222Message")
	})
}

func dhcpMessage(optionsAndPadding ...byte) []byte {
	message := make([]byte, 240, 240+len(optionsAndPadding))
	message[0] = 1
	message[1] = 1
	binary.BigEndian.PutUint32(message[236:240], 0x63825363)
	return append(message, optionsAndPadding...)
}

func TestDHCPOptionsEndPadding(t *testing.T) {
	message := dhcpMessage(53, 1, 1, 255, 0, 0, 0)
	requireExactMessageParse(t, message, "application-layer.dhcp", "DHCP")

	t.Run("non-zero byte after end", func(t *testing.T) {
		requireExactMessageReject(t, dhcpMessage(53, 1, 1, 255, 0, 1), "application-layer.dhcp", "DHCP")
	})
	t.Run("truncated option value", func(t *testing.T) {
		requireExactMessageReject(t, dhcpMessage(12, 5, 'a', 'b'), "application-layer.dhcp", "DHCP")
	})
	t.Run("fixed option length mismatch", func(t *testing.T) {
		requireExactMessageReject(t, dhcpMessage(53, 2, 1, 2, 255), "application-layer.dhcp", "DHCP")
	})
}

func compoundRTCPMessage() []byte {
	message := make([]byte, 112)
	message[0] = 0x81
	message[1] = 200
	binary.BigEndian.PutUint16(message[2:4], 12)
	message[52] = 0x81
	message[53] = 202
	binary.BigEndian.PutUint16(message[54:56], 14)
	return message
}

func TestRTCPCompoundPacketLengths(t *testing.T) {
	message := compoundRTCPMessage()
	requireExactMessageParse(t, message, "rtp", "RTCP")

	t.Run("truncated first packet", func(t *testing.T) {
		requireExactMessageReject(t, message[:51], "rtp", "RTCP")
	})
	t.Run("report count exceeds first packet", func(t *testing.T) {
		malformed := append([]byte(nil), message...)
		malformed[0] = 0x82
		requireExactMessageReject(t, malformed, "rtp", "RTCP")
	})
	t.Run("compound packet length exceeds input", func(t *testing.T) {
		malformed := append([]byte(nil), message...)
		binary.BigEndian.PutUint16(malformed[54:56], 15)
		requireExactMessageReject(t, malformed, "rtp", "RTCP")
	})
}

func TestUSBHIDReportBounds(t *testing.T) {
	const rule = "application-layer.extended_protocols"
	const entry = "USBHIDReport"

	// HID report sizes are descriptor-defined. Exercise both an ID-only report
	// and the complete four-byte report carried by the pinned upstream capture.
	requireExactMessageParse(t, []byte{0x01}, rule, entry)
	requireExactMessageParse(t, []byte{0x01, 0xff, 0xff, 0xff}, rule, entry)

	_, err := parser.ParseBinary(newExactMessageReader(nil), rule, entry)
	require.ErrorContains(t, err, "usb-hid: report is empty")
}

func TestFTPAuthTLSExactCommand(t *testing.T) {
	const rule = "application-layer.ftp"
	const entry = "FTPAuthTLS"

	requireExactMessageParse(t, []byte("AUTH TLS\r\n"), rule, entry)

	t.Run("different command", func(t *testing.T) {
		requireExactMessageReject(t, []byte("USER TLS\r\n"), rule, entry)
	})
	t.Run("different mechanism", func(t *testing.T) {
		requireExactMessageReject(t, []byte("AUTH SSL\r\n"), rule, entry)
	})
	t.Run("missing terminator", func(t *testing.T) {
		requireExactMessageReject(t, []byte("AUTH TLS"), rule, entry)
	})
}
