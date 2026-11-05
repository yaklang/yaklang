package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
)

// Inserts an IPv6 Fragment header directly after an authored fixed IPv6 header.
// The original Next Header becomes the Fragment Next Header; no decoder result
// is used to derive an expected answer.
func n14AuditSplitIPv6Frame(t *testing.T, whole []byte, split int) [][]byte {
	t.Helper()
	require.GreaterOrEqual(t, len(whole), 54)
	body := whole[54:]
	require.Positive(t, split)
	require.Zero(t, split%8)
	require.Greater(t, len(body), split)
	var frames [][]byte
	for off := 0; off < len(body); {
		end := len(body)
		if off == 0 {
			end = split
		}
		raw := bytes.Clone(whole[:54])
		next := raw[20]
		raw[20] = 44
		binary.BigEndian.PutUint16(raw[18:20], uint16(8+end-off))
		h := []byte{next, 0, 0, 0, 0x11, 0x22, 0x33, 0x44}
		bits := uint16(off)
		if end < len(body) {
			bits |= 1
		}
		binary.BigEndian.PutUint16(h[2:4], bits)
		raw = append(raw, h...)
		frames = append(frames, append(raw, body[off:end]...))
		off = end
	}
	return frames
}

func n14AuditGREIPv6(t *testing.T, kind string) ([]byte, int) {
	t.Helper()
	inner := pptpV1TestInner(t, false, 0, false, nil)
	var gre []byte
	headerBytes := 0
	switch kind {
	case "key-sequence":
		gre = []byte{0x30, 0, 0x08, 0, 0, 0, 0, 7, 0, 0, 0, 9}
		headerBytes = 12
	case "checksum-key-sequence":
		gre = []byte{0xb0, 0, 0x08, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 9}
		headerBytes = 16
	case "routing-key-sequence":
		gre = []byte{0x70, 0, 0x08, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 9, 0x08, 0, 0, 4, 192, 0, 2, 99, 0, 0, 0, 0}
		headerBytes = 28
	case "pptp-sequence":
		ppp := append([]byte{0, 0x21}, inner...)
		gre = pptpV1TestGRE(0x3001, 0x880b, uint16(len(ppp)), 42, 9, 0, ppp)
		inner = nil
		headerBytes = 12
	case "pptp-sequence-ack":
		ppp := append([]byte{0, 0x21}, inner...)
		gre = pptpV1TestGRE(0x3081, 0x880b, uint16(len(ppp)), 42, 9, 11, ppp)
		inner = nil
		headerBytes = 16
	case "unselected-version":
		gre = []byte{0, 2, 0x08, 0}
		headerBytes = 4
	default:
		t.Fatalf("unexpected GRE case %q", kind)
	}
	gre = append(gre, inner...)
	if kind == "checksum-key-sequence" {
		var sum uint32
		for i := 0; i < len(gre); i += 2 {
			sum += uint32(gre[i]) << 8
			if i+1 < len(gre) {
				sum += uint32(gre[i+1])
			}
		}
		for sum>>16 != 0 {
			sum = (sum & 65535) + (sum >> 16)
		}
		binary.BigEndian.PutUint16(gre[4:6], ^uint16(sum))
	}
	prefix, _, _ := n14AuditTransport(t, false, 0x5a4b)
	prefix[20] = 47
	binary.BigEndian.PutUint16(prefix[18:20], uint16(len(gre)))
	return append(prefix, gre...), headerBytes
}

