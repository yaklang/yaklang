package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func n14AuditPPPoE(t *testing.T, frame []byte, owner byte, reverse bool, vlan uint16) []byte {
	t.Helper()
	require.GreaterOrEqual(t, len(frame), 14)
	src, dst := []byte{2, 0, 0, 0, owner, 1}, []byte{2, 0, 0, 0, owner, 2}
	if reverse {
		src, dst = dst, src
	}
	eth := append(bytes.Clone(dst), src...)
	eth = append(eth, 0x88, 0x64)
	body := frame[14:]
	proto := uint16(0x21)
	if body[0]>>4 == 6 {
		proto = 0x57
	}
	h := []byte{0x11, 0, 0, 30, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(h[4:6], uint16(2+len(body)))
	binary.BigEndian.PutUint16(h[6:8], proto)
	raw := append(append(eth, h...), body...)
	if vlan != 0 {
		raw = n14AuditVLAN(raw, vlan)
	}
	return raw
}

func TestN14PPPoEFragmentOwnerCannotJoinSameSIDAcrossMACPairs(t *testing.T) {
	for _, tunnel := range []bool{false, true} {
		for _, recover := range []bool{false, true} {
			for _, workers := range []int{1, 4} {
				t.Run(fmt.Sprintf("vxlan%t/recover%t/w%d", tunnel, recover, workers), func(t *testing.T) {
					frames := n14AuditFragments(t, "destination-after-fragment", false, false, 0x11223344, 0x5a4b)
					wrap := func(raw []byte, owner byte) []byte {
						raw = n14AuditPPPoE(t, raw, owner, false, 7)
						if tunnel {
							raw = vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(8, 0, 0, 42, 0), raw...), 55000, 4789, false)
						}
						return raw
					}
					records := []n14AuditRecord{{raw: wrap(frames[0], 1)}, {raw: wrap(frames[1], 2)}}
					if recover {
						records = append(records, n14AuditRecord{raw: wrap(frames[1], 1)}, n14AuditRecord{raw: wrap(frames[0], 2)})
					}
					var events []*ProtocolEvent
					var stats ProtocolStats
					require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
					require.Zero(t, stats.BufferedBytes)
					require.Len(t, events, 2)
					if !recover {
						require.Zero(t, stats.Messages)
						for _, e := range events {
							require.Equal(t, "ip", e.Protocol)
							require.Equal(t, "FragmentCaptureEnded", e.ExpertCode)
							require.Len(t, e.SourceBytes.PacketRefs, 1)
							require.Empty(t, e.Raw)
							require.Nil(t, e.SourceBytes.CompletedIn)
						}
						require.NotEqual(t, events[0].Domain, events[1].Domain)
					} else {
						require.EqualValues(t, 2, stats.Messages)
						domains := map[CaptureDomain]bool{}
						for _, e := range events {
							require.Equal(t, "dns", e.Protocol)
							require.Empty(t, e.Error)
							require.Len(t, e.SourceBytes.PacketRefs, 2)
							numbers := []uint64{e.SourceBytes.PacketRefs[0].Number, e.SourceBytes.PacketRefs[1].Number}
							if numbers[0] == 1 {
								require.Equal(t, []uint64{1, 3}, numbers)
							} else {
								require.Equal(t, []uint64{4, 2}, numbers)
							}
							domains[e.Domain] = true
						}
						require.Len(t, domains, 2)
					}
				})
			}
		}
	}
}

func TestN14PPPoEDomainUsesNearestEthernetAndCanonicalMACPair(t *testing.T) {
	frames := n14AuditFragments(t, "destination-after-fragment", false, false, 0x11223344, 0x5a4b)
	one := n14AuditPPPoE(t, frames[0], 1, false, 7)
	reverse := n14AuditPPPoE(t, frames[0], 1, true, 7)
	two := n14AuditPPPoE(t, frames[0], 2, false, 7)
	domain := func(raw []byte) CaptureDomain {
		return PacketDomain(raw, gopacket.CaptureInfo{}, layers.LinkTypeEthernet)
	}
	require.Equal(t, domain(one), domain(reverse), "both directions of one PPPoE session share its namespace")
	require.NotEqual(t, domain(one), domain(two), "SID is scoped by its Ethernet peers")
	outer := func(inner []byte) []byte {
		return vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(8, 0, 0, 42, 0), inner...), 55000, 4789, false)
	}
	vxOne, vxTwo := outer(one), outer(two)
	require.NotEqual(t, domain(vxOne), domain(vxTwo), "the enclosing VXLAN Ethernet pair cannot substitute for PPPoE's nearest Ethernet pair")
	changedOuter := bytes.Clone(vxOne)
	changedOuter[11] ^= 0x20
	require.Equal(t, domain(vxOne), domain(changedOuter), "changing the outer Ethernet alone cannot rename a fixed inner PPPoE session")
}

func TestN14PPPoETCPOwnerKeepsSameTupleAndSequenceInSeparateSessions(t *testing.T) {
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprintf("w%d", workers), func(t *testing.T) {
			prefix, body, _ := n14AuditTransport(t, true, 0)
			whole := append(prefix, body...)
			records := []n14AuditRecord{{raw: n14AuditPPPoE(t, whole, 1, false, 7)}, {raw: n14AuditPPPoE(t, whole, 2, false, 7)}}
			var events []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithTCPReassemblyWorkers(workers), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
			var http []*ProtocolEvent
			for _, e := range events {
				if e.Protocol == "http" {
					http = append(http, e)
				}
			}
			require.Len(t, http, 2)
			require.NotZero(t, http[0].FlowID)
			require.NotZero(t, http[1].FlowID)
			require.NotEqual(t, http[0].FlowID, http[1].FlowID)
			require.NotEqual(t, http[0].Domain, http[1].Domain)
			var sourceFrames []uint64
			for _, e := range http {
				require.Empty(t, e.Error)
				require.Equal(t, body[20:], e.Raw)
				require.Len(t, e.SourceBytes.PacketRefs, 1)
				sourceFrames = append(sourceFrames, e.SourceBytes.PacketRefs[0].Number)
			}
			require.ElementsMatch(t, []uint64{1, 2}, sourceFrames)
		})
	}
}
