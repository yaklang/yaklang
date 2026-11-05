package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
)

// RFC3931 4.1.1.2: zero SID4 + flags/length4 + CCID4 + Ns/Nr4 is
// the control header. The following eight-byte HELLO Message-Type AVP is body.
// Inputs are authored bytes, exercised through ReplayPcap and native ingress.
func TestN14L2TPKnownControlFirstHeaderCaptureIngress(t *testing.T) {
	control := []byte{0, 0, 0, 0, 0xc8, 3, 0, 20, 0x12, 0x34, 0x56, 0x78, 0xff, 0xfe, 0, 5, 0x80, 8, 0, 0, 0, 0, 0, 6}
	whole := l2tp3NativeFrame(t, control, false, true)
	for _, complete := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			for _, workers := range []int{1, 4} {
				t.Run(fmt.Sprintf("complete%t/reverse%t/w%d", complete, reverse, workers), func(t *testing.T) {
					split := 8
					if complete {
						split = 16
					}
					frames := n14AuditSplitIPv6Frame(t, whole, split)
					if reverse {
						frames[0], frames[1] = frames[1], frames[0]
					}
					var records []n14AuditRecord
					for _, frame := range frames {
						records = append(records, n14AuditRecord{raw: frame})
					}
					var events []*ProtocolEvent
					var stats ProtocolStats
					var conf *CaptureConfig
					flows, delivered := 0, 0
					options := []CaptureOption{
						func(c *CaptureConfig) error { conf = c; return nil },
						WithTCPReassemblyWorkers(workers),
						WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }),
						WithOnProtocolStats(func(s ProtocolStats) { stats = s }),
						WithOnTrafficFlowCreated(func(*TrafficFlow) { flows++ }),
						WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, frame *TrafficFrame) { delivered += len(frame.Payload) }),
						WithEveryPacket(func(gopacket.Packet) {
							require.Nil(t, conf.binParser.udpSessions)
							if !complete {
								require.Nil(t, conf.binParser.l2tp3, "refused first header must not reach L2TP session state")
							}
						}),
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), options...))
					require.Zero(t, flows)
					require.Zero(t, delivered)
					require.Zero(t, stats.BufferedBytes)
					require.Len(t, events, 1)
					e := events[0]
					require.Zero(t, e.FlowID)
					require.Zero(t, e.TCPFlowID)
					if complete {
						require.Equal(t, "l2tp", e.Protocol)
						require.Equal(t, "decoded", e.Status, e.Error)
						require.Equal(t, control, e.Raw)
						require.Equal(t, true, e.Session["Control"])
						require.Equal(t, "HELLO", e.Session["Packet Name"])
						require.Equal(t, uint64(0x12345678), e.Session["Control Connection ID"])
						require.Equal(t, uint64(0xfffe), e.Session["Ns"])
						require.Len(t, e.SourceBytes.PacketRefs, 2)
						require.NotNil(t, e.SourceBytes.CompletedIn)
					} else {
						require.Equal(t, "ip", e.Protocol)
						require.Equal(t, "FragmentFirstHeader", e.ExpertCode)
						require.Empty(t, e.Raw)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.Nil(t, e.SourceBytes.CompletedIn)
						_, err := e.GetFields()
						require.Error(t, err)
					}
				})
			}
		}
	}
}

func TestN14L2TPNonzeroSIDKeepsOpaqueContextContract(t *testing.T) {
	wire := []byte{1, 2, 3, 4, 0xde, 0xad, 0xbe, 0xef, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	whole := l2tp3NativeFrame(t, wire, false, true)
	frames := n14AuditSplitIPv6Frame(t, whole, 8)
	var records []n14AuditRecord
	for _, frame := range frames {
		records = append(records, n14AuditRecord{raw: frame})
	}
	var events []*ProtocolEvent
	var stats ProtocolStats
	require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "l2tp", e.Protocol)
	require.Equal(t, "decoded", e.Status, e.Error)
	require.Equal(t, "limited", e.Completeness)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, false, e.Session["Payload Decoded"])
	require.Contains(t, e.Session["Wire Completeness"], "cookie, sublayer and pseudowire require receiver context")
	require.Zero(t, e.FlowID)
	require.Zero(t, e.TCPFlowID)
	require.Zero(t, stats.BufferedBytes)
	// This positive only preserves the envelope/opaque contract. It does not
	// certify the complete data header or permit a missing observed cookie.
}
