package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const steamDiscoveryStatusHex = "ffffffff214c5fa01600000008fcd3f1e3b7bcbfcf76100118afca9baaa080d9e4029200000008081006189cd30122096c6f63616c686f7374300238c8feffffffffffffff0140014a0e09362910060100100110dd9ff030580160d2f99bad0670007a1146303a32463a37343a41443a34333a4635a2010e3137322e31372e3232392e313834a2010e3139322e3136382e38382e323331aa010e3139352e3139312e3135382e3934b80102c00100c801ccc587ad06d00102"

// Independently frame protobuf fixture bytes; no production decoder/generator.
func steamDiscoveryTestWire(header, body []byte) []byte {
	b := []byte{255, 255, 255, 255, 0x21, 0x4c, 0x5f, 0xa0}
	b = binary.LittleEndian.AppendUint32(b, uint32(len(header)))
	b = append(b, header...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(body)))
	return append(b, body...)
}

func steamDiscoveryTestEthernet(body []byte) []byte {
	frame := make([]byte, 42+len(body))
	copy(frame, []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0, 0x45})
	binary.BigEndian.PutUint16(frame[16:], uint16(28+len(body)))
	frame[22], frame[23] = 64, 17
	copy(frame[26:34], []byte{192, 0, 2, 1, 192, 0, 2, 2})
	binary.BigEndian.PutUint16(frame[34:], 27036)
	binary.BigEndian.PutUint16(frame[36:], 27036)
	binary.BigEndian.PutUint16(frame[38:], uint16(8+len(body)))
	copy(frame[42:], body)
	return frame
}

// Every nonempty byte has exactly one leaf, ordered and without overlap.
// Parent links, immutable wire views and Result origins survive imports too.
func steamDiscoveryRequireTree(t *testing.T, root *base.Node, wire []byte, offset int) {
	t.Helper()
	signature := protocolCorpusFindNode(root, "Signature")
	require.NotNil(t, signature)
	n := signature.Cfg.GetItem(base.CfgParent).(*base.Node)
	pos := uint64(offset * 8)
	var walk func(*base.Node)
	walk = func(current *base.Node) {
		if current.Cfg.Has(base.CfgNodeResult) {
			span := stream_parser.GetNodeResultPos(current)
			require.Equal(t, pos, span[0], current.Name)
			require.GreaterOrEqual(t, span[1], span[0])
			pos = span[1]
			v, err := current.Result()
			require.NoError(t, err, current.Name)
			require.Same(t, current, v.Origin)
			return
		}
		for _, child := range current.Children {
			require.Same(t, current, child.Cfg.GetItem(base.CfgParent), child.Name)
			require.Same(t, current.Ctx, child.Ctx)
			walk(child)
		}
		if len(current.Children) > 0 && current.Children[0].Name == "Field" {
			require.True(t, current.Cfg.GetBool(stream_parser.CfgIsList), current.Name)
			v, err := current.Result()
			require.NoError(t, err)
			require.True(t, v.IsList(), current.Name)
			require.Len(t, v.Children(), len(current.Children))
			mapped, ok := NodeToMap(current).([]any)
			require.True(t, ok, "ordered field records must remain a list in NodeToMap")
			require.Len(t, mapped, len(current.Children))
		}
	}
	walk(n)
	require.Equal(t, uint64((offset+len(wire))*8), pos)
	require.Equal(t, wire, NodeToBytes(n)[offset:offset+len(wire)])
	_, err := n.Result()
	require.NoError(t, err)
}