func TestN14FragmentSelectedUpperHeaderCaptureIngress(t *testing.T) {
	ospf := ospfv3NativeHello(0, 2)
	ospfDestination := ospfv3NativeFrame(ospf, 60, []byte{89, 0, 1, 4, 0, 0, 0, 0})
	rsvp := rsvpNativeFrame(t, rsvpNativeWire(7), true)
	rsvpDestination := append(bytes.Clone(rsvp[:54]), append([]byte{46, 0, 1, 4, 0, 0, 0, 0}, rsvp[54:]...)...)
	rsvpDestination[20] = 60
	binary.BigEndian.PutUint16(rsvpDestination[18:20], uint16(len(rsvpDestination)-54))
	cases := []struct {
		name                string
		whole               []byte
		badSplit, goodSplit int
		protocol            string
	}{
		{"ospfv3-fixed16", ospfv3NativeFrame(ospf, 89, nil), 8, 16, "ospf"},
		{"destination-ospfv3-fixed16", ospfDestination, 16, 24, "ospf"},
		{"destination-rsvp-fixed8", rsvpDestination, 8, 16, "rsvp"},
	}
	for _, kind := range []string{"key-sequence", "checksum-key-sequence", "routing-key-sequence", "pptp-sequence", "pptp-sequence-ack"} {
		whole, n := n14AuditGREIPv6(t, kind)
		bad := 8
		if n > 16 {
			bad = 16
		}
		cases = append(cases, struct {
			name                string
			whole               []byte
			badSplit, goodSplit int
			protocol            string
		}{"gre-" + kind, whole, bad, (n + 7) / 8 * 8, "dns"})
	}
	for _, c := range cases {
		for _, accepted := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				for _, workers := range []int{1, 4} {
					t.Run(fmt.Sprintf("%s/accepted%t/reverse%t/w%d", c.name, accepted, reverse, workers), func(t *testing.T) {
						split := c.badSplit
						if accepted {
							split = c.goodSplit
						}
						frames := n14AuditSplitIPv6Frame(t, c.whole, split)
						if reverse {
							frames[0], frames[1] = frames[1], frames[0]
						}
						var records []n14AuditRecord
						for _, raw := range frames {
							records = append(records, n14AuditRecord{raw: raw})
						}
						var events []*ProtocolEvent
						var stats ProtocolStats
						require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
						require.Zero(t, stats.BufferedBytes)
						require.Len(t, events, 1)
						e := events[0]
						if accepted {
							require.Equal(t, c.protocol, e.Protocol)
							require.Equal(t, "decoded", e.Status, e.Error)
							require.NotEmpty(t, e.Raw)
							require.NotNil(t, e.SourceBytes.CompletedIn)
							require.Len(t, e.SourceBytes.PacketRefs, 2)
						} else {
							require.Equal(t, "ip", e.Protocol)
							require.Equal(t, "FragmentFirstHeader", e.ExpertCode)
							require.Zero(t, e.FlowID)
							require.Zero(t, e.TCPFlowID)
							require.Empty(t, e.Raw)
							require.Nil(t, e.SourceBytes.CompletedIn)
							require.Len(t, e.SourceBytes.PacketRefs, 1)
							want := uint64(1)
							if reverse {
								want = 2
							}
							require.EqualValues(t, want, e.SourceBytes.PacketRefs[0].Number)
						}
					})
				}
			}
		}
	}
}

func TestN14FragmentUnknownUpperAndGREProfilePreserveAdmission(t *testing.T) {
	whole, _ := n14AuditGREIPv6(t, "unselected-version")
	frames := n14AuditSplitIPv6Frame(t, whole, 8)
	var records []n14AuditRecord
	for _, raw := range frames {
		records = append(records, n14AuditRecord{raw: raw})
	}
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	require.Equal(t, "EncapsulationDepthOrProfile", events[0].ExpertCode)
	require.Equal(t, "limited", events[0].Status)
	require.Len(t, events[0].SourceBytes.PacketRefs, 2)
	require.True(t, completeFragmentFirstHeader(253, []byte{}), "an unknown upper protocol must keep the existing unknown policy")
}

// PIM's common header is four bytes. Its type-specific flags/body are not
// promoted into this upper-header requirement. The native PIM profile with a
// Destination Options predecessor remains unselected after complete assembly.
func TestN14FragmentPIMCommonHeaderAfterDestination(t *testing.T) {
	hello := []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105, 0, 19, 0, 4, 0, 0, 0, 7, 0, 20, 0, 4, 1, 2, 3, 4}
	direct := pimControlFrame(t, hello, true)
	whole := append(bytes.Clone(direct[:54]), append([]byte{103, 0, 1, 4, 0, 0, 0, 0}, direct[54:]...)...)
	whole[20] = 60
	binary.BigEndian.PutUint16(whole[18:20], uint16(len(whole)-54))
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprintf("accepted%t", accepted), func(t *testing.T) {
			split := 8
			if accepted {
				split = 16
			}
			frames := n14AuditSplitIPv6Frame(t, whole, split)
			var records []n14AuditRecord
			for _, raw := range frames {
				records = append(records, n14AuditRecord{raw: raw})
			}
			var events []*ProtocolEvent
			var stats TCPReassemblyStats
			transportCalls := 0
			err := ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithBeforeTransportPacket(func(p gopacket.Packet) { transportCalls++; require.Equal(t, whole[14:], p.Data()) }))
			if accepted {
				require.EqualError(t, err, "Unable to decode IPProtocol 103", "the existing extension-carried PIM profile remains unsupported")
				require.EqualValues(t, 1, stats.DecodeErrors)
				require.Empty(t, events, "a complete upper header does not select an unsupported PIM profile")
				require.Equal(t, 1, transportCalls)
			} else {
				require.NoError(t, err)
				require.Zero(t, stats.DecodeErrors)
				require.Len(t, events, 1)
				require.Equal(t, "FragmentFirstHeader", events[0].ExpertCode)
				require.Zero(t, transportCalls)
			}
		})
	}
}
