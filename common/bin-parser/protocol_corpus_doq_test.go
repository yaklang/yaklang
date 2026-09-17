package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

const doqTestDraftQueryHex = "000000000001000000000001076578616d706c6503636f6d0000010001000029ffff000000000000"
const doqTestDraftResponseHex = "000080800001000100000001076578616d706c6503636f6d0000010001c00c000100010000542c00045db8d8220000290200000000000000"
const doqTestRFCQueryHex = "001d000001000001000000000000076578616d706c6503636f6d0000010001"
const doqTestRFCResponseHex = "002d000081800001000100000000076578616d706c6503636f6d0000010001c00c000100010000003c00045db8d822"

func doqTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.doq", entry)
	require.Equal(t, wire, NodeToBytes(n))
	h225TestTree(t, n, wire, 0)
	return n
}

func TestProtocolCorpusDoQAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-doq.pcapng"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "62f2bfb0ef5c35401b9c93dab4069b37043280bcc7f61452d86e6f9c61c1129d", fmt.Sprintf("%x", sha256.Sum256(data)))
	// Pin the presence of the original embedded DSB without logging or using its
	// keys in production. The clear stream fixtures were externally derived.
	require.Equal(t, uint32(0x1a2b3c4d), binary.LittleEndian.Uint32(data[8:]))
	dsb := 0
	for pos := 0; pos < len(data); {
		require.GreaterOrEqual(t, len(data)-pos, 12)
		typ, size := binary.LittleEndian.Uint32(data[pos:]), int(binary.LittleEndian.Uint32(data[pos+4:]))
		require.GreaterOrEqual(t, size, 12)
		require.LessOrEqual(t, pos+size, len(data))
		require.Equal(t, uint32(size), binary.LittleEndian.Uint32(data[pos+size-4:]))
		if typ == 10 {
			dsb++
		}
		pos += size
	}
	require.Equal(t, 1, dsb)
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 20)
	lengths := []int{1294, 1294, 541, 279, 117, 117, 117, 147, 147, 195, 147, 195, 147, 195, 147, 195, 147, 195, 147, 195}
	for i, record := range records {
		frame := i + 1
		require.Len(t, record, lengths[i], "frame %d", frame)
		packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer(), "frame %d", frame)
		ip := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
		require.Equal(t, "::1", ip.SrcIP.String())
		require.Equal(t, "::1", ip.DstIP.String())
		if frame >= 10 && frame%2 == 0 {
			require.Equal(t, layers.IPProtocolICMPv6, ip.NextHeader)
			// Each ICMPv6 message quotes the entire immediately preceding IPv6
			// datagram. It is not a new response on the DoQ stream.
			require.Equal(t, records[i-1][14:], record[62:])
			continue
		}
		require.Equal(t, layers.IPProtocolUDP, ip.NextHeader)
		udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
		if frame == 1 || frame == 4 || frame == 7 {
			require.Equal(t, layers.UDPPort(47826), udp.SrcPort)
			require.Equal(t, layers.UDPPort(784), udp.DstPort)
		} else {
			require.Equal(t, layers.UDPPort(784), udp.SrcPort)
			require.Equal(t, layers.UDPPort(47826), udp.DstPort)
		}
		// No encrypted packet is ever silently promoted to a clear DNS tree.
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(udp.Payload), "application-layer.doq", "DoQDraft00QueryStream")
		require.Error(t, err, "frame %d", frame)
	}
}

func TestProtocolCorpusDoQOriginalClearStreamFields(t *testing.T) {
	// Independently exported quic.stream_data from the original embedded DSB:
	// frame4 client->server stream0 offset0 FIN1 40B; frame8 server->client
	// stream0 offset0 FIN1 56B. Frames9/11/13/15/17/19 repeat all response bytes
	// at offset0; frames10/12/14/16/18/20 only quote those packets in ICMPv6.
	// ALPN frame1/2 is doq-i00 and QUIC version is ff000020 (draft32).
	query, response := h225TestHex(t, doqTestDraftQueryHex), h225TestHex(t, doqTestDraftResponseHex)
	require.Len(t, query, 40)
	require.Len(t, response, 56)
	n := doqTestParse(t, query, "DoQDraft00QueryStream")
	protocolCorpusRequireValue(t, n, "ID", uint64(0))
	protocolCorpusRequireValue(t, n, "Flags", uint64(0))
	protocolCorpusRequireValue(t, n, "Name", "example.com.")
	protocolCorpusRequireValue(t, n, "Type", uint64(1))
	protocolCorpusRequireValue(t, n, "UDP Payload Size", uint64(65535))
	protocolCorpusRequireValue(t, n, "EDNS Version", uint64(0))
	require.Equal(t, "draft-i00", n.Cfg.GetItem("additionInfo").(map[string]any)["Mapping"])
	for _, frame := range []int{8, 9, 11, 13, 15, 17, 19} {
		t.Run(fmt.Sprint(frame), func(t *testing.T) {
			n := doqTestParse(t, response, "DoQDraft00ResponseStream")
			protocolCorpusRequireValue(t, n, "ID", uint64(0))
			protocolCorpusRequireValue(t, n, "Flags", uint64(0x8080))
			protocolCorpusRequireValue(t, n, "Name", "example.com.")
			protocolCorpusRequireValue(t, n, "TTL", uint64(21548))
			protocolCorpusRequireValue(t, n, "Address", []byte{93, 184, 216, 34})
			protocolCorpusRequireValue(t, n, "UDP Payload Size", uint64(512))
			answers := protocolCorpusFindNode(n, "Answers")
			protocolCorpusRequireValue(t, answers, "Name", "example.com.")
		})
	}
	// These original draft bytes are not malformed draft messages; the final
	// RFC entry rejects their absent length field before interpreting DNS ID.
	for i, wire := range [][]byte{query, response} {
		entry := []string{"DoQQueryStream", "DoQResponseStream"}[i]
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.doq", entry)
		require.ErrorContains(t, err, "RFC 9250 message length must be at least 12")
	}
}

