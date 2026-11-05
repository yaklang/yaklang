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

func n14L2TPDataCarrier(t *testing.T, wire []byte, dir int) []byte {
	t.Helper()
	raw := l2tp3NativeFrame(t, wire, false, true)
	if dir == 1 {
		src, dst := bytes.Clone(raw[6:12]), bytes.Clone(raw[:6])
		copy(raw[:6], src)
		copy(raw[6:12], dst)
		src, dst = bytes.Clone(raw[22:38]), bytes.Clone(raw[38:54])
		copy(raw[22:38], dst)
		copy(raw[38:54], src)
	}
	return raw
}

func n14L2TPDataCapture(t *testing.T, cookieBytes, dir, split int, reverse bool, domain string) []n14AuditRecord {
	t.Helper()
	c0, c1 := bytes.Repeat([]byte{0x31}, cookieBytes), bytes.Repeat([]byte{0x62}, cookieBytes)
	setup := l2tp3TestSetup(false, 100, 200, 300, 400, c0, c1, 2)
	wrap := func(raw []byte, frameDir int, data bool) n14AuditRecord {
		record := n14AuditRecord{raw: raw}
		switch domain {
		case "VLAN":
			record.raw = n14AuditVLAN(raw, 7)
		case "PPPoE":
			record.raw = n14AuditPPPoE(t, raw, 1, frameDir == 1, 7)
		case "foreign-interface":
			if data {
				record.iface = 1
			}
		case "foreign-VLAN":
			vlan := uint16(7)
			if data {
				vlan = 8
			}
			record.raw = n14AuditVLAN(raw, vlan)
		case "foreign-PPPoE":
			owner := byte(1)
			if data {
				owner = 2
			}
			record.raw = n14AuditPPPoE(t, raw, owner, frameDir == 1, 7)
		case "foreign-peer":
			if data {
				record.raw = bytes.Clone(raw)
				record.raw[37] = 9
			}
		}
		return record
	}
	var records []n14AuditRecord
	for i, w := range setup {
		d := []int{0, 1, 0, 0, 1, 0}[i]
		records = append(records, wrap(n14L2TPDataCarrier(t, w, d), d, false))
	}
	sid, cookie := uint32(400), c1
	if dir == 1 {
		sid, cookie = 300, c0
	}
	wire := l2tp3TestData(false, sid, cookie, 7)
	dns := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1}
	inner := l2tp3TestEthernet(t, dns, layers.IPProtocolUDP, "198.51.100.1", "198.51.100.2", 41000, 53, false)
	wire = append(bytes.Clone(wire[:4+cookieBytes+4]), inner...)
	frames := n14AuditSplitIPv6Frame(t, n14L2TPDataCarrier(t, wire, dir), split)
	// Distinct fragment IDs keep future controls in this capture independent.
	for _, raw := range frames {
		binary.BigEndian.PutUint32(raw[58:62], 0x715a4321)
	}
	if reverse {
		frames[0], frames[1] = frames[1], frames[0]
	}
	for _, raw := range frames {
		records = append(records, wrap(raw, dir, true))
	}
	return records
}

