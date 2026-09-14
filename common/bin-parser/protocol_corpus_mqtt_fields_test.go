package bin_parser

import (
	"bytes"
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
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const mqttFieldsTestRule = "application-layer.mqtt_fields"

// Independent fixture traversal, retaining every prefix and value, including
// original binary fields. No original User Name/Password is copied into code.
func mqttFieldsTestValues(t *testing.T, n *base.Node, w []byte) int {
	t.Helper()
	protocolCorpusRequireValue(t, n, "Fixed Header", uint64(w[0]))
	at, remain, shift := 1, 0, 0
	for {
		b := w[at]
		protocolCorpusRequireValue(t, n, fmt.Sprintf("Remaining Length Byte %d", at-1), uint64(b))
		remain += int(b&127) << shift
		at++
		shift += 7
		if b&128 == 0 {
			break
		}
	}
	require.Equal(t, len(w)-at, remain)
	require.Equal(t, remain, n.Cfg.GetItem("additionInfo").(map[string]any)["Remaining Length"])
	number := func(name string, size int) uint64 {
		var value uint64
		for _, b := range w[at : at+size] {
			value = value<<8 | uint64(b)
		}
		at += size
		protocolCorpusRequireValue(t, n, name, value)
		return value
	}
	vector := func(name string, text bool) []byte {
		size := int(binary.BigEndian.Uint16(w[at:]))
		number(name+" Length", 2)
		v := w[at : at+size]
		at += size
		if text {
			protocolCorpusRequireValue(t, n, name, string(v))
		} else {
			protocolCorpusRequireValue(t, n, name, v)
		}
		return v
	}
	payload := 0
	switch w[0] >> 4 {
	case 1:
		vector("Protocol Name", true)
		number("Protocol Level", 1)
		flags := number("Connect Flags", 1)
		number("Keep Alive", 2)
		vector("Client Identifier", true)
		require.Zero(t, flags&4, "original CONNECT has no Will")
		if flags&128 != 0 {
			vector("User Name", false)
		}
		if flags&64 != 0 {
			vector("Password", false)
		}
	case 2:
		number("Acknowledgment Flags", 1)
		number("Return Code", 1)
	case 3:
		vector("Topic Name", true)
		require.Zero(t, w[0]&6, "original publications have QoS 0")
		protocolCorpusRequireValue(t, n, "Application Message", w[at:])
		require.True(t, json.Valid(w[at:]), "fixture body oracle only; production body stays raw")
		payload, at = len(w)-at, len(w)
	case 8:
		number("Packet Identifier", 2)
		vector("Topic Filter", true)
		number("Requested QoS", 1)
	case 9:
		number("Packet Identifier", 2)
		number("Granted QoS or Return Code", 1)
	case 14:
	default:
		t.Fatalf("unaccounted original type %d", w[0]>>4)
	}
	require.Equal(t, len(w), at)
	return payload
}

func mqttFieldsTestWhole(t *testing.T, frame []byte, offset, size int, entry string) *base.Node {
	t.Helper()
	tail := len(frame) - offset - size
	root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Capture:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Envelope\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Envelope: raw,%d\n    Message: \"import:application-layer/mqtt_fields.yaml;node:%s\"\n    Tail: raw,%d\n", offset, size, tail, offset, entry, tail))
	root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
	r := base.NewBitReader(bytes.NewReader(frame))
	require.NoError(t, r.Backup())
	require.NoError(t, root.ParseSubNode(r, "Capture"))
	require.Equal(t, frame, NodeToBytes(base.GetNodeByPath(root, "@Capture")))
	n := protocolCorpusFindNode(root, "Message")
	tlsCertificateTestTree(t, n, frame[offset:offset+size], uint64(offset)*8)
	latTestTree(t, n, uint64(offset)*8, uint64(offset+size)*8)
	require.NoError(t, r.Recovery())
	got, err := r.ReadBits(uint64(len(frame)) * 8)
	require.NoError(t, err)
	require.Equal(t, frame, got)
	require.ErrorContains(t, r.PopBackup(), "no backup")
	return n
}

func TestProtocolCorpusMQTTFieldsAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-mqtt.pcap"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "7d6552eb815eead6eabe0adbaa13366b4863b4c1c54dbb6fa65dec68a9738465", tlsCertificateTestSHA(raw))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 9)
	nextSequence := map[string]uint32{}
	counts := map[uint64]int{}
	physicalBytes, bodyBytes, controls := 0, 0, 0
	for i, record := range records {
		packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if len(tcp.Payload) == 0 {
			controls++
			require.True(t, tcp.SYN && tcp.ACK)
			continue
		}
		key := fmt.Sprint(packet.NetworkLayer().NetworkFlow(), tcp.TransportFlow())
		if seq, exists := nextSequence[key]; exists {
			require.Equal(t, seq, tcp.Seq)
		}
		nextSequence[key] = tcp.Seq + uint32(len(tcp.Payload))
		offset := 0
		for _, layer := range packet.Layers() {
			offset += len(layer.LayerContents())
			if layer.LayerType() == layers.LayerTypeTCP {
				break
			}
		}
		require.Equal(t, tcp.Payload, record[offset:])
		entry, level := "MQTT31PacketFields", 3
		if i == 8 {
			entry, level = "MQTT311PacketFields", 4
		}
		w := tcp.Payload
		t.Run(fmt.Sprintf("frame-%d", i+1), func(t *testing.T) {
			n := protocolCorpusRequireBoundedRuleParse(t, w, mqttFieldsTestRule, entry)
			tlsCertificateTestTree(t, n, w, 0)
			latTestTree(t, n, 0, uint64(len(w))*8)
			bodyBytes += mqttFieldsTestValues(t, n, w)
			full := mqttFieldsTestWhole(t, record, offset, len(w), entry)
			require.Equal(t, n.Cfg.GetItem("additionInfo"), full.Cfg.GetItem("additionInfo"))
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, level, info["Protocol Level Context"])
			if i == 1 || i == 8 {
				flagAt, clean := 11, true
				if i == 8 {
					flagAt, clean = 10, false
				}
				require.Equal(t, []map[string]any{{"Field": "Connect Flags", "Relative Byte Range": [2]int{flagAt, flagAt + 1}, "User Name Flag": true, "Password Flag": true, "Will Retain": false, "Will QoS": uint64(0), "Will Flag": false, "Clean Session": clean, "Reserved": uint64(0)}}, info["Decoded Flag Fields"])
			}
			counts[info["Packet Type"].(uint64)]++
			for _, field := range []string{"TCP Reassembly Performed", "Session State Validated", "Delivery Outcome Validated", "Application Body Decoded", "Structured Generation Supported"} {
				require.Equal(t, false, info[field])
			}
			if i == 2 {
				protocolCorpusRequireValue(t, n, "Topic Filter", "astr/s720/02D5050223D3/99")
				protocolCorpusRequireValue(t, n, "Requested QoS", uint64(0))
				require.Equal(t, 1, info["List Item Count"])
			} else if i == 4 {
				protocolCorpusRequireValue(t, n, "Granted QoS or Return Code", uint64(0))
				require.Equal(t, 1, info["List Item Count"])
			} else if i == 5 || i == 6 {
				want := "83f10462c8e694b6760006b8210e1ca488024180cbe47145f9b786800bab2f76"
				if i == 6 {
					want = "ebdbae8c6742480fd065ba55f3cdac722ce15ee197bbd06c34ea95b221a6aab9"
				}
				require.Equal(t, want, tlsCertificateTestSHA(stream_parser.GetBytesByNode(protocolCorpusFindNode(n, "Application Message"))))
			}
			for cut := 0; cut < len(w); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w[:cut]), mqttFieldsTestRule, entry)
				require.Error(t, err)
			}
		})
		physicalBytes += len(w)
	}
	require.Equal(t, map[uint64]int{1: 2, 2: 1, 3: 2, 8: 1, 9: 1, 14: 1}, counts)
	require.Equal(t, 1, controls)
	require.Len(t, nextSequence, 3)
	require.Equal(t, 875, physicalBytes)
	require.Equal(t, 429, bodyBytes)
}

func TestProtocolCorpusMQTTFieldsEmptyValues(t *testing.T) {
	for _, spec := range []struct {
		wire  []byte
		empty []string
	}{
		{latTestBytes(t, "3003000161"), []string{"Application Message"}},
		{latTestBytes(t, "101500044d51545404c600000000000161000000000000"), []string{"Client Identifier", "Will Message", "User Name", "Password"}},
	} {
		n := protocolCorpusRequireBoundedRuleParse(t, spec.wire, mqttFieldsTestRule, "MQTT311PacketFields")
		tlsCertificateTestTree(t, n, spec.wire, 0)
		latTestTree(t, n, 0, uint64(len(spec.wire))*8)
		for _, name := range spec.empty {
			field := protocolCorpusFindNode(n, name)
			require.NotNil(t, field)
			require.True(t, stream_parser.NodeHasResult(field), "zero-width values remain real results")
			span := stream_parser.GetNodeResultPos(field)
			require.Equal(t, span[0], span[1])
			require.Empty(t, stream_parser.GetBytesByNode(field))
		}
	}
}

func TestProtocolCorpusMQTTFieldsBoundariesAndIsolation(t *testing.T) {
	wire := latTestBytes(t, "32070001610007ff00")
	for _, entry := range []string{"MQTT31PacketFields", "MQTT311PacketFields"} {
		for _, name := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(wire), mqttFieldsTestRule, name)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, mqttFieldsTestRule, name)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, ((1 << 20) + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, mqttFieldsTestRule, name)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := bytes.Clone(wire)
				if !good {
					w[0] = 0xff
				}
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(w, uint64(len(w))*8))
				require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
				}
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/mqtt_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				m := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, m, w, offset)
				if good {
					protocolCorpusRequireValue(t, m, "Packet Identifier", uint64(7))
					protocolCorpusRequireValue(t, m, "Application Message", []byte{0xff, 0})
				} else {
					protocolCorpusRequireValue(t, m, "Unparsed MQTT Wire", w)
					require.Nil(t, protocolCorpusFindNode(m, "Packet Identifier"))
				}
				protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
				require.Equal(t, 123, root.Ctx.GetItem("marker"))
				require.Equal(t, packed.Bytes(), NodeToBytes(base.GetNodeByPath(root, "@Wrapped")))
				require.NoError(t, r.Recovery())
				got, err := r.ReadBits(uint64(packed.Len()) * 8)
				require.NoError(t, err)
				require.Equal(t, packed.Bytes(), got)
				require.ErrorContains(t, r.PopBackup(), "no backup")
				_, err = r.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			}
		}
		both := append(bytes.Clone(wire), wire...)
		mqttFieldsTestWhole(t, both, 0, len(wire), entry)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(both), mqttFieldsTestRule, entry)
		require.Error(t, err)
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, mqttFieldsTestRule, "MQTT31PacketFields", cfg)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			info["Publish Flags"].(map[string]any)["QoS"] = worker
			next := protocolCorpusRequireBoundedRuleParse(t, wire, mqttFieldsTestRule, "MQTT311PacketFields")
			require.Equal(t, uint64(1), next.Cfg.GetItem("additionInfo").(map[string]any)["Publish Flags"].(map[string]any)["QoS"])
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
