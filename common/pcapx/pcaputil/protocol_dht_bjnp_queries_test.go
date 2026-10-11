package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// The literal complete projections below are checked against the historical
// datagrams and actual tshark 4.2.5 PDML (BEP 5 for KRPC). Neither this capture
// nor the stateless native projections prove endpoint success or correlation.
// In particular the sixth DHT response has only id/token, without peers/nodes.
func datagramOwnedInputs(t *testing.T, protocol string) ([]byte, [][]byte, []map[string]any) {
	t.Helper()
	var name, digest string
	var wireHex []string
	var want []map[string]any
	if protocol == "bittorrent-dht" {
		name, digest = "16-bittorrent-dht.pcapng", "73d6d75249381dfc0f527e3920fbbc1544a7099c0a1e1aaeeed77da83cd984c1"
		wireHex = []string{"64313a6164323a696432303a77696e6c61622d6e6f64652d353031332121212165313a71343a70696e67313a74323a6161313a79313a7165", "64313a7264323a696432303a6c61622d706565722d6e6f64652d35303133212165313a74323a6161313a79313a7265", "64313a6164323a696432303a77696e6c61622d6e6f64652d3530313321212121363a74617267657432303a7461726765742d6e6f64652d353031332121212165313a71393a66696e645f6e6f6465313a74323a6162313a79313a7165", "64313a7264323a696432303a6c61622d706565722d6e6f64652d353031332121353a6e6f64657332363a6c61622d706565722d6e6f64652d353031332121c63364091ae165313a74323a6162313a79313a7265", "64313a6164323a696432303a77696e6c61622d6e6f64652d3530313321212121393a696e666f5f6861736832303a6c61622d696e666f686173682d3530313321212165313a71393a6765745f7065657273313a74323a6163313a79313a7165", "64313a7264323a696432303a6c61622d706565722d6e6f64652d353031332121353a746f6b656e333a6c616265313a74323a6163313a79313a7265", "64313a6164323a696432303a77696e6c61622d6e6f64652d353031332121212131323a696d706c6965645f706f7274693065393a696e666f5f6861736832303a6c61622d696e666f686173682d35303133212121343a706f7274693638383165353a746f6b656e333a6c616265313a7131333a616e6e6f756e63655f70656572313a74323a6164313a79313a7165", "64313a7264323a696432303a6c61622d706565722d6e6f64652d35303133212165313a74323a6164313a79313a7265"}
		node, peer := []byte("winlab-node-5013!!!!"), []byte("lab-peer-node-5013!!")
		want = []map[string]any{
			{"Transaction ID": []byte("aa"), "Packet Name": "ping", "Node ID": node},
			{"Transaction ID": []byte("aa"), "Packet Name": "response", "Node ID": peer},
			{"Transaction ID": []byte("ab"), "Packet Name": "find_node", "Node ID": node, "Target": []byte("target-node-5013!!!!")},
			{"Transaction ID": []byte("ab"), "Packet Name": "response", "Node ID": peer, "Nodes Length": 26, "First Node ID": peer, "First Node IP": "198.51.100.9", "First Node Port": uint16(6881), "Nodes": []map[string]any{{"Node ID": peer, "IP": "198.51.100.9", "Port": uint16(6881), "IP Version": byte(4)}}},
			{"Transaction ID": []byte("ac"), "Packet Name": "get_peers", "Node ID": node, "Info Hash": []byte("lab-infohash-5013!!!")},
			{"Transaction ID": []byte("ac"), "Packet Name": "response", "Node ID": peer, "Token": []byte("lab")},
			{"Transaction ID": []byte("ad"), "Packet Name": "announce_peer", "Node ID": node, "Info Hash": []byte("lab-infohash-5013!!!"), "Token": []byte("lab"), "Port": int64(6881), "Port Present": true, "Implied Port": int64(0), "Observed Peer Port": uint16(6881), "Peer Port Source": "announce-port", "Peer IP": "192.0.2.10"},
			{"Transaction ID": []byte("ad"), "Packet Name": "response", "Node ID": peer},
		}
	} else {
		name, digest = "06-bjnp.pcapng", "ad41277dc98207000c1f8d2cf688ec854312649faffb33d870578622f170cff7"
		wireHex = []string{"424a4e50010100000001000000000000", "424a4e5081010000000100010000000b4c41422d5052494e544552", "424a4e50013000000002000100000000", "424a4e50813000000002000100000007534e3a35303133", "424a4e500110000000030001000000056a6f623d37", "424a4e508110000000030001000000086163636570746564", "424a4e50012000000004000100000000", "424a4e5081200000000400010000000469646c65"}
		payloads := []string{"", "LAB-PRINTER", "", "SN:5013", "job=7", "accepted", "", "idle"}
		names := []string{"Discover", "Get Printer Identity", "Print Job Details", "Get Printer Status"}
		commands := []byte{1, 0x30, 0x10, 0x20}
		for i, payload := range payloads {
			role, session := "request", uint16(1)
			if i%2 == 1 {
				role = "response"
			}
			if i == 0 {
				session = 0
			}
			want = append(want, map[string]any{"Packet Name": names[i/2], "Role": role, "Command": commands[i/2], "Sequence": uint32(i/2 + 1), "Session": session, "Payload": []byte(payload)})
		}
	}
	capture, err := trafficfixture.ReadFile("../../bin-parser/testdata/winlab5013/captures/" + name)
	require.NoError(t, err)
	require.Equal(t, digest, fmt.Sprintf("%x", sha256.Sum256(capture)))
	var wires [][]byte
	for _, literal := range wireHex {
		wire, err := hex.DecodeString(literal)
		require.NoError(t, err)
		wires = append(wires, wire)
	}
	return capture, wires, want
}

