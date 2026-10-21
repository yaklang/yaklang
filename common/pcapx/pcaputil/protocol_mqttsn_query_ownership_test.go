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

// These literals describe the complete selected datagrams in the historical
// capture. The MQTT-SN 1.2 layouts were checked against Scapy v2.6.1's codec
// source and the capture's fields against actual tshark 4.2.5 PDML. They do
// not assert a successful gateway endpoint, QoS delivery or session matching.
func mqttsnOwnedInputs(t *testing.T) ([]byte, [][]byte, []map[string]any) {
	t.Helper()
	capture, err := trafficfixture.ReadFile("../../bin-parser/testdata/winlab5013/captures/08-mqttsn.pcapng")
	require.NoError(t, err)
	require.Equal(t, "d57651b77ca675deb298bc9f55337f64ef209a02eac87bf6c98bde7844a316ca", fmt.Sprintf("%x", sha256.Sum256(capture)))
	wireHex := []string{"030101", "030201", "0c040401003c77696e6c6162", "030500", "0e0a000000016c61622f74656d70", "070b0001000100", "0b0c200001000232332e35", "070d0001000200", "0218"}
	names := []string{"SEARCHGW", "GWINFO", "CONNECT", "CONNACK", "REGISTER", "REGACK", "PUBLISH", "PUBACK", "DISCONNECT"}
	codes := []byte{1, 2, 4, 5, 10, 11, 12, 13, 24}
	extras := []map[string]any{
		{"Radius": byte(1)}, {"Gateway ID": byte(1)},
		{"Flags": byte(4), "Protocol ID": byte(1), "Duration": uint16(60), "Client ID": "winlab"},
		{"Return Code": byte(0)},
		{"Topic ID": uint16(0), "Message ID": uint16(1), "Topic Name": "lab/temp"},
		{"Topic ID": uint16(1), "Message ID": uint16(1), "Return Code": byte(0)},
		{"Flags": byte(32), "QoS": byte(1), "Topic ID": uint16(1), "Message ID": uint16(2), "Data": []byte("23.5")},
		{"Topic ID": uint16(1), "Message ID": uint16(2), "Return Code": byte(0)}, {},
	}
	var wires [][]byte
	for i, h := range wireHex {
		wire, err := hex.DecodeString(h)
		require.NoError(t, err)
		wires = append(wires, wire)
		extras[i]["Length"], extras[i]["Message Type Code"], extras[i]["Message Type"] = len(wire), codes[i], names[i]
	}
	return capture, wires, extras
}

func mqttsnAssertOwnedQuery(t *testing.T, e *ProtocolEvent, want map[string]any, wire []byte) {
	t.Helper()
	require.Equal(t, "mqtt-sn", e.Protocol)
	require.Equal(t, "mqtt-sn-native", e.Profile)
	require.Equal(t, "udp", e.Transport)
	require.Equal(t, "message", e.Completeness)
	require.Empty(t, e.Error)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, len(wire), e.Length)
	require.Equal(t, want, e.Session)
	f, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, f)
	mutateRTPSQuery(f)
	tree, err := e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, protocolFields(tree))
	mutateRTPSQuery(tree)
	mutateRTPSQuery(e.Session)
	mutateRTPSQuery(e.Fields)
	mutateRTPSQuery(e.Structured)
	again, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, again)
	tree, err = e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, protocolFields(tree))
	require.Equal(t, want, tree["session"])
	e.Session = nil
	tree, err = e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, tree["session"])
	require.Equal(t, wire, e.Raw)
}