func TestProtocolCorpusSteamDiscoveryOriginalEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-steam.pcapng"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "d4064515b97c5b3ff5a2e8b0493b878a377a9ef6ff9a376e886eb268ceba3fde", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 48)
	counts := map[string]int{}
	for i, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", i+1), func(t *testing.T) {
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			root := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			require.Equal(t, frame, NodeToBytes(root))
			udpLayer := packet.Layer(layers.LayerTypeUDP)
			if udpLayer == nil {
				counts["TCP carrier"]++
				require.NotNil(t, packet.Layer(layers.LayerTypeTCP))
				require.Nil(t, protocolCorpusFindNode(root, "SteamDiscovery"))
				return
			}
			udp := udpLayer.(*layers.UDP)
			require.Equal(t, int(udp.Length)-8, len(udp.Payload))
			if udp.DstPort != 27036 {
				counts["Other UDP"]++
				require.Equal(t, layers.UDPPort(27045), udp.DstPort)
				require.True(t, bytes.HasPrefix(udp.Payload, []byte{1, 0x81, 's', 'd', 'p', 'i', 'n', 'g'}))
				require.Nil(t, protocolCorpusFindNode(root, "SteamDiscovery"))
				return
			}
			counts["Local discovery"]++
			require.Equal(t, frame[42:], udp.Payload)
			require.LessOrEqual(t, i+1, 6)
			wire := udp.Payload
			for _, parse := range []struct {
				node   *base.Node
				offset int
			}{{root, 42}, {protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.steam_discovery", "SteamDiscovery"), 0}} {
				steamDiscoveryRequireTree(t, parse.node, wire, parse.offset)
				message := protocolCorpusFindNode(parse.node, "Signature").Cfg.GetItem(base.CfgParent).(*base.Node)
				protocolCorpusRequireValue(t, message, "Header Length", uint64(22))
				bodyLen := int(binary.LittleEndian.Uint32(wire[34:]))
				protocolCorpusRequireValue(t, message, "Body Length", uint64(bodyLen))
				steamDiscoveryRequireObservedFields(t, message, wire, i+1)
			}
		})
	}
	require.Equal(t, map[string]int{"Local discovery": 6, "Other UDP": 2, "TCP carrier": 40}, counts)
}

func steamDiscoveryRequireObservedFields(t *testing.T, root *base.Node, wire []byte, frame int) {
	t.Helper()
	value := func(name string) any {
		n := protocolCorpusFindNode(root, name)
		require.NotNil(t, n, name)
		v, err := n.Result()
		require.NoError(t, err)
		return v.Value
	}
	client, _ := binary.Uvarint([]byte{0xfc, 0xd3, 0xf1, 0xe3, 0xb7, 0xbc, 0xbf, 0xcf, 0x76})
	instance, _ := binary.Uvarint([]byte{0xaf, 0xca, 0x9b, 0xaa, 0xa0, 0x80, 0xd9, 0xe4, 2})
	require.Equal(t, client, value("client_id"))
	require.Equal(t, instance, value("instance_id"))
	kind := int32(0)
	if frame == 1 || frame == 3 || frame == 6 {
		kind = 1
	}
	require.Equal(t, kind, value("msg_type"))
	if kind == 0 {
		require.Equal(t, uint32(1), value("seq_num"))
		return
	}
	for name, want := range map[string]any{
		"version": int32(8), "min_version": int32(6), "connect_port": uint32(27036),
		"hostname": "localhost", "enabled_services": uint32(2), "ostype": int32(-184),
		"is64bit": true, "euniverse": int32(1), "games_running": false,
		"mac_addresses": "F0:2F:74:AD:43:F5", "public_ip_address": "195.191.158.94",
		"supported_services": uint32(2), "steam_deck": false, "vr_link_caps": int32(2),
	} {
		require.Equal(t, want, value(name), name)
	}
	// Field values below are independently extracted from literal schema wire
	// values. In particular the signed 10-byte int32 must not become uint64.
	steamID, _ := hex.DecodeString("3629100601001001")
	require.Equal(t, binary.LittleEndian.Uint64(steamID), value("steamid"))
	keyID, _ := hex.DecodeString("dd9ff030")
	key, _ := binary.Uvarint(keyID)
	require.Equal(t, uint32(key), value("auth_key_id"))
	versionBytes, _ := hex.DecodeString("ccc587ad06")
	version, _ := binary.Uvarint(versionBytes)
	require.Equal(t, version, value("steam_version"))
	timestamps := map[int][]byte{1: {0xd2, 0xf9, 0x9b, 0xad, 6}, 3: {0xd3, 0xf9, 0x9b, 0xad, 6}, 6: {0xe8, 0xf9, 0x9b, 0xad, 6}}
	timestamp, _ := binary.Uvarint(timestamps[frame])
	require.Equal(t, uint32(timestamp), value("timestamp"))
	var addresses []any
	var walk func(*base.Node)
	walk = func(n *base.Node) {
		if n.Name == "ip_addresses" {
			v, err := n.Result()
			require.NoError(t, err)
			addresses = append(addresses, v.Value)
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)
	require.Equal(t, []any{"172.17.229.184", "192.168.88.231"}, addresses)
}

func TestProtocolCorpusSteamDiscoveryBoundariesAndTransactions(t *testing.T) {
	good, err := hex.DecodeString(steamDiscoveryStatusHex)
	require.NoError(t, err)
	for size := 0; size < len(good); size++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(good[:size]), "application-layer.steam_discovery", "SteamDiscovery")
		require.Error(t, err, "prefix %d", size)
	}
	badLength := bytes.Clone(good)
	binary.LittleEndian.PutUint32(badLength[34:], 0xffffffff)
	badField := bytes.Clone(good)
	badField[len(badField)-1] = 0x80
	for _, body := range [][]byte{good, good[:1], good[:len(good)-1], badLength, badField, append(bytes.Clone(good), 0xaa)} {
		valid := bytes.Equal(body, good)
		for _, carrier := range []bool{false, true} {
			wire, rule, entry := body, "application-layer/steam_discovery.yaml", "SteamDiscoveryCarrier"
			offset := 0
			if carrier {
				wire, rule, entry, offset = steamDiscoveryTestEthernet(body), "ethernet.yaml", "Ethernet", 42
			}
			root, err := base.ParseRule(rule)
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire)*8))
			tail := []byte{0xd1, 0xa5, 0x37}
			r := bytes.NewReader(append(bytes.Clone(wire), tail...))
			reader := base.NewBitReader(r)
			require.NoError(t, root.ParseSubNode(reader, entry))
			remaining, err := reader.ReadBits(uint64(len(tail) * 8))
			require.NoError(t, err)
			require.Equal(t, tail, remaining)
			require.ErrorContains(t, reader.Recovery(), "no backup")
			require.ErrorContains(t, reader.PopBackup(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
			require.Equal(t, wire, NodeToBytes(root))
			if valid {
				steamDiscoveryRequireTree(t, root, body, offset)
			} else {
				require.Nil(t, protocolCorpusFindNode(root, "Signature"), "failed tree must not leak")
				field := protocolCorpusFindNode(root, "Steam Discovery Payload")
				require.NotNil(t, field)
				require.Equal(t, [2]uint64{uint64(offset * 8), uint64(len(wire) * 8)}, stream_parser.GetNodeResultPos(field))
			}
		}
	}
	_, err = parser.ParseBinary(bytes.NewReader(good), "application-layer.steam_discovery", "SteamDiscovery")
	require.ErrorContains(t, err, "explicit byte boundary required")
	// The caller can declare a complete datagram but supply a short reader.
	// The native bridge must roll back both the reader and output buffer even
	// when staging fails before the protobuf validator is reached.
	for _, size := range []int{0, 1, 17, len(good) - 1} {
		root, err := base.ParseRule("application-layer/steam_discovery.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(good)*8))
		reader := base.NewBitReader(bytes.NewReader(good[:size]))
		err = root.ParseSubNode(reader, "SteamDiscovery")
		require.Error(t, err)
		require.Empty(t, NodeToBytes(root))
		require.Nil(t, protocolCorpusFindNode(root, "Signature"))
		remaining, err := reader.ReadBits(uint64(size * 8))
		require.NoError(t, err)
		require.Equal(t, good[:size], remaining)
		require.ErrorContains(t, reader.Recovery(), "no backup")
		require.ErrorContains(t, reader.PopBackup(), "no backup")
	}
}