func assertOwnedDatagram(t *testing.T, e *ProtocolEvent, protocol string, wire []byte, want map[string]any) {
	t.Helper()
	require.Equal(t, protocol, e.Protocol)
	require.Equal(t, protocol+"-native", e.Profile)
	require.Equal(t, "udp", e.Transport)
	require.Equal(t, "message", e.Completeness)
	require.Contains(t, []string{"decoded", "deferred"}, e.Status)
	require.Empty(t, e.Error)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, len(wire), e.Length)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID) // Binary KRPC field t is not a session association.
	require.Equal(t, want, e.Session)
	fields, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, fields)
	mutateRTPSQuery(fields)
	tree, err := e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, protocolFields(tree))
	require.Equal(t, want, tree["session"])
	mutateRTPSQuery(tree)
	mutateRTPSQuery(e.Fields)
	mutateRTPSQuery(e.Structured)
	mutateRTPSQuery(e.Session)
	fields, err = e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, fields)
	e.Session = nil
	tree, err = e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, protocolFields(tree))
	require.Equal(t, want, tree["session"])
	require.Equal(t, wire, e.Raw)
}

func testOwnedDatagramMatrix(t *testing.T, protocol string) {
	capture, wires, want := datagramOwnedInputs(t, protocol)
	client, server := "192.0.2.10:46881", "192.0.2.20:6881"
	if protocol == "bjnp" {
		client, server = "192.0.2.10:48611", "192.0.2.20:8611"
	}
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("owned/workers%d/deferred%t/observe%t", workers, deferred, observe), func(t *testing.T) {
					var packets atomic.Int64
					opts := []CaptureOption{WithProtocolDeferred(deferred)}
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { packets.Add(1) }))
					}
					events, stats, err := binReplay(t, capture, workers, opts...)
					require.NoError(t, err)
					require.Len(t, events, len(wires))
					require.Zero(t, stats.Malformed)
					require.Zero(t, stats.BufferedBytes)
					ids := map[uint64]bool{}
					for i, event := range events {
						status := "decoded"
						if deferred {
							status = "deferred"
						}
						require.Equal(t, status, event.Status)
						require.NotZero(t, event.ID)
						require.False(t, ids[event.ID])
						ids[event.ID] = true
						endpoints := []string{client, server}
						require.Equal(t, endpoints[i%2], event.Source)
						require.Equal(t, endpoints[1-i%2], event.Destination)
						require.Zero(t, event.Direction)
						require.True(t, time.Unix(1758585600, int64(i+1)*1000000).Equal(event.Timestamp))
						require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wires[i])), event.SourceBytes.SHA256)
						require.Len(t, event.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, i+1, event.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, event.Domain, event.SourceBytes.PacketRefs[0].Domain)
						assertOwnedDatagram(t, event, protocol, wires[i], want[i])
					}
					if observe {
						require.EqualValues(t, len(wires), packets.Load())
					}
				})
			}
		}
	}
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("owned/deferred%t/boundaries", deferred), func(t *testing.T) {
			port := uint16(6881)
			if protocol == "bjnp" {
				port = 8611
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(46000, port))
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = deferred
			bad := [][]byte{wires[1][:12], wires[1][12:], append(bytes.Clone(wires[0]), wires[1]...), append(bytes.Clone(wires[0]), 'x')}
			for i, wire := range bad {
				out := s.Feed(i%2, time.Unix(int64(i+1), 0), bytes.Clone(wire))
				rocTypedError(t, "UnsupportedFeature", out.Err)
				require.Equal(t, len(wire), out.Consumed)
				require.False(t, out.NeedMore)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Empty(t, e.Protocol)
				require.Equal(t, "unrecognized", e.Status)
				// Unrecognized traffic deliberately retains only the bounded
				// 64-byte probe, while Consumed accounts for the whole datagram.
				require.Equal(t, wire[:min(len(wire), 64)], e.Raw)
				require.Nil(t, e.Session)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, s.Stats().BufferedBytes)
			}
			owned := bytes.Clone(wires[1])
			good := s.Feed(1, time.Unix(10, 0), owned)
			require.Nil(t, good.Err)
			require.Len(t, good.Events, 1)
			clear(owned)
			assertOwnedDatagram(t, good.Events[0], protocol, wires[1], want[1])
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Zero(t, s.Stats().Flows)
		})
	}
}