func testMQTTSNOwnedQueries(t *testing.T) {
	capture, wires, fields := mqttsnOwnedInputs(t)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%t/observe%t", workers, deferred, observe), func(t *testing.T) {
					var packets atomic.Int64
					opts := []CaptureOption{WithProtocolDeferred(deferred)}
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { packets.Add(1) }))
					}
					events, stats, err := binReplay(t, capture, workers, opts...)
					require.NoError(t, err)
					require.Len(t, events, 9)
					require.Zero(t, stats.Malformed)
					require.Zero(t, stats.BufferedBytes)
					ids := map[uint64]bool{}
					for i, e := range events {
						require.NotZero(t, e.ID)
						require.False(t, ids[e.ID])
						ids[e.ID] = true
						require.Zero(t, e.Direction)
						endpoints := []string{"192.0.2.10:41883", "192.0.2.20:1883"}
						require.Equal(t, endpoints[i%2], e.Source)
						require.Equal(t, endpoints[1-i%2], e.Destination)
						require.True(t, time.Unix(1758585600, int64(i+1)*1000000).Equal(e.Timestamp))
						require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wires[i])), e.SourceBytes.SHA256)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						mqttsnAssertOwnedQuery(t, e, fields[i], wires[i])
					}
					if observe {
						require.EqualValues(t, 9, packets.Load())
					}
				})
			}
		}
	}
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("deferred%t/datagram-boundaries", deferred), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41883, 1883))
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = deferred
			// Split datagrams must not form a PUBLISH, and two complete
			// datagrams concatenated into one are not MQTT-SN framing. An
			// unsupported shape is not admitted as a native MQTT-SN error.
			bad := [][]byte{wires[6][:6], wires[6][6:], append(bytes.Clone(wires[0]), wires[1]...), {3, 5, 4}, {1, 0, 6, 24, 0}}
			for i, wire := range bad {
				out := s.Feed(i%2, time.Unix(int64(i+1), 0), bytes.Clone(wire))
				require.False(t, out.NeedMore)
				require.Equal(t, len(wire), out.Consumed)
				require.Equal(t, "undetected", out.State)
				rocTypedError(t, "UnsupportedFeature", out.Err)
				require.Len(t, out.Events, 1)
				for _, e := range out.Events {
					require.Empty(t, e.Protocol)
					require.Equal(t, "unrecognized", e.Status)
					require.Equal(t, wire, e.Raw)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
				require.Zero(t, s.Stats().BufferedBytes)
			}
			require.EqualValues(t, len(bad), s.Stats().Unknown)
			owned := bytes.Clone(wires[6])
			out := s.Feed(0, time.Unix(20, 0), owned)
			require.Nil(t, out.Err)
			require.False(t, out.NeedMore)
			require.Len(t, out.Events, 1)
			clear(owned)
			mqttsnAssertOwnedQuery(t, out.Events[0], fields[6], wires[6])
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Zero(t, s.Stats().Flows)
		})
		t.Run(fmt.Sprintf("deferred%t/message-budget", deferred), func(t *testing.T) {
			budget := DefaultParserBudget()
			budget.MaxMessageBytes, budget.MaxFrameBytes = 64, 64
			s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(41883, 1883))
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = deferred
			atLimit := append(bytes.Clone(wires[6][:7]), bytes.Repeat([]byte{'a'}, 57)...)
			atLimit[0] = 64
			good := s.Feed(0, time.Unix(1, 0), atLimit)
			require.Nil(t, good.Err)
			require.Len(t, good.Events, 1)
			want := cloneSession(fields[6])
			want["Length"], want["Data"] = 64, bytes.Repeat([]byte{'a'}, 57)
			mqttsnAssertOwnedQuery(t, good.Events[0], want, atLimit)
			overLimit := append(bytes.Clone(atLimit), 'a')
			overLimit[0] = 65
			out := s.Feed(0, time.Unix(2, 0), overLimit)
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.False(t, out.NeedMore)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			require.Equal(t, "limited", e.Status)
			require.Empty(t, e.Protocol) // Rejected before native protocol admission.
			require.Nil(t, e.Session)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, overLimit[:64], e.Raw) // Bounded probe evidence.
			require.EqualValues(t, len(overLimit), s.Stats().LimitedBytes)
			require.Zero(t, s.Stats().BufferedBytes)
			next := s.Feed(0, time.Unix(3, 0), wires[6])
			require.Nil(t, next.Err)
			require.Len(t, next.Events, 1)
			mqttsnAssertOwnedQuery(t, next.Events[0], fields[6], wires[6])
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Zero(t, s.Stats().Flows)
		})
	}
}