func TestN14L2TPPrivateReceiverDataHeaderFitsFirstFragment(t *testing.T) {
	for _, cookieBytes := range []int{4, 8} {
		for _, dir := range []int{0, 1} {
			for _, domain := range []string{"plain", "VLAN", "PPPoE"} {
				for _, complete := range []bool{false, true} {
					for _, reverse := range []bool{false, true} {
						t.Run(fmt.Sprintf("cookie%d/dir%d/%s/complete%t/reverse%t", cookieBytes, dir, domain, complete, reverse), func(t *testing.T) {
							split := 8
							if complete {
								split = 16
							}
							records := n14L2TPDataCapture(t, cookieBytes, dir, split, reverse, domain)
							var events []*ProtocolEvent
							var stats ProtocolStats
							var conf *CaptureConfig
							calls, sequenceCommitted := 0, false
							require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)),
								func(c *CaptureConfig) error { conf = c; return nil },
								WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }),
								WithOnProtocolStats(func(s ProtocolStats) { stats = s }),
								WithEveryPacket(func(gopacket.Packet) {
									calls++
									if calls != len(records) || complete {
										return
									}
									p := conf.binParser
									require.Nil(t, p.udpSessions, "a refused known L2TP header cannot allocate an inner UDP owner")
									p.l2tp3Mu.Lock()
									defer p.l2tp3Mu.Unlock()
									if p.l2tp3 != nil {
										for _, bindings := range p.l2tp3.bindings {
											for _, binding := range bindings {
												v := binding.session
												for _, sequence := range v.sequence {
													sequenceCommitted = sequenceCommitted || sequence.known || len(sequence.seen) > 0
												}
											}
										}
									}
								}),
							))
							require.Zero(t, stats.BufferedBytes)
							require.Equal(t, len(records), calls)
							require.GreaterOrEqual(t, len(events), 7)
							for _, e := range events[:6] {
								require.Equal(t, "l2tp", e.Protocol)
								require.Equal(t, "decoded", e.Status, e.Error)
							}
							last := events[6:]
							if complete {
								require.Len(t, last, 2, "one admitted L2TP envelope and its DNS message")
								require.Equal(t, "l2tp", last[0].Protocol)
								require.Equal(t, "decoded", last[0].Status, last[0].Error)
								require.Equal(t, true, last[0].Session["Payload Decoded"])
								require.Equal(t, "dns", last[1].Protocol)
								require.Equal(t, "decoded", last[1].Status, last[1].Error)
							} else {
								require.Len(t, last, 1, "known split cookie/sublayer cannot produce an inner observation")
								e := last[0]
								require.Equal(t, "l2tp", e.Protocol)
								require.Equal(t, "malformed", e.Status, "full private owner match must not legalize its split data header")
								require.Equal(t, string(ErrMalformedMessage), e.ExpertCode)
								require.Contains(t, e.Error, "first fragment")
								require.Equal(t, 8, e.Session["IPv6 First Fragment Payload Bytes"])
								require.Equal(t, 4+cookieBytes+4, e.Session["Required L2TP Data Header Bytes"])
								require.Equal(t, false, e.Session["Payload Decoded"])
								require.Zero(t, e.FlowID)
								require.Zero(t, e.TCPFlowID)
								require.Len(t, e.SourceBytes.PacketRefs, 2)
								require.Nil(t, e.SourceBytes.CompletedIn)
								require.False(t, sequenceCommitted, "refusal precedes sequence high/seen mutation")
								_, err := e.GetFields()
								require.Error(t, err)
							}
						})
					}
				}
			}
		}
	}
}

