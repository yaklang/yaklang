package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// The key is public synthetic test material, never a real device credential.
func mavlinkSigningTestKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

type mavlinkSigningReference struct {
	ID       string `json:"id"`
	Hex      string `json:"hex"`
	Before   uint64 `json:"receiver_before"`
	After    uint64 `json:"receiver_after"`
	Accepted bool   `json:"accepted"`
	Status   int    `json:"status"`
	Count    int    `json:"stream_count"`
}

func mavlinkSigningReferences(t *testing.T) []mavlinkSigningReference {
	b, err := trafficfixture.ReadBatch("mavlink-signing", "validation/reference-signing.jsonl")
	require.NoError(t, err)
	var rows []mavlinkSigningReference
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
		var r mavlinkSigningReference
		require.NoError(t, json.Unmarshal(line, &r))
		rows = append(rows, r)
	}
	require.Len(t, rows, 17)
	return rows
}

func mavlinkSigningWire(t *testing.T, r mavlinkSigningReference) []byte {
	b, err := hex.DecodeString(r.Hex)
	require.NoError(t, err)
	return b
}

// Bitwise CRC in mavlinkTestFrame and this SHA256 signer are independent of
// the production verifier. Official C-produced vectors additionally pin both.
func mavlinkSigningTestPacket(id uint32, payload []byte, sys, comp, link byte, stamp uint64) []byte {
	extra := map[uint32]byte{0: 50, 1: 124, 24: 24, 30: 39, 33: 104}[id]
	w := mavlinkTestFrame(2, id, payload, 1, extra)
	w[5], w[6] = sys, comp
	crc := uint16(0xffff)
	for _, b := range append(bytes.Clone(w[1:len(w)-15]), extra) {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0x8408
			} else {
				crc >>= 1
			}
		}
	}
	binary.LittleEndian.PutUint16(w[len(w)-15:], crc)
	sig := w[len(w)-13:]
	sig[0] = link
	for i := 0; i < 6; i++ {
		sig[1+i] = byte(stamp >> (8 * i))
	}
	h := sha256.Sum256(append(mavlinkSigningTestKey(), w[:len(w)-6]...))
	copy(sig[7:], h[:6])
	return w
}

func mavlinkSigningSession(t *testing.T, budget ParserBudget, clock uint64, maxStreams int, deferred bool) *captureSession {
	s, err := NewProtocolSessionWithOptions(budget,
		WithSessionMAVLinkSigning(MAVLinkSigningConfig{Key: mavlinkSigningTestKey(), InitialTimestamp: clock, MaxStreams: maxStreams}),
		WithSessionPorts(40000, 14550), WithSessionTransport("udp"))
	require.NoError(t, err)
	c := s.(*captureSession)
	c.f.a.config.Deferred = deferred
	t.Cleanup(func() { c.Close("cleanup") })
	return c
}

// Complete unsigned/common header projection from the fixed C heartbeat wire.
// It is independent of mavlinkFields and decoder-generated answers.
func mavlinkSigningHeartbeatFields(w []byte) map[string]any {
	return map[string]any{
		"Version": 2, "Payload Length": 9, "Sequence": 0, "System ID": int(w[5]), "Component ID": int(w[6]),
		"Message ID": uint32(0), "Message Name": "HEARTBEAT", "CRC Extra": 50,
		"Summary":  fmt.Sprintf("MAVLink v2 HEARTBEAT system %d component %d", w[5], w[6]),
		"Checksum": binary.LittleEndian.Uint16(w[19:21]), "Signature Present": true,
		"Signature Status": "verified-receiver-context", "Observation": "unverified-telemetry",
		"Incompatibility Flags": 1, "Compatibility Flags": 0, "Signature": bytes.Clone(w[21:]),
		"Message Fields": map[string]any{"custom_mode": uint32(42), "type": byte(2), "autopilot": byte(3), "base_mode": byte(129), "system_status": byte(4), "mavlink_version": byte(3)},
	}
}