func steamDiscoveryValues(t *testing.T, node *base.Node) []any {
	t.Helper()
	var result []any
	var walk func(*base.Node)
	walk = func(n *base.Node) {
		if n.Cfg.Has(base.CfgNodeResult) {
			v, err := n.Result()
			require.NoError(t, err)
			result = append(result, n.Name, stream_parser.GetNodeResultPos(n), reflect.TypeOf(v.Value), v.Value)
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(node)
	return result
}

func TestProtocolCorpusSteamDiscoveryStructuredBranchesAndLimits(t *testing.T) {
	// Independent protobuf wire literals cover branches absent from the real
	// discovery/status capture. Never derive these fixtures from parser schemas.
	for _, c := range []struct {
		name         string
		header, body []byte
	}{
		{"empty-default", nil, nil},
		{"offline-empty", []byte{0x10, 2}, nil},
		{"packed-client-ids", []byte{0x10, 0}, []byte{0x12, 3, 1, 0xac, 2}},
		{"empty-packed", []byte{0x10, 0}, []byte{0x12, 0}},
		{"nested-user", []byte{0x10, 1}, []byte{0x4a, 11, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0x10, 2}},
		{"empty-user", []byte{0x10, 1}, []byte{0x4a, 0}},
		{"gamepad-and-transport", []byte{0x10, 5}, []byte{8, 1, 0x72, 2, 1, 4, 0x92, 1, 4, 8, 2, 0x10, 3}},
		{"progress-float", []byte{0x10, 13}, []byte{8, 7, 0x15, 0, 0, 0xc0, 0x3f}},
		{"unknown-groups", []byte{0x10, 2}, []byte{0xa3, 6, 8, 7, 0xab, 6, 0xac, 6, 0xa4, 6}},
		{"unknown-future", []byte{0x10, 99}, []byte{0x0d, 0x12, 0x34, 0x56, 0x78, 0x12, 0}},
		{"duplicate-values", []byte{0x10, 0}, []byte{8, 1, 8, 2}},
	} {
		t.Run(c.name, func(t *testing.T) {
			wire := steamDiscoveryTestWire(c.header, c.body)
			n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.steam_discovery", "SteamDiscovery")
			steamDiscoveryRequireTree(t, n, wire, 0)
			imported := protocolCorpusRequireBoundedRuleParse(t, steamDiscoveryTestEthernet(wire), "ethernet", "Ethernet")
			steamDiscoveryRequireTree(t, imported, wire, 42)
			legacy, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "application-layer.steam_discovery", map[string]any{"outScalarLegacy": true}, "SteamDiscovery")
			require.NoError(t, err)
			require.Equal(t, steamDiscoveryValues(t, legacy), steamDiscoveryValues(t, n))
		})
	}
	for _, count := range []int{1, 64, 1024, 4095, 4096} {
		body := bytes.Repeat([]byte{8, 1}, count)
		wire := steamDiscoveryTestWire([]byte{0x10, 0}, body)
		n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.steam_discovery", "SteamDiscovery")
		if count == 4096 {
			require.ErrorContains(t, err, "field/element count")
			continue
		}
		require.NoError(t, err)
		steamDiscoveryRequireTree(t, n, wire, 0)
	}
	for _, size := range []int{65535, 65536} {
		// Unknown field 100 with a three-byte length, inside an offline body.
		payloadSize := size - 23
		body := binary.AppendUvarint([]byte{0xa2, 6}, uint64(payloadSize))
		body = append(body, bytes.Repeat([]byte{0x97}, payloadSize)...)
		wire := steamDiscoveryTestWire([]byte{0x10, 2}, body)
		require.Len(t, wire, size)
		n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.steam_discovery", "SteamDiscovery")
		if size == 65536 {
			require.ErrorContains(t, err, "16..65535")
			continue
		}
		require.NoError(t, err)
		steamDiscoveryRequireTree(t, n, wire, 0)
	}
	_, err := parser.GenerateBinary(map[string]any{}, "application-layer.steam_discovery", "SteamDiscovery")
	require.ErrorContains(t, err, "structured generation is not supported")
}

func TestProtocolCorpusSteamDiscoveryScalarOutCompatibilityAndParallel(t *testing.T) {
	good, err := hex.DecodeString(steamDiscoveryStatusHex)
	require.NoError(t, err)
	want := steamDiscoveryValues(t, protocolCorpusRequireBoundedRuleParse(t, good, "application-layer.steam_discovery", "SteamDiscovery"))
	for _, config := range []map[string]any{{"outScalarLegacy": true}, {"outProgramLegacy": true}, {}} {
		n, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(good), "application-layer.steam_discovery", config, "SteamDiscovery")
		require.NoError(t, err)
		require.Equal(t, want, steamDiscoveryValues(t, n))
	}
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			n := protocolCorpusRequireBoundedRuleParse(t, good, "application-layer.steam_discovery", "SteamDiscovery")
			require.Equal(t, want, steamDiscoveryValues(t, n))
		})
	}
}

func BenchmarkSteamDiscoveryParseAndResult(b *testing.B) {
	wire, _ := hex.DecodeString(steamDiscoveryStatusHex)
	for _, legacy := range []bool{true, false} {
		b.Run(fmt.Sprintf("scalar-legacy-%t", legacy), func(b *testing.B) {
			config := map[string]any{"outScalarLegacy": legacy}
			warm, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "application-layer.steam_discovery", config, "SteamDiscovery")
			if err != nil {
				b.Fatal(err)
			}
			if _, err = warm.Result(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(wire)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "application-layer.steam_discovery", config, "SteamDiscovery")
				if err != nil {
					b.Fatal(err)
				}
				if _, err = n.Result(); err != nil {
					b.Fatal(err)
				}
				if !bytes.Equal(wire, NodeToBytes(n)) {
					b.Fatal("wire changed")
				}
			}
		})
	}
}
