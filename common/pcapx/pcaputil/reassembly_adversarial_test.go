package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// Each expected byte string is independent of the production reassembler.
// Run the same adversarial input through synchronous and queued ingestion,
// streaming and late readers, including the IPv6 address representation.
func TestTCPAdversarialSegments(t *testing.T) {
	cases := []struct {
		name  string
		steps []tcpStep
		want  string
		bad   bool
	}{
		{"repeated_syn_does_not_rewind", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 99, syn: true}, {seq: 103, data: "def"}}, "abcdef", false},
		{"different_syn_cannot_inject", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 102, syn: true, data: "EVIL"}, {seq: 103, data: "def"}}, "abcdef", true},
		{"syn_fin_cannot_close", []tcpStep{{seq: 99, syn: true}, {seq: 99, syn: true, fin: true}, {seq: 100, data: "abc"}}, "abc", true},
		{"syn_rst_cannot_close", []tcpStep{{seq: 99, syn: true}, {seq: 99, syn: true, rst: true}, {seq: 100, data: "abc"}}, "abc", true},
		{"stale_rst_cannot_close", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 100, rst: true}, {seq: 103, data: "def"}}, "abcdef", true},
		{"future_rst_cannot_close", []tcpStep{{seq: 99, syn: true}, {seq: 120, rst: true}, {seq: 100, data: "abc"}}, "abc", true},
		{"reset_before_data_capture_order", []tcpStep{{seq: 99, syn: true}, {seq: 106, rst: true}, {seq: 103, data: "def"}, {seq: 100, data: "abc"}, {seq: 106, data: "ignored"}}, "abcdef", false},
		{"conflicting_future_resets", []tcpStep{{seq: 99, syn: true}, {seq: 103, rst: true}, {seq: 104, rst: true}, {seq: 100, data: "abc"}}, "abc", true},
		{"reset_before_fin_capture_order", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 104, rst: true}, {seq: 103, fin: true}}, "abc", false},
		{"reset_before_fin_duplicate", []tcpStep{{seq: 99, syn: true}, {seq: 101, rst: true}, {seq: 101, rst: true}, {seq: 100, fin: true}}, "", false},
		{"reset_before_fin_missing_fin", []tcpStep{{seq: 99, syn: true}, {seq: 101, rst: true}}, "", true},
		{"reset_before_fin_cannot_consume_data", []tcpStep{{seq: 99, syn: true}, {seq: 101, rst: true}, {seq: 100, data: "abc"}, {seq: 103, fin: true}}, "abc", true},
		{"reset_before_fin_wrap", []tcpStep{{seq: math.MaxUint32 - 1, syn: true}, {seq: 0, rst: true}, {seq: math.MaxUint32, fin: true}}, "", false},
		{"valid_rst_ignores_payload", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 103, rst: true, data: "EVIL"}, {seq: 103, data: "ignored"}}, "abc", false},
		{"ack_only_does_not_advance", []tcpStep{{seq: 99, syn: true}, {seq: 9999}, {seq: 100, data: "abc"}}, "abc", false},
		{"keepalive_does_not_duplicate", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 102, data: "c"}, {seq: 102}, {seq: 103, data: "def"}}, "abcdef", false},
		{"wrap_duplicate_and_overlap", []tcpStep{{seq: math.MaxUint32 - 2, syn: true}, {seq: 1, data: "def"}, {seq: math.MaxUint32 - 1, data: "abc"}, {seq: 0, data: "cdefg"}}, "abcdefg", false},
		{"half_space_ambiguity", []tcpStep{{seq: 99, syn: true}, {seq: 100 + (1 << 31), data: "EVIL"}, {seq: 100, data: "abc"}}, "abc", true},
		{"far_future_gap", []tcpStep{{seq: 99, syn: true}, {seq: 100 + (1 << 30), data: "EVIL"}, {seq: 100, data: "abc"}}, "abc", true},
		{"far_future_fin", []tcpStep{{seq: 99, syn: true}, {seq: 100 + (1 << 30), fin: true}, {seq: 100, data: "abc"}}, "abc", true},
		{"old_data_never_replaces_delivered", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 100, data: "XYZ"}, {seq: 103, data: "def"}}, "abcdef", false},
		{"pending_same_start_first_bytes_win", []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "de"}, {seq: 103, data: "XYfg"}, {seq: 100, data: "abc"}}, "abcdefg", true},
		{"conflicting_nested_overlap", []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "XYZ"}, {seq: 102, data: "cdef"}, {seq: 100, data: "ab"}}, "abcdef", true},
		{"conflicting_nested_reverse_arrival", []tcpStep{{seq: 99, syn: true}, {seq: 102, data: "cdef"}, {seq: 103, data: "XYZ"}, {seq: 100, data: "ab"}}, "abcdef", true},
		{"conflicting_bridge", []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "XYZ"}, {seq: 100, data: "abcdef"}}, "abcdef", true},
		{"pending_nested_duplicate", []tcpStep{{seq: 99, syn: true}, {seq: 102, data: "cdefgh"}, {seq: 104, data: "ef"}, {seq: 100, data: "ab"}}, "abcdefgh", false},
		{"pending_equal_end_overlap", []tcpStep{{seq: 99, syn: true}, {seq: 104, data: "ef"}, {seq: 102, data: "cdef"}, {seq: 100, data: "abc"}}, "abcdef", false},
		{"in_order_bridge_multiple_pending", []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "def"}, {seq: 106, data: "ghi"}, {seq: 100, data: "abcdefghi"}}, "abcdefghi", false},
		{"fin_before_gap_fill", []tcpStep{{seq: 99, syn: true}, {seq: 106, fin: true}, {seq: 103, data: "def"}, {seq: 100, data: "abc"}}, "abcdef", false},
		{"fin_clips_later_data", []tcpStep{{seq: 99, syn: true}, {seq: 103, fin: true}, {seq: 100, data: "abcdef"}}, "abc", false},
		{"fin_clips_earlier_pending_data", []tcpStep{{seq: 99, syn: true}, {seq: 102, data: "cdef"}, {seq: 104, fin: true}, {seq: 100, data: "ab"}}, "abcd", false},
		{"data_after_pending_fin_ignored", []tcpStep{{seq: 99, syn: true}, {seq: 103, fin: true}, {seq: 104, data: "EVIL"}, {seq: 100, data: "abc"}}, "abc", false},
		{"longer_duplicate_cannot_remove_fin", []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "def", fin: true}, {seq: 103, data: "defEVIL"}, {seq: 100, data: "abc"}}, "abcdef", false},
		{"conflicting_fin_cannot_move_end", []tcpStep{{seq: 99, syn: true}, {seq: 106, fin: true}, {seq: 103, fin: true}, {seq: 100, data: "abcdef"}}, "abcdef", true},
		{"fin_retransmit_at_cursor", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 100, data: "abc", fin: true}, {seq: 104, data: "ignored"}}, "abc", false},
		{"stale_fin_before_cursor", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abcdef"}, {seq: 101, fin: true}, {seq: 106, data: "ghi"}}, "abcdefghi", false},
		{"zero_sequence_and_empty_segments", []tcpStep{{seq: math.MaxUint32, syn: true}, {seq: 0}, {seq: 0, data: "a"}, {seq: 1, fin: true}}, "a", false},
		{"wrap_fin_boundary", []tcpStep{{seq: math.MaxUint32 - 2, syn: true}, {seq: 1, fin: true}, {seq: math.MaxUint32 - 1, data: "abcEVIL"}}, "abc", false},
	}
	for _, tc := range cases {
		for _, workers := range []int{1, 4} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/workers=%d/stream=%v", tc.name, workers, stream), func(t *testing.T) {
					got, err := adversarialReplay(t, tc.steps, workers, stream, workers == 4)
					require.Equal(t, tc.want, got)
					if tc.bad {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				})
			}
		}
	}
}