func TestN14L2TPDataHeaderCannotBorrowReceiverFromForeignDomainOrPeer(t *testing.T) {
	for _, domain := range []string{"foreign-section", "foreign-interface", "foreign-VLAN", "foreign-PPPoE", "foreign-peer"} {
		t.Run(domain, func(t *testing.T) {
			records := n14L2TPDataCapture(t, 8, 0, 8, false, domain)
			var events []*ProtocolEvent
			var stats ProtocolStats
			capture := n14AuditCapture(t, records)
			if domain == "foreign-section" {
				capture = append(n14AuditCapture(t, records[:6]), n14AuditCapture(t, records[6:])...)
			}
			require.NoError(t, ReplayPcap(bytes.NewReader(capture), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Len(t, events, 7)
			e := events[6]
			require.Equal(t, "l2tp", e.Protocol)
			if domain == "foreign-peer" {
				require.Equal(t, "context-required", e.Status, e.Error)
				require.Equal(t, "context-required", e.Completeness)
			} else {
				require.Equal(t, "decoded", e.Status, e.Error)
				require.Equal(t, "limited", e.Completeness)
			}
			require.Equal(t, false, e.Session["Payload Decoded"])
			require.NotContains(t, e.Session, "Required L2TP Data Header Bytes", "foreign bindings cannot supply header extent")
			require.Zero(t, e.FlowID)
			require.Zero(t, e.TCPFlowID)
			require.Zero(t, stats.BufferedBytes)
		})
	}
}

func TestN14L2TPFirstFragmentLossAndDuplicateHaveNoPrematureOwner(t *testing.T) {
	for _, mode := range []string{"duplicate-complete", "duplicate-split-cookie", "missing-first"} {
		t.Run(mode, func(t *testing.T) {
			split := 16
			if mode != "duplicate-complete" {
				split = 8
			}
			records := n14L2TPDataCapture(t, 8, 0, split, false, "PPPoE")
			if mode == "missing-first" {
				records = append(records[:6], records[7])
			} else {
				records = append(append(append([]n14AuditRecord(nil), records[:7]...), records[6]), records[7])
			}
			var events []*ProtocolEvent
			var stats ProtocolStats
			require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Zero(t, stats.BufferedBytes)
			if mode == "duplicate-complete" {
				require.Len(t, events, 8)
				require.Equal(t, "l2tp", events[6].Protocol)
				require.Equal(t, "dns", events[7].Protocol)
				require.Len(t, events[6].SourceBytes.PacketRefs, 2, "an exact duplicate is not another byte contributor")
			} else {
				require.Len(t, events, 7)
				e := events[6]
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
				require.Nil(t, e.SourceBytes.CompletedIn)
				if mode == "missing-first" {
					require.Equal(t, "ip", e.Protocol)
					require.Equal(t, "FragmentCaptureEnded", e.ExpertCode)
					require.Empty(t, e.Raw)
				} else {
					require.Equal(t, "l2tp", e.Protocol)
					require.Equal(t, "malformed", e.Status)
					require.Contains(t, e.Error, "first fragment")
					require.Len(t, e.SourceBytes.PacketRefs, 2)
				}
			}
		})
	}
}

func n14L2TPSetupSelected(t *testing.T, cid0, cid1 uint32, cookie0, cookie1 []byte, sublayer, sequencing uint16) [][]byte {
	t.Helper()
	messages := l2tp3TestSetup(false, cid0, cid1, 300, 400, cookie0, cookie1, sequencing)
	for _, wire := range messages {
		for at := 16; at < len(wire); {
			require.LessOrEqual(t, at+6, len(wire))
			n := int(binary.BigEndian.Uint16(wire[at:]) & 1023)
			require.GreaterOrEqual(t, n, 6)
			require.LessOrEqual(t, at+n, len(wire))
			if binary.BigEndian.Uint16(wire[at+4:]) == 69 {
				require.Equal(t, 8, n)
				binary.BigEndian.PutUint16(wire[at+6:at+8], sublayer)
			}
			at += n
		}
	}
	return messages
}

func n14L2TPDataWire(sid uint32, cookie []byte, sublayer bool, body []byte) []byte {
	wire := binary.BigEndian.AppendUint32(nil, sid)
	wire = append(wire, cookie...)
	if sublayer {
		wire = append(wire, 0x40, 0, 0, 7)
	}
	return append(wire, body...)
}

func TestN14L2TPSameSIDNeedsUniqueCompleteCookieBeforeHeaderExtent(t *testing.T) {
	for _, mode := range []string{"short-owner-complete", "long-owner-split", "unknown-cookie", "ambiguous-cookie"} {
		t.Run(mode, func(t *testing.T) {
			shortCookie := []byte{1, 2, 3, 4}
			longCookie := []byte{5, 6, 7, 8, 9, 10, 11, 12}
			sublayer := false
			if mode == "ambiguous-cookie" {
				longCookie = append(bytes.Clone(shortCookie), 0x40, 0, 0, 7)
				sublayer = true
			}
			var records []n14AuditRecord
			for generation, cookies := range [][2][]byte{{shortCookie, shortCookie}, {longCookie, longCookie}} {
				sub, seq := uint16(0), uint16(0)
				if generation == 1 || mode == "ambiguous-cookie" {
					sub, seq = 1, 2
				}
				messages := n14L2TPSetupSelected(t, 100+uint32(generation), 200+uint32(generation), cookies[0], cookies[1], sub, seq)
				for i, w := range messages {
					dir := []int{0, 1, 0, 0, 1, 0}[i]
					records = append(records, n14AuditRecord{raw: n14L2TPDataCarrier(t, w, dir)})
				}
			}
			dns := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1}
			inner := l2tp3TestEthernet(t, dns, layers.IPProtocolUDP, "198.51.100.1", "198.51.100.2", 41000, 53, false)
			cookie := shortCookie
			if mode == "long-owner-split" {
				cookie = longCookie
				sublayer = true
			}
			if mode == "unknown-cookie" {
				cookie = bytes.Repeat([]byte{0xee}, 8)
				sublayer = true
			}
			wire := n14L2TPDataWire(400, cookie, sublayer, inner)
			for _, raw := range n14AuditSplitIPv6Frame(t, n14L2TPDataCarrier(t, wire, 0), 8) {
				records = append(records, n14AuditRecord{raw: raw})
			}
			var events []*ProtocolEvent
			var stats ProtocolStats
			require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Zero(t, stats.BufferedBytes)
			require.GreaterOrEqual(t, len(events), 13)
			last := events[12:]
			if mode == "short-owner-complete" {
				require.Len(t, last, 2)
				require.Equal(t, "decoded", last[0].Status, last[0].Error)
				require.Equal(t, true, last[0].Session["Payload Decoded"])
				require.Equal(t, "dns", last[1].Protocol)
			} else {
				require.Len(t, last, 1)
				e := last[0]
				require.Equal(t, "l2tp", e.Protocol)
				require.Equal(t, false, e.Session["Payload Decoded"])
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
				if mode == "long-owner-split" {
					require.Equal(t, "malformed", e.Status)
					require.Equal(t, 16, e.Session["Required L2TP Data Header Bytes"])
				} else {
					require.Equal(t, "context-required", e.Status, e.Error)
					require.NotContains(t, e.Session, "Required L2TP Data Header Bytes")
				}
			}
		})
	}
}

func TestN14L2TPOuterFirstFragmentWitnessDoesNotConstrainUnfragmentedInner(t *testing.T) {
	cookie := []byte{1, 2, 3, 4}
	var records []n14AuditRecord
	add := func(w []byte, dir int) { records = append(records, n14AuditRecord{raw: n14L2TPDataCarrier(t, w, dir)}) }
	for i, w := range n14L2TPSetupSelected(t, 100, 200, cookie, cookie, 0, 0) {
		add(w, []int{0, 1, 0, 0, 1, 0}[i])
	}
	// Inner receiver has its own namespace and a larger sixteen-byte data
	// header. Setup reaches it through the already admitted outer L2TP owner.
	innerCookie := bytes.Repeat([]byte{0x55}, 8)
	for i, w := range n14L2TPSetupSelected(t, 101, 201, innerCookie, innerCookie, 1, 2) {
		dir := []int{0, 1, 0, 0, 1, 0}[i]
		add(n14L2TPDataWire(400, cookie, false, n14L2TPDataCarrier(t, w, dir)), 0)
	}
	dns := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1}
	inner := l2tp3TestEthernet(t, dns, layers.IPProtocolUDP, "198.51.100.1", "198.51.100.2", 41000, 53, false)
	middle := n14L2TPDataCarrier(t, n14L2TPDataWire(400, innerCookie, true, inner), 0)
	outer := n14L2TPDataCarrier(t, n14L2TPDataWire(400, cookie, false, middle), 0)
	for _, raw := range n14AuditSplitIPv6Frame(t, outer, 8) {
		records = append(records, n14AuditRecord{raw: raw})
	}
	var events []*ProtocolEvent
	var stats ProtocolStats
	require.NoError(t, ReplayPcap(bytes.NewReader(n14AuditCapture(t, records)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
	require.Zero(t, stats.BufferedBytes)
	require.NotEmpty(t, events)
	require.Equal(t, "dns", events[len(events)-1].Protocol, "outer firstPayload8 applies to outer header8 only; inner unfragmented header16 has no such constraint")
	require.Equal(t, "decoded", events[len(events)-1].Status, events[len(events)-1].Error)
	var selected []*ProtocolEvent
	for _, e := range events {
		if e.Protocol == "l2tp" && e.Session["Payload Decoded"] == true {
			selected = append(selected, e)
		}
	}
	require.GreaterOrEqual(t, len(selected), 8)
	outerData, innerData := selected[len(selected)-2], selected[len(selected)-1]
	require.NotEqual(t, outerData.Domain, innerData.Domain)
	require.NotEqual(t, outerData.FlowID, innerData.FlowID)
	require.Equal(t, true, innerData.Session["Cookie Matches Proposal"])
}
