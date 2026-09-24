package bin_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestProtocolCorpusUSBHIDFullCapture(t *testing.T) {
	const capturePath = "testdata/protocol-corpus/captures/wireshark-tests/wireshark-usb-hid.pcapng"
	captureData := readProtocolCorpusFile(t, ".", capturePath)
	reader, err := pcapgo.NewNgReader(bytes.NewReader(captureData), pcapgo.NgReaderOptions{SkipUnknownVersion: true})
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeLinuxUSB, reader.LinkType())

	type expectedURB struct {
		event        byte
		transferType byte
		urbID        uint64
		urbLength    uint32
		dataLength   uint32
		payload      []byte
	}
	expected := []expectedURB{
		{event: 'S', transferType: 2, urbID: 0xffff8eb264422e40, urbLength: 41},
		{event: 'C', transferType: 2, urbID: 0xffff8eb264422e40, urbLength: 41, dataLength: 41},
		{event: 'S', transferType: 2, urbID: 0xffff8eb264422d80, urbLength: 65},
		{event: 'C', transferType: 2, urbID: 0xffff8eb264422d80, urbLength: 65, dataLength: 65},
		{event: 'C', transferType: 1, urbID: 0xffff8eb264422780, urbLength: 4, dataLength: 4, payload: []byte{0x01, 0xff, 0xff, 0xff}},
		{event: 'S', transferType: 1, urbID: 0xffff8eb264422780, urbLength: 4},
		{event: 'C', transferType: 1, urbID: 0xffff8eb264422780, urbLength: 4, dataLength: 4, payload: []byte{0x01, 0xd9, 0x3a, 0xf7}},
		{event: 'S', transferType: 1, urbID: 0xffff8eb264422780, urbLength: 4},
		{event: 'C', transferType: 1, urbID: 0xffff8eb264422780, urbLength: 4, dataLength: 4, payload: []byte{0x01, 0x00, 0x00, 0x00}},
		{event: 'S', transferType: 1, urbID: 0xffff8eb264422780, urbLength: 4},
	}

	frameNumber := 0
	parsedReports := 0
	for {
		frame, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		frameNumber++
		require.LessOrEqual(t, frameNumber, len(expected))
		want := expected[frameNumber-1]

		require.GreaterOrEqualf(t, len(frame), 64, "frame %d USB pseudo-header", frameNumber)
		require.Equalf(t, want.urbID, binary.LittleEndian.Uint64(frame[0:8]), "frame %d URB id", frameNumber)
		require.Equalf(t, want.event, frame[8], "frame %d event", frameNumber)
		require.Equalf(t, want.transferType, frame[9], "frame %d transfer type", frameNumber)
		wantEndpoint := byte(0x81)
		if want.transferType == 2 {
			wantEndpoint = 0x80
		}
		require.Equalf(t, wantEndpoint, frame[10], "frame %d endpoint", frameNumber)
		require.Equalf(t, byte(30), frame[11], "frame %d device", frameNumber)
		require.Equalf(t, uint16(4), binary.LittleEndian.Uint16(frame[12:14]), "frame %d bus", frameNumber)
		require.Equalf(t, want.urbLength, binary.LittleEndian.Uint32(frame[32:36]), "frame %d URB length", frameNumber)
		require.Equalf(t, want.dataLength, binary.LittleEndian.Uint32(frame[36:40]), "frame %d data length", frameNumber)
		require.Lenf(t, frame[64:], int(want.dataLength), "frame %d captured payload", frameNumber)

		if want.payload == nil {
			continue
		}
		require.Equalf(t, want.payload, frame[64:], "frame %d HID report", frameNumber)
		bounded := newProtocolCorpusBoundedReader(frame[64:])
		node, parseErr := protocolCorpusParseRule(bounded, protocolCorpusParseContract{
			RuleFile: "application-layer/extended_protocols.yaml", EntryNode: "USBHIDReport", Layer: "L7",
		})
		require.NoErrorf(t, parseErr, "frame %d", frameNumber)
		require.Zero(t, bounded.Len(), "frame %d HID bytes unread", frameNumber)
		terminals, coverageErr := protocolCorpusTerminalCoverage(node, want.payload)
		require.NoErrorf(t, coverageErr, "frame %d", frameNumber)
		require.Equal(t, 2, terminals, "frame %d terminal count", frameNumber)
		protocolCorpusRequireValue(t, node, "Report ID", uint64(1))
		protocolCorpusRequireValue(t, node, "Report Data", want.payload[1:])
		parsedReports++
	}

	require.Equal(t, len(expected), frameNumber)
	require.Equal(t, 3, parsedReports)
}