func TestProtocolCorpusDoQDerivedProvenanceAndAllFragments(t *testing.T) {
	var evidence struct {
		SourceCapture       string         `json:"source_capture"`
		SourceCaptureSHA256 string         `json:"source_capture_sha256"`
		Extraction          map[string]any `json:"extraction"`
		FragmentCount       int            `json:"fragment_count"`
		StreamCount         int            `json:"stream_count"`
		Streams             []struct {
			ID                 uint64 `json:"stream_id"`
			SourceAddress      string `json:"source_address"`
			DestinationAddress string `json:"destination_address"`
			SourcePort         uint16 `json:"source_port"`
			DestinationPort    uint16 `json:"destination_port"`
			Length             int    `json:"length"`
			SHA256             string `json:"sha256"`
			Hex                string `json:"hex"`
			FIN                bool   `json:"fin_observed"`
			Boundary           string `json:"boundary"`
			Fragments          []struct {
				Frame          int    `json:"frame"`
				Offset         int    `json:"offset"`
				Length         int    `json:"length"`
				FIN            bool   `json:"fin"`
				SHA256         string `json:"sha256"`
				Hex            string `json:"hex"`
				Retransmission bool   `json:"retransmission"`
			} `json:"fragments"`
		} `json:"streams"`
	}
	data, err := os.ReadFile("testdata/protocol-corpus/derived/ndpi-doq.streams.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &evidence))
	require.Equal(t, "captures/ndpi/ndpi-doq.pcapng", evidence.SourceCapture)
	require.Equal(t, "62f2bfb0ef5c35401b9c93dab4069b37043280bcc7f61452d86e6f9c61c1129d", evidence.SourceCaptureSHA256)
	require.Equal(t, false, evidence.Extraction["binparser_decryption_performed"])
	require.Equal(t, true, evidence.Extraction["icmp_quotations_excluded"])
	require.Contains(t, evidence.Extraction["command"], "tls.keylog_file:")
	require.Equal(t, 8, evidence.FragmentCount)
	require.Equal(t, 2, evidence.StreamCount)
	require.Len(t, evidence.Streams, 2)
	frames := [][]int{{4}, {8, 9, 11, 13, 15, 17, 19}}
	for i, stream := range evidence.Streams {
		require.Equal(t, uint64(0), stream.ID)
		require.Equal(t, "::1", stream.SourceAddress)
		require.Equal(t, "::1", stream.DestinationAddress)
		require.Equal(t, [][2]uint16{{47826, 784}, {784, 47826}}[i], [2]uint16{stream.SourcePort, stream.DestinationPort})
		require.Equal(t, []string{doqTestDraftQueryHex, doqTestDraftResponseHex}[i], stream.Hex)
		wire := h225TestHex(t, stream.Hex)
		require.Len(t, wire, stream.Length)
		require.Equal(t, stream.SHA256, fmt.Sprintf("%x", sha256.Sum256(wire)))
		require.True(t, stream.FIN)
		require.Equal(t, "observed FIN", stream.Boundary)
		require.Len(t, stream.Fragments, len(frames[i]))
		reconstructed, present := make([]byte, stream.Length), make([]bool, stream.Length)
		entry := []string{"DoQDraft00QueryStream", "DoQDraft00ResponseStream"}[i]
		for j, fragment := range stream.Fragments {
			require.Equal(t, frames[i][j], fragment.Frame)
			require.Equal(t, 0, fragment.Offset)
			require.True(t, fragment.FIN)
			require.Equal(t, j != 0, fragment.Retransmission)
			part := h225TestHex(t, fragment.Hex)
			require.Len(t, part, fragment.Length)
			require.Equal(t, wire, part)
			require.Equal(t, fragment.SHA256, fmt.Sprintf("%x", sha256.Sum256(part)))
			require.LessOrEqual(t, fragment.Offset+len(part), len(reconstructed))
			for k, b := range part {
				pos := fragment.Offset + k
				if present[pos] {
					require.Equal(t, reconstructed[pos], b, "overlap conflict")
				}
				reconstructed[pos], present[pos] = b, true
			}
			n := doqTestParse(t, part, entry)
			protocolCorpusRequireValue(t, n, "ID", uint64(0))
			protocolCorpusRequireValue(t, n, "Name", "example.com.")
		}
		for _, have := range present {
			require.True(t, have, "gap in final stream")
		}
		require.Equal(t, wire, reconstructed)
	}
}

func TestProtocolCorpusDoQIndependentRFC9250Fields(t *testing.T) {
	// Independently serialized RFC 9250 + RFC 1035 messages. These are not a
	// rewrite of the original draft capture and have their own flags/TTL/EDNS.
	query, response := h225TestHex(t, doqTestRFCQueryHex), h225TestHex(t, doqTestRFCResponseHex)
	n := doqTestParse(t, query, "DoQQueryStream")
	protocolCorpusRequireValue(t, n, "Message Length", uint64(29))
	protocolCorpusRequireValue(t, n, "Flags", uint64(0x0100))
	protocolCorpusRequireValue(t, n, "Name", "example.com.")
	n = doqTestParse(t, response, "DoQResponseStream")
	protocolCorpusRequireValue(t, n, "Message Length", uint64(45))
	protocolCorpusRequireValue(t, n, "Flags", uint64(0x8180))
	protocolCorpusRequireValue(t, n, "TTL", uint64(60))
	protocolCorpusRequireValue(t, n, "Address", []byte{93, 184, 216, 34})
	// Framing can contain several response messages. Their semantic relationship
	// cannot be established without the originating query and transfer state.
	n = doqTestParse(t, append(bytes.Clone(response), response...), "DoQResponseStream")
	require.Equal(t, 2, n.Cfg.GetItem("additionInfo").(map[string]any)["Message Count"])
}

func TestProtocolCorpusDoQNegativeBoundariesAndTransactions(t *testing.T) {
	query, response := h225TestHex(t, doqTestRFCQueryHex), h225TestHex(t, doqTestRFCResponseHex)
	draft := h225TestHex(t, doqTestDraftQueryHex)
	for _, tc := range []struct {
		wire           []byte
		entry, carrier string
	}{
		{query, "DoQQueryStream", "DoQQueryCarrier"},
		{response, "DoQResponseStream", "DoQResponseCarrier"},
		{draft, "DoQDraft00QueryStream", "DoQDraft00QueryCarrier"},
		{h225TestHex(t, doqTestDraftResponseHex), "DoQDraft00ResponseStream", "DoQDraft00ResponseCarrier"},
	} {
		for cut := 0; cut < len(tc.wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tc.wire[:cut]), "application-layer.doq", tc.entry)
			require.Error(t, err)
		}
		_, err := parser.ParseBinary(bytes.NewReader(tc.wire), "application-layer.doq", tc.entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, "application-layer.doq", tc.entry)
		require.Error(t, err)
		bad := append(bytes.Clone(tc.wire), 0)
		n := doqTestParse(t, bad, tc.carrier)
		protocolCorpusRequireValue(t, n, "Unparsed DoQ Clear Stream", bad)
		require.Nil(t, protocolCorpusFindNode(n, "DNS Message"))
	}
	for offset := uint64(0); offset < 8; offset++ {
		for _, good := range []bool{true, false} {
			t.Run(fmt.Sprintf("%d/%t", offset, good), func(t *testing.T) {
				wire := bytes.Clone(query)
				if !good {
					wire[2] = 1
				}
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
				require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
				}
				source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Stream\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Stream\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Stream: \"import:application-layer/doq.yaml;node:DoQQueryCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
				var doc yaml.MapSlice
				require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
				root, err := base.NewNodeTree(doc)
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				n := base.GetNodeByPath(root, "@Wrapped")
				stream := protocolCorpusFindNode(n, "Stream")
				h225TestTree(t, stream, wire, offset)
				if good {
					protocolCorpusRequireValue(t, stream, "Name", "example.com.")
				} else {
					protocolCorpusRequireValue(t, stream, "Unparsed DoQ Clear Stream", wire)
					require.Nil(t, protocolCorpusFindNode(stream, "DNS Message"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, packed.Bytes(), NodeToBytes(n))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				_, err = reader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint("isolated-", worker), func(t *testing.T) {
			t.Parallel()
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, query, "application-layer.doq", "DoQQueryStream", cfg)
			protocolCorpusRequireValue(t, n, "ID", uint64(0))
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
