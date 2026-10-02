package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func protocolCorpusIPXFields(t *testing.T, root *base.Node, wire []byte) {
	t.Helper()
	node := protocolCorpusFindNode(root, "IPX")
	require.NotNil(t, node)
	require.GreaterOrEqual(t, len(wire), 30)
	require.EqualValues(t, len(wire), binary.BigEndian.Uint16(wire[2:]))
	for field, offset := range map[string]int{"Checksum": 0, "Length": 2, "Destination Socket": 16, "Source Socket": 28} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint16(wire[offset:])))
	}
	for field, offset := range map[string]int{"Transport Control": 4, "Packet Type": 5} {
		protocolCorpusRequireValue(t, node, field, uint64(wire[offset]))
	}
	for field, offset := range map[string]int{"Destination Network": 6, "Source Network": 18} {
		protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint32(wire[offset:])))
	}
	protocolCorpusRequireValue(t, node, "Destination Node", wire[10:16])
	protocolCorpusRequireValue(t, node, "Source Node", wire[22:28])
	if len(wire) > 30 {
		protocolCorpusRequireValue(t, node, "Payload", wire[30:])
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "Payload"))
	}
	require.Equal(t, false, node.Cfg.GetItem("additionInfo").(map[string]any)["Upper Layer Decoded"])
}

func TestProtocolCorpusIPXEveryRecordAndField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/google-samples/google-ipx-session.pcapng")
	require.Len(t, frames, 16827)
	trailers, sockets := map[int]int{}, map[uint16]int{}
	frameTrailers := map[int]int{}
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			require.GreaterOrEqual(t, len(frame), 47)
			linkLength := int(binary.BigEndian.Uint16(frame[12:]))
			require.Less(t, linkLength, 0x600)
			require.LessOrEqual(t, 14+linkLength, len(frame))
			require.Equal(t, []byte{0xe0, 0xe0, 3}, frame[14:17])
			length := int(binary.BigEndian.Uint16(frame[19:]))
			require.GreaterOrEqual(t, length, 30)
			require.LessOrEqual(t, 17+length, 14+linkLength)
			wire, trailer := frame[17:17+length], frame[17+length:14+linkLength]
			frameTrailer := frame[14+linkLength:]
			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			protocolCorpusRequireValue(t, envelope, "Destination", frame[:6])
			protocolCorpusRequireValue(t, envelope, "Source", frame[6:12])
			protocolCorpusRequireValue(t, envelope, "Type", uint64(linkLength))
			require.Equal(t, 3, protocolCorpusLLCFields(t, envelope, frame[14:]))
			protocolCorpusIPXFields(t, envelope, wire)
			if len(trailer) != 0 {
				protocolCorpusRequireValue(t, envelope, "IPX Link Trailer", trailer)
			} else {
				require.Nil(t, protocolCorpusFindNode(envelope, "IPX Link Trailer"))
			}
			if len(frameTrailer) != 0 {
				protocolCorpusRequireValue(t, envelope, "Frame Trailer", frameTrailer)
			} else {
				require.Nil(t, protocolCorpusFindNode(envelope, "Frame Trailer"))
			}
			require.Nil(t, protocolCorpusFindNode(envelope, "IPXPrefix"), "lookahead must not duplicate consumed bytes")
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.extended_protocols", "IPX")
			protocolCorpusIPXFields(t, node, wire)
			// The legacy public entry remains tested for every stored record.
			// The small standalone rule is now the carrier's canonical entry.
			standalone := protocolCorpusRequireBoundedRuleParse(t, wire, "ipx", "IPX")
			protocolCorpusIPXFields(t, standalone, wire)
			require.Zero(t, wire[5])
			socket := binary.BigEndian.Uint16(wire[16:])
			require.Equal(t, socket, binary.BigEndian.Uint16(wire[28:]))
			trailers[len(trailer)]++
			frameTrailers[len(frameTrailer)]++
			sockets[socket]++
		})
	}
	require.Equal(t, map[int]int{0: 16347, 1: 480}, trailers)
	require.Equal(t, map[int]int{0: 16824, 4: 3}, frameTrailers)
	require.Equal(t, map[uint16]int{0x17df: 56, 0x17e0: 16771}, sockets)
}

func TestProtocolCorpusIPXCarrierBoundaries(t *testing.T) {
	wire := make([]byte, 33)
	for i := range wire {
		wire[i] = byte(i*7 + 1)
	}
	binary.BigEndian.PutUint16(wire[2:], uint16(len(wire)))
	for _, header := range [][]byte{{0xe0, 0xe0, 3}, {0xe1, 0xe1, 0x13}, {0xe0, 0xe1, 2, 5}} {
		for _, trailer := range [][]byte{nil, {0x73}, {0x80, 0, 0xff}} {
			llc := append(append(bytes.Clone(header), wire...), trailer...)
			node := protocolCorpusRequireBoundedRuleParse(t, llc, "llc", "LLC")
			protocolCorpusIPXFields(t, node, wire)
			if len(trailer) != 0 {
				protocolCorpusRequireValue(t, node, "IPX Link Trailer", trailer)
			}
			for cut := 0; cut < len(header)+len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(llc[:cut]), "llc", "LLC")
				require.Errorf(t, err, "incomplete carrier/header/body at %d", cut)
			}
		}
	}
	for cut := 0; cut < len(wire); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.extended_protocols", "IPX")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0x73)), "application-layer.extended_protocols", "IPX")
	require.ErrorContains(t, err, "ipx: packet length does not match input", "trailing data is accepted only by a carrier, never by the strict direct entry")
	for _, length := range []uint16{0, 4, 29, 34, 65535} {
		b := append([]byte{0xe0, 0xe0, 3}, wire...)
		binary.BigEndian.PutUint16(b[5:], length)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(b), "llc", "LLC")
		require.ErrorContains(t, err, "llc: IPX length exceeds carrier or is shorter than its header")
	}
	// A complete empty-body IPX header is legitimate. Packet type and socket
	// values alone are not enough to infer or decode its upper-layer protocol.
	empty := bytes.Clone(wire[:30])
	binary.BigEndian.PutUint16(empty[2:], 30)
	protocolCorpusIPXFields(t, protocolCorpusRequireBoundedRuleParse(t, empty, "application-layer.extended_protocols", "IPX"), empty)
	_, err = parser.ParseBinary(bytes.NewBuffer(wire), "application-layer.extended_protocols", "IPX")
	require.ErrorContains(t, err, "ipx: packet boundary is required")
}