func adversarialReplay(t testing.TB, steps []tcpStep, workers int, stream, ipv6 bool) (string, error) {
	t.Helper()
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Workers: workers, Stream: stream, MaxFrameBytes: 13})
	defer p.Close()
	var got bytes.Buffer
	var flow *TrafficFlow
	p.onFlowCreated = func(f *TrafficFlow) { flow = f }
	p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { got.Write(f.Payload) })
	p.startWorkers()
	var ip gopacket.SerializableLayer = &layers.IPv4{SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
	if ipv6 {
		ip = &layers.IPv6{SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	}
	for _, s := range steps {
		tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, Seq: s.seq, SYN: s.syn, FIN: s.fin, RST: s.rst, ACK: !s.syn}
		tcp.Payload = []byte(s.data)
		p.Feed(nil, ip, tcp, time.Unix(1700000000, 0))
		clear(tcp.Payload)
	}
	p.Close()
	if !stream && flow != nil {
		data, err := io.ReadAll(flow.ClientConn.GetBuffer())
		require.NoError(t, err)
		require.Equal(t, got.String(), string(data))
	}
	if workers > 1 {
		s := p.Stats()
		require.Equal(t, s.AcceptedPackets, s.ProcessedPackets)
		require.Zero(t, p.parallel.budget.bytes)
		require.Zero(t, p.parallel.budget.segments)
		require.Zero(t, p.parallel.budget.flows)
	}
	return got.String(), p.Err()
}