func mavlinkSigningObservation(w []byte, before, after uint64, previous *uint64) map[string]any {
	stamp := uint64(0)
	for i := 0; i < 6; i++ {
		stamp |= uint64(w[len(w)-12+i]) << (8 * i)
	}
	f := map[string]any{"Context": "caller-supplied-key-and-receiver-clock", "Scope": "capture-domain/system/component/link",
		"Link ID": int(w[len(w)-13]), "Timestamp": stamp, "Timestamp Units": "10us-since-2015-01-01-UTC",
		"Receiver Timestamp Before": before, "Receiver Timestamp After": after, "Previous Timestamp Available": previous != nil}
	if previous != nil {
		f["Previous Timestamp"] = *previous
	}
	return f
}

func mavlinkSigningAssertRefusal(t *testing.T, out FeedResult, w []byte, kind string) {
	rocTypedError(t, kind, out.Err)
	require.Equal(t, len(w), out.Consumed)
	require.Len(t, out.Events, 1)
	e := out.Events[0]
	require.Equal(t, w, e.Raw)
	require.Nil(t, e.Session)
	require.Nil(t, e.Fields)
	require.Nil(t, e.Structured)
	f, err := e.GetFields()
	require.Nil(t, f)
	rocTypedError(t, kind, err)
	assertOwnedProtocolRefusal(t, e, kind, err)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.Offset)
}