func testDHTBudgetsAndIntegers(t *testing.T) {
	ping := []byte("d1:ad2:id20:winlab-node-5013!!!!e1:q4:ping1:t2:aa1:y1:qe")
	want := map[string]any{"Transaction ID": []byte("aa"), "Packet Name": "ping", "Node ID": []byte("winlab-node-5013!!!!")}
	for _, literal := range []string{"9223372036854775807", "-9223372036854775808", "340282366920938463463374607431768211455"} {
		t.Run("canonical-integer/"+literal, func(t *testing.T) {
			wire := append(bytes.Clone(ping[:len(ping)-1]), []byte("1:zi"+literal+"ee")...)
			for _, deferred := range []bool{false, true} {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(46000, 6881))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				out := s.Feed(0, time.Unix(1, 0), wire)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				assertOwnedDatagram(t, out.Events[0], "bittorrent-dht", wire, want)
				require.Empty(t, s.Close("done"))
				require.Zero(t, s.Stats().BufferedBytes)
			}
		})
	}
	for _, literal := range []string{"-0", "00", "01", "-01", "+1", "1x", ""} {
		wire := append(bytes.Clone(ping[:len(ping)-1]), []byte("1:zi"+literal+"ee")...)
		require.Falsef(t, validDHTMessage(wire), "noncanonical %q", literal)
	}

	// Empty byte-string dictionary keys and arbitrary-size canonical integers
	// are legal BEP 3 extensions. They must not relax known KRPC field types.
	t.Run("canonical-empty-key", func(t *testing.T) {
		fields, err := decodeDHTMessage(append([]byte("d0:0:"), ping[1:]...), 32)
		require.NoError(t, err)
		require.Equal(t, want, fields)
	})
	t.Run("known-field-types", func(t *testing.T) {
		for _, tail := range []string{"5:nodesi1e", "5:tokeni1e", "6:valuesi1e", "5:nodes1:x", "5:token0:"} {
			wire := []byte("d1:rd2:id20:lab-peer-node-5013!!" + tail + "e1:t2:aa1:y1:re")
			require.Falsef(t, validDHTMessage(wire), "invalid response %q", tail)
		}
		_, wires, _ := datagramOwnedInputs(t, "bittorrent-dht")
		for _, literal := range []string{"1:x", "i9223372036854775808e", "i2e", "i-1e"} {
			require.Falsef(t, validDHTMessage(bytes.Replace(wires[6], []byte("12:implied_porti0e"), []byte("12:implied_port"+literal), 1)), "invalid implied_port %q", literal)
		}
		for _, literal := range []string{"i-1e", "i65536e", "i9223372036854775808e"} {
			require.Falsef(t, validDHTMessage(bytes.Replace(wires[6], []byte("4:porti6881e"), []byte("4:port"+literal), 1)), "invalid port %q", literal)
		}
		for _, n := range []string{"9223372036854775807", "-9223372036854775808"} {
			parser := &dhtBencode{w: []byte("i" + n + "e"), limit: 1, maxDepth: 1}
			value, err := parser.value(0)
			require.NoError(t, err)
			require.IsType(t, int64(0), value)
			require.Equal(t, n, fmt.Sprint(value))
		}
	})
	t.Run("nested-extension-budget", func(t *testing.T) {
		wire := append(bytes.Clone(ping[:len(ping)-1]), []byte("1:zlli1eeee")...)
		fields, err := decodeDHTMessageWithBudget(wire, 32, 3)
		require.NoError(t, err)
		require.Equal(t, want, fields)
		fields, err = decodeDHTMessageWithBudget(wire, 32, 2)
		require.Nil(t, fields)
		rocTypedError(t, "ResourceExceeded", err)
	})
	for _, deferred := range []bool{false, true} {
		for _, kind := range []string{"collection", "depth"} {
			t.Run(fmt.Sprintf("budget/deferred%t/%s", deferred, kind), func(t *testing.T) {
				b := DefaultParserBudget()
				if kind == "collection" {
					b.MaxCollectionElements = 10
				} else {
					b.MaxRecursionDepth = 1
				}
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(46000, 6881))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				out := s.Feed(0, time.Unix(1, 0), ping)
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Equal(t, len(ping), out.Consumed)
				require.False(t, out.NeedMore)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, "bittorrent-dht", e.Protocol)
				require.Equal(t, "limited", e.Status)
				require.Nil(t, e.Session)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				fields, err := e.GetFields()
				require.Nil(t, fields)
				rocTypedError(t, "ResourceExceeded", err)
				_, err = e.Decode()
				rocTypedError(t, "ResourceExceeded", err)
				require.Zero(t, s.Stats().BufferedBytes)
				if kind == "collection" {
					// The BEP 5 error wire requires nine elements, below this same limit.
					wire := []byte("d1:eli201e23:A Generic Error Ocurrede1:t2:aa1:y1:ee")
					next := s.Feed(1, time.Unix(2, 0), wire)
					require.Nil(t, next.Err)
					require.Len(t, next.Events, 1)
					assertOwnedDatagram(t, next.Events[0], "bittorrent-dht", wire, map[string]any{"Transaction ID": []byte("aa"), "Packet Name": "error", "Error Code": int64(201), "Error Message": "A Generic Error Ocurred"})
				}
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Zero(t, s.Stats().Flows)
				// Adjacent inclusive limit: eleven elements and depth two admit ping.
				b.MaxCollectionElements, b.MaxRecursionDepth = 11, 2
				neighbor, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(46000, 6881))
				require.NoError(t, err)
				defer neighbor.Close("cleanup")
				next := neighbor.Feed(0, time.Unix(3, 0), ping)
				require.Nil(t, next.Err)
				require.Len(t, next.Events, 1)
				assertOwnedDatagram(t, next.Events[0], "bittorrent-dht", ping, want)
				require.Empty(t, neighbor.Close("done"))
				require.Zero(t, neighbor.Stats().BufferedBytes)
			})
		}
	}
}