// Generate overlapping segments from a separate source byte array. Their
// arrival order, retransmits, split positions and ISN vary independently.
func TestTCPAdversarialRandomOverlap(t *testing.T) {
	for seed := int64(0); seed < 80; seed++ {
		r := rand.New(rand.NewSource(seed))
		source := make([]byte, 256)
		r.Read(source)
		base := r.Uint32()
		steps := []tcpStep{{seq: base - 1, syn: true}}
		for i := 0; i < 80; i++ {
			begin := r.Intn(len(source))
			end := begin + 1 + r.Intn(len(source)-begin)
			steps = append(steps, tcpStep{seq: base + uint32(begin), data: string(source[begin:end])})
		}
		steps = append(steps, tcpStep{seq: base, data: string(source)}, tcpStep{seq: base + uint32(len(source)), fin: true})
		got, err := adversarialReplay(t, steps, 1+int(seed%2)*3, true, seed%2 != 0)
		require.NoError(t, err, "seed %d", seed)
		require.Equal(t, string(source), got, "seed %d", seed)
	}
}

func TestTCPSequenceWindowAndFloodBounds(t *testing.T) {
	_, err := (TCPReassemblyOptions{MaxSequenceGap: -1}).normalized()
	require.Error(t, err)
	if uint64(^uint(0)>>1) >= 1<<31 {
		invalid := uint64(1 << 31)
		_, err = (TCPReassemblyOptions{MaxSequenceGap: int(invalid)}).normalized()
		require.Error(t, err)
	}
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			p := newTrafficPool(context.Background(), TCPReassemblyOptions{Workers: workers, Stream: true, MaxSequenceGap: 64, MaxPendingBytes: 32, MaxPendingSegments: 8})
			defer p.Close()
			var got bytes.Buffer
			p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { got.Write(f.Payload) })
			p.startWorkers()
			ip := &layers.IPv4{SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
			feed := func(seq uint32, data string, syn bool) {
				tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, Seq: seq, SYN: syn, ACK: !syn}
				tcp.Payload = []byte(data)
				p.Feed(nil, ip, tcp)
			}
			feed(9999, string(bytes.Repeat([]byte{'x'}, 65)), false) // must not initialize the cursor
			feed(99, "", true)
			for i := 0; i < 2000; i++ {
				feed(1<<30+uint32(i), "garbage", false)
			}
			feed(103, "def", false)
			feed(100, "abc", false)
			p.Close()
			require.Equal(t, "abcdef", got.String())
			require.Error(t, p.Err())
			if workers > 1 {
				s := p.Stats()
				require.Equal(t, s.AcceptedPackets, s.ProcessedPackets)
				require.Equal(t, uint64(2001), s.InvalidSegments)
				require.Zero(t, s.UnreassembledBytes)
				require.Zero(t, p.parallel.budget.bytes)
				require.Zero(t, p.parallel.budget.segments)
			} else {
				require.Zero(t, p.pendingBytes)
				require.Zero(t, p.pendingSegments)
			}
		})
	}
}