func testMAVLinkSigning(t *testing.T) {
	rows := mavlinkSigningReferences(t)
	t.Run("default-is-observation-not-authentication", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
		require.NoError(t, err)
		defer s.Close("done")
		for i, index := range []int{0, 1, 7} {
			r := rows[index]
			out := s.Feed(0, time.Unix(int64(i+1), 0), mavlinkSigningWire(t, r))
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			f, err := out.Events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, "opaque-unverified", f["Signature Status"])
			require.NotContains(t, f, "Signature Verification")
		}
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("all-supported-common-fields", func(t *testing.T) {
		type golden struct {
			raw    []byte
			fields map[string]any
		}
		var goldens []golden
		for _, id := range []string{"v2-heartbeat", "v2-sys_status", "v2-global_position_int", "v2-zero-tail-1", "v2-zero-tail-33"} {
			b, err := trafficfixture.ReadFile("incremental-mvp/mavlink/answers/" + id + ".json")
			require.NoError(t, err)
			var a struct {
				Fields []map[string]any `json:"native_expected_messages"`
				Raw    []string         `json:"native_raw_hex"`
			}
			d := json.NewDecoder(bytes.NewReader(b))
			d.UseNumber()
			require.NoError(t, d.Decode(&a))
			require.Len(t, a.Raw, 1)
			w, err := hex.DecodeString(a.Raw[0])
			require.NoError(t, err)
			goldens = append(goldens, golden{w, a.Fields[0]})
		}
		for _, cases := range [][]mavlinkAttitudeControl{mavlinkAttitudeControls(t), mavlinkGPSControls(t)} {
			for _, c := range cases {
				if c.Error != "" || len(c.Events) != 1 {
					continue
				}
				w, err := hex.DecodeString(c.Events[0].Raw)
				require.NoError(t, err)
				if w[0] == 0xfd {
					goldens = append(goldens, golden{w, c.Events[0].Fields})
				}
			}
		}
		require.Greater(t, len(goldens), 5)
		for _, deferred := range []bool{false, true} {
			s := mavlinkSigningSession(t, ParserBudget{}, 100000000, 16, deferred)
			for i, g := range goldens {
				id := uint32(g.raw[7]) | uint32(g.raw[8])<<8 | uint32(g.raw[9])<<16
				stamp := uint64(100000000 + i)
				w := mavlinkSigningTestPacket(id, g.raw[10:10+int(g.raw[1])], 1, 2, 9, stamp)
				out := s.Feed(i%2, time.Unix(int64(i+1), 0), w)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				want := cloneSession(g.fields)
				want["Sequence"], want["System ID"], want["Component ID"], want["Incompatibility Flags"] = 7, 1, 2, 1
				want["Checksum"] = binary.LittleEndian.Uint16(w[len(w)-15:])
				want["Signature Present"], want["Signature Status"], want["Signature"] = true, "verified-receiver-context", bytes.Clone(w[len(w)-13:])
				before := uint64(100000000)
				var previous *uint64
				if i > 0 {
					before = stamp - 1
					previous = &before
				}
				want["Signature Verification"] = mavlinkSigningObservation(w, before, stamp, previous)
				got, err := out.Events[0].GetFields()
				require.NoError(t, err)
				mavlinkGPSEqual(t, want, got)
				require.Zero(t, out.Events[0].ResponseTo)
				require.Zero(t, out.Events[0].TransactionID)
				out.Events[0].Session["Signature Verification"].(map[string]any)["Timestamp"] = uint64(999999)
				got, err = out.Events[0].GetFields()
				require.NoError(t, err)
				mavlinkGPSEqual(t, want, got)
			}
		}
	})
	t.Run("configuration-key-ownership-and-reuse", func(t *testing.T) {
		for _, bad := range []MAVLinkSigningConfig{{}, {Key: make([]byte, 31)}, {Key: make([]byte, 33)}, {Key: mavlinkSigningTestKey(), InitialTimestamp: 1 << 48}, {Key: mavlinkSigningTestKey(), MaxStreams: -1}, {Key: mavlinkSigningTestKey(), MaxStreams: 4097}} {
			_, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"), WithSessionMAVLinkSigning(bad))
			require.Error(t, err)
			require.Error(t, WithProtocolMAVLinkSigning(bad)(NewDefaultConfig()))
		}
		for _, transport := range []string{"", "tcp"} {
			opts := []ProtocolSessionOption{WithSessionMAVLinkSigning(MAVLinkSigningConfig{Key: mavlinkSigningTestKey()})}
			if transport != "" {
				opts = append(opts, WithSessionTransport(transport))
			}
			_, err := NewProtocolSessionWithOptions(ParserBudget{}, opts...)
			require.ErrorContains(t, err, "UDP session")
		}
		key := mavlinkSigningTestKey()
		option := WithSessionMAVLinkSigning(MAVLinkSigningConfig{Key: key, InitialTimestamp: 100000000})
		clear(key)
		for i := 0; i < 2; i++ {
			s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"), WithSessionPorts(40000, 14550), option)
			require.NoError(t, err)
			w := mavlinkSigningWire(t, rows[0])
			require.Nil(t, s.Feed(0, time.Unix(1, 0), w).Err)
			mavlinkSigningAssertRefusal(t, s.Feed(0, time.Unix(2, 0), w), w, "AuthenticationFailed")
			s.Close("done")
			require.Zero(t, s.Stats().BufferedBytes)
		}
		// Constructor failure after an option must wipe its private live state.
		var partial *captureSession
		_, err := NewProtocolSessionWithOptions(ParserBudget{}, option, func(s *captureSession) error { partial = s; return fmt.Errorf("later option refused") })
		require.Error(t, err)
		require.True(t, partial.f.a.mavlinkSigning.closed)
		require.Equal(t, [32]byte{}, partial.f.a.mavlinkSigning.options.key)
	})
	t.Run("unsigned-crc-and-datagram-boundaries", func(t *testing.T) {
		s := mavlinkSigningSession(t, ParserBudget{}, 100000000, 16, false)
		for _, version := range []int{1, 2} {
			w := mavlinkTestFrame(version, 0, mavlinkTestPayload(0), 0, 50)
			mavlinkSigningAssertRefusal(t, s.Feed(0, time.Unix(1, 0), w), w, "AuthenticationFailed")
		}
		w := mavlinkSigningWire(t, rows[0])
		bad := bytes.Clone(w)
		bad[10] ^= 1
		mavlinkSigningAssertRefusal(t, s.Feed(0, time.Unix(2, 0), bad), bad, "MalformedMessage")
		for n := 10; n < len(w); n++ {
			out := s.Feed(0, time.Unix(3, 0), w[:n])
			mavlinkSigningAssertRefusal(t, out, w[:n], "NeedMore")
		}
		require.Zero(t, s.f.a.mavlinkSigning.count)
		require.Zero(t, s.Stats().BufferedBytes)
		require.Nil(t, s.Feed(1, time.Unix(4, 0), w).Err)
	})
	t.Run("interface-and-endpoint-isolation", func(t *testing.T) {
		wire, err := trafficfixture.ReadBatch("mavlink-signing", "captures/signing-replay.pcap")
		require.NoError(t, err)
		packet := bytes.Clone(wire[40 : 40+int(binary.LittleEndian.Uint32(wire[32:36]))])
		changed := bytes.Clone(packet)
		changed[29] = 3
		binary.BigEndian.PutUint16(changed[34:36], 40002)
		var ng bytes.Buffer
		writer, err := pcapgo.NewNgWriter(&ng, layers.LinkTypeEthernet)
		require.NoError(t, err)
		id, err := writer.AddInterface(pcapgo.NgInterface{LinkType: layers.LinkTypeEthernet, SnapLength: 65535})
		require.NoError(t, err)
		require.Equal(t, 1, id)
		for i, step := range []struct {
			iface int
			wire  []byte
		}{{0, packet}, {0, changed}, {1, packet}, {1, changed}} {
			require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(int64(i+1), 0), CaptureLength: len(step.wire), Length: len(step.wire), InterfaceIndex: step.iface}, step.wire))
		}
		require.NoError(t, writer.Flush())
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				var events []*ProtocolEvent
				var stats ProtocolStats
				require.NoError(t, ReplayPcap(bytes.NewReader(ng.Bytes()), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolMAVLinkSigning(MAVLinkSigningConfig{Key: mavlinkSigningTestKey(), InitialTimestamp: 100000000}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
				require.Len(t, events, 4)
				require.NotEqual(t, events[0].Domain, events[2].Domain)
				for i, e := range events {
					require.Equal(t, i/2, e.Domain.Interface)
					require.Equal(t, e.Domain, events[i-i%2].Domain)
					if i%2 == 0 {
						f, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, "verified-receiver-context", f["Signature Status"])
					} else {
						f, err := e.GetFields()
						require.Nil(t, f)
						rocTypedError(t, "AuthenticationFailed", err)
						require.Equal(t, "192.0.2.3:40002", e.Source)
					}
				}
				require.EqualValues(t, 2, stats.Malformed)
				require.Zero(t, stats.BufferedBytes)
			}
		}
		// Section and nested tunnel identifiers must also isolate clock/history.
		s := mavlinkSigningSession(t, ParserBudget{}, 100000000, 16, false)
		for _, domain := range []CaptureDomain{{}, {Section: 1}, {Encapsulation: "/vlan:7/vxlan:99"}} {
			base := &ProtocolEvent{Domain: domain}
			events, ok := s.f.a.decodeMAVLinkDatagram(base, mavlinkSigningWire(t, rows[0]), 40000, 14550)
			require.True(t, ok)
			require.Len(t, events, 1)
			f, err := events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, "verified-receiver-context", f["Signature Status"])
			base = &ProtocolEvent{Domain: domain}
			events, _ = s.f.a.decodeMAVLinkDatagram(base, mavlinkSigningWire(t, rows[0]), 40000, 14550)
			_, err = events[0].GetFields()
			rocTypedError(t, "AuthenticationFailed", err)
		}
		require.Equal(t, 3, s.f.a.mavlinkSigning.count)
	})
	t.Run("official-sequence-and-query-ownership", func(t *testing.T) {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprint(deferred), func(t *testing.T) {
				s := mavlinkSigningSession(t, ParserBudget{}, 100000000, 16, deferred)
				last := map[[3]byte]uint64{}
				var previousID uint64
				for i, r := range rows[:9] {
					w := mavlinkSigningWire(t, r)
					input := bytes.Clone(w)
					out := s.Feed(i%2, time.Unix(int64(i+1), 0), input)
					clear(input)
					if !r.Accepted {
						mavlinkSigningAssertRefusal(t, out, w, "AuthenticationFailed")
					} else {
						require.Nil(t, out.Err)
						require.Len(t, out.Events, 1)
						require.Equal(t, len(w), out.Consumed)
						want := mavlinkSigningHeartbeatFields(w)
						k := [3]byte{w[5], w[6], w[21]}
						v, known := last[k]
						var p *uint64
						if known {
							p = &v
						}
						want["Signature Verification"] = mavlinkSigningObservation(w, r.Before, r.After, p)
						assertMAVLinkGoldenSession(t, out.Events[0], want)
						last[k] = want["Signature Verification"].(map[string]any)["Timestamp"].(uint64)
					}
					e := out.Events[0]
					require.Greater(t, e.ID, previousID)
					previousID = e.ID
					require.Equal(t, i%2, e.Direction)
					require.Equal(t, w, e.Raw)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.FlowID)
					require.Equal(t, r.After, s.f.a.mavlinkSigning.domains[CaptureDomain{}].clock)
					require.Equal(t, r.Count, s.f.a.mavlinkSigning.count)
					require.EqualValues(t, 256+128*r.Count, s.Stats().BufferedBytes)
				}
				require.EqualValues(t, 3, s.Stats().Malformed)
				require.EqualValues(t, 9, s.Stats().Messages)
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Equal(t, [32]byte{}, s.f.a.mavlinkSigning.options.key)
				require.Nil(t, s.f.a.mavlinkSigning.domains)
			})
		}
	})
	t.Run("clock-and-replay-boundaries", func(t *testing.T) {
		for _, i := range []int{9, 10, 14, 15, 16} {
			r := rows[i]
			t.Run(r.ID, func(t *testing.T) {
				key := mavlinkSigningTestKey()
				if i == 14 {
					key[0] ^= 1
				}
				s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"), WithSessionPorts(40000, 14550), WithSessionMAVLinkSigning(MAVLinkSigningConfig{Key: key, InitialTimestamp: r.Before}))
				require.NoError(t, err)
				defer s.Close("done")
				if i == 16 {
					out := s.Feed(0, time.Unix(1, 0), mavlinkSigningWire(t, rows[15]))
					require.Nil(t, out.Err)
				}
				w := mavlinkSigningWire(t, r)
				out := s.Feed(1, time.Unix(2, 0), w)
				if r.Accepted {
					require.Nil(t, out.Err)
					require.Len(t, out.Events, 1)
					want := mavlinkSigningHeartbeatFields(w)
					want["Signature Verification"] = mavlinkSigningObservation(w, r.Before, r.After, nil)
					assertMAVLinkGoldenSession(t, out.Events[0], want)
				} else {
					mavlinkSigningAssertRefusal(t, out, w, "AuthenticationFailed")
				}
			})
		}
		// A previously accepted stream may increase while behind the receiver
		// clock: only a NEW tuple has the fixed C one-minute admission window.
		s := mavlinkSigningSession(t, ParserBudget{}, 100000000, 16, false)
		for i, w := range [][]byte{mavlinkSigningWire(t, rows[0]), mavlinkSigningTestPacket(0, mavlinkTestPayload(0), 9, 2, 9, 106000001), mavlinkSigningWire(t, rows[11])} {
			require.Nil(t, s.Feed(0, time.Unix(int64(i+1), 0), w).Err)
		}
		require.EqualValues(t, 106000001, s.f.a.mavlinkSigning.domains[CaptureDomain{}].clock)
	})
	t.Run("atomic-datagram-and-budget", func(t *testing.T) {
		first, next := mavlinkSigningWire(t, rows[0]), mavlinkSigningWire(t, rows[3])
		bad := mavlinkSigningWire(t, rows[7])
		for _, tail := range [][]byte{bad, first, first[:len(first)-1], mavlinkTestFrame(2, 0, mavlinkTestPayload(0), 0, 50)} {
			s := mavlinkSigningSession(t, ParserBudget{}, 100000000, 16, false)
			pair := append(bytes.Clone(first), tail...)
			out := s.Feed(0, time.Unix(1, 0), pair)
			require.Error(t, out.Err)
			require.Len(t, out.Events, 1)
			require.Nil(t, out.Events[0].Session)
			require.Zero(t, s.Stats().BufferedBytes)
			require.Zero(t, s.f.a.mavlinkSigning.count)
			require.Nil(t, s.Feed(0, time.Unix(2, 0), first).Err)
			s.Close("done")
		}
		for name, budget := range map[string]ParserBudget{"retained": {MaxBufferedBytes: 383, MaxMessageBytes: 64}, "depth": {MaxRecursionDepth: 1}, "collection": {MaxCollectionElements: 1}} {
			t.Run(name, func(t *testing.T) {
				s := mavlinkSigningSession(t, budget, 100000000, 16, false)
				w := first
				if name == "collection" {
					w = append(bytes.Clone(first), next...)
				}
				mavlinkSigningAssertRefusal(t, s.Feed(0, time.Unix(1, 0), w), w, "ResourceExceeded")
				require.Zero(t, s.Stats().BufferedBytes)
				require.Zero(t, s.f.a.mavlinkSigning.count)
			})
		}
		s := mavlinkSigningSession(t, ParserBudget{MaxBufferedBytes: 384, MaxMessageBytes: 64}, 100000000, 1, false)
		require.Nil(t, s.Feed(0, time.Unix(1, 0), first).Err)
		other := mavlinkSigningWire(t, rows[4])
		mavlinkSigningAssertRefusal(t, s.Feed(1, time.Unix(2, 0), other), other, "ResourceExceeded")
		require.Nil(t, s.Feed(1, time.Unix(3, 0), next).Err)
		require.EqualValues(t, 384, s.Stats().BufferedBytes)
		mavlinkSigningAssertRefusal(t, s.Feed(0, time.Unix(4, 0), first), first, "AuthenticationFailed")
		// Stream-count and shared-retained-byte refusals are separate boundaries.
		shared := mavlinkSigningSession(t, ParserBudget{MaxBufferedBytes: 384, MaxMessageBytes: 64}, 100000000, 16, false)
		require.Nil(t, shared.Feed(0, time.Unix(1, 0), first).Err)
		mavlinkSigningAssertRefusal(t, shared.Feed(0, time.Unix(2, 0), other), other, "ResourceExceeded")
		require.Equal(t, 1, shared.f.a.mavlinkSigning.count)
		require.EqualValues(t, 384, shared.Stats().BufferedBytes)
		require.Nil(t, shared.Feed(1, time.Unix(3, 0), next).Err)
		pair := mavlinkSigningSession(t, ParserBudget{}, 100000000, 16, false)
		out := pair.Feed(0, time.Unix(1, 0), append(bytes.Clone(first), next...))
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 2)
		prev := uint64(100000000)
		for i, e := range out.Events {
			w := first
			var p *uint64
			after := prev
			if i == 1 {
				w = next
				p = &prev
				after++
			}
			want := mavlinkSigningHeartbeatFields(w)
			want["Signature Verification"] = mavlinkSigningObservation(w, prev, after, p)
			assertMAVLinkGoldenSession(t, e, want)
			require.EqualValues(t, i*len(first), e.Offset)
		}
		require.EqualValues(t, 384, pair.Stats().BufferedBytes)
		require.Equal(t, 1, pair.f.a.mavlinkSigning.count)
	})
	t.Run("official-sixteen-stream-capacity", func(t *testing.T) {
		s := mavlinkSigningSession(t, ParserBudget{}, 106000001, 16, false)
		for i := 0; i < 16; i++ {
			w := mavlinkSigningTestPacket(0, mavlinkTestPayload(0), byte(100+i), 2, 9, 106000001)
			require.Nil(t, s.Feed(0, time.Unix(int64(i+1), 0), w).Err)
		}
		w := mavlinkSigningWire(t, rows[13])
		mavlinkSigningAssertRefusal(t, s.Feed(0, time.Unix(17, 0), w), w, "ResourceExceeded")
		require.Equal(t, rows[13].Count, s.f.a.mavlinkSigning.count)
		require.Equal(t, rows[13].After, s.f.a.mavlinkSigning.domains[CaptureDomain{}].clock)
		old := mavlinkSigningTestPacket(0, mavlinkTestPayload(0), 9, 2, 9, 1)
		mavlinkSigningAssertRefusal(t, s.Feed(0, time.Unix(18, 0), old), old, "ResourceExceeded")
	})
	t.Run("capture-matrix", func(t *testing.T) {
		wire, err := trafficfixture.ReadBatch("mavlink-signing", "captures/signing-replay.pcap")
		require.NoError(t, err)
		key := mavlinkSigningTestKey()
		signingOption := WithProtocolMAVLinkSigning(MAVLinkSigningConfig{Key: key, InitialTimestamp: 100000000})
		clear(key)
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, observe := range []bool{false, true} {
					t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
						var events []*ProtocolEvent
						var stats ProtocolStats
						var seen atomic.Int64
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), signingOption, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
						if observe {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
						}
						require.NoError(t, ReplayPcap(bytes.NewReader(bytes.Clone(wire)), opts...))
						require.Len(t, events, 9)
						last := map[[3]byte]uint64{}
						var previousID uint64
						for i, e := range events {
							r := rows[i]
							w := mavlinkSigningWire(t, r)
							require.Greater(t, e.ID, previousID)
							previousID = e.ID
							require.Equal(t, w, e.Raw)
							require.Equal(t, "mavlink", e.Protocol)
							require.Zero(t, e.ResponseTo)
							require.Zero(t, e.TransactionID)
							require.Zero(t, e.Offset)
							require.Equal(t, time.Unix(int64(i+1), 0), e.Timestamp)
							require.Equal(t, "192.0.2.1:40000", e.Source)
							require.Equal(t, "192.0.2.2:14550", e.Destination)
							require.Len(t, e.SourceBytes.PacketRefs, 1)
							require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
							require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
							if r.Accepted {
								want := mavlinkSigningHeartbeatFields(w)
								k := [3]byte{w[5], w[6], w[21]}
								v, known := last[k]
								var p *uint64
								if known {
									p = &v
								}
								want["Signature Verification"] = mavlinkSigningObservation(w, r.Before, r.After, p)
								assertMAVLinkGoldenSession(t, e, want)
								last[k] = want["Signature Verification"].(map[string]any)["Timestamp"].(uint64)
							} else {
								f, err := e.GetFields()
								require.Nil(t, f)
								rocTypedError(t, "AuthenticationFailed", err)
								assertOwnedProtocolRefusal(t, e, "AuthenticationFailed", err)
							}
						}
						require.EqualValues(t, 9, stats.Messages)
						require.EqualValues(t, 3, stats.Malformed)
						require.Zero(t, stats.BufferedBytes)
						require.Zero(t, stats.CallbackPanics)
						if observe {
							require.EqualValues(t, 9, seen.Load())
						} else {
							require.Zero(t, seen.Load())
						}
					})
				}
			}
		}
	})
}
