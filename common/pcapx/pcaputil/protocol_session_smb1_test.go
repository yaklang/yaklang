package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Authored from the CIFS field layout, without importing a capture or a
// dissector's output. The two SMB2 strings are offers, not a selected version.
func smb1UpgradeOffer(names ...string) []byte {
	h := make([]byte, 35)
	copy(h, []byte{0xff, 'S', 'M', 'B', 0x72})
	h[9] = 0x18
	binary.LittleEndian.PutUint16(h[10:12], 0xc853)
	binary.LittleEndian.PutUint16(h[30:32], 7)
	for _, name := range names {
		h = append(h, 2)
		h = append(h, []byte(name)...)
		h = append(h, 0)
	}
	binary.LittleEndian.PutUint16(h[33:35], uint16(len(h)-35))
	return smb2TCP(h)
}

func TestSMB1NegotiateAdmissionNeedsCompleteDialectList(t *testing.T) {
	w := smb1UpgradeOffer("NT LM 0.12", "SMB 2.002", "SMB 2.???")
	require.Len(t, w, 73)
	for i := 8; i < len(w); i++ {
		require.Equal(t, ProbeNeedMore, probeSMB1Negotiate(w[:i], 65574).Verdict, "prefix %d", i)
	}
	require.Equal(t, ProbeAccept, probeSMB1Negotiate(w, 65574).Verdict)
	require.Equal(t, ProbeAccept, probeSMB1Negotiate(append(bytes.Clone(w), smb2NegotiateReq(0x0311)...), 65574).Verdict, "only the first framed record is admitted")
	for _, tc := range []struct {
		name string
		edit func([]byte)
	}{
		{"signature", func(w []byte) { w[6] ^= 1 }},
		{"command", func(w []byte) { w[8] = 0x73 }},
		{"response", func(w []byte) { w[13] |= 0x80 }},
		{"word-count", func(w []byte) { w[36] = 1 }},
		{"byte-count", func(w []byte) { w[37]-- }},
		{"marker", func(w []byte) { w[39] = 3 }},
		{"missing-final-nul", func(w []byte) { w[len(w)-1] = 'x' }},
		{"embedded-marker", func(w []byte) { w[51] = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := bytes.Clone(w)
			tc.edit(bad)
			require.Equal(t, ProbeReject, probeSMB1Negotiate(bad, 65574).Verdict)
		})
	}
	require.Equal(t, ProbeReject, probeSMB1Negotiate(w, len(w)-1).Verdict)
	for _, names := range [][]string{{""}, {"OEM-\x80\xfe", ""}, {"SMB 2.002", "SMB 2.002"}} {
		require.Equal(t, ProbeAccept, probeSMB1Negotiate(smb1UpgradeOffer(names...), 65574).Verdict)
	}
}

func TestProtocolSessionSMB1UpgradePreservesFramingAndPrivateFields(t *testing.T) {
	for _, chunk := range []int{1, 7, 8, 38, 64, 73, 4096} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			t.Cleanup(func() { s.Close("FIN") })
			offer := smb1UpgradeOffer("NT LM 0.12", "SMB 2.002", "SMB 2.???")
			require.Equal(t, "smb", s.Probe(offer).Protocol)
			require.Equal(t, ProbeAccept, s.Probe(offer).Verdict)
			combined := append(bytes.Clone(offer), smb2NegotiateReq(0x0202, 0x0311)...)
			var events []*ProtocolEvent
			for at := 0; at < len(combined); at += chunk {
				end := min(at+chunk, len(combined))
				r := s.Feed(0, time.Unix(1, 0), combined[at:end])
				require.Equal(t, end-at, r.Consumed)
				if end == len(combined) {
					require.Nil(t, r.Err, "%v", r.Err)
				} else if r.Err != nil {
					require.Equal(t, ErrNeedMore, r.Err.Kind)
					require.True(t, r.NeedMore)
				}
				events = append(events, r.Events...)
			}
			require.Len(t, events, 2)
			legacy, modern := events[0], events[1]
			require.Equal(t, "smb", legacy.Protocol)
			require.Equal(t, "CIFSDirectTCP", legacy.Entry)
			require.Equal(t, offer, legacy.Raw)
			require.Equal(t, "cifs-negotiate-request", legacy.Profile)
			require.Equal(t, true, legacy.Session["SMB2 Upgrade Offered"])
			require.Equal(t, false, legacy.Session["Dialect Negotiated"])
			require.NotContains(t, legacy.Session, "Dialect")
			require.NotContains(t, legacy.Session, "Message ID")
			require.Zero(t, legacy.ResponseTo)
			fields, err := legacy.GetFields()
			require.NoError(t, err)
			require.Len(t, fields["Dialects"], 3)
			// Public DTO mutations must not change the next private decode or
			// an already validated transition and its independent SMB2 MID.
			fields["Header"].(map[string]any)["Command"] = uint8(0)
			legacy.Session["Byte Count"] = uint16(0)
			fresh, err := legacy.GetFields()
			require.NoError(t, err)
			require.Equal(t, uint8(0x72), fresh["Header"].(map[string]any)["Command"])
			require.Equal(t, uint16(34), fresh["Byte Count"])
			require.Equal(t, "smb2", modern.Protocol)
			require.Equal(t, uint64(1), modern.Session["Message ID"])
			require.Equal(t, []uint16{0x0202, 0x0311}, modern.Session["Dialects"])
			require.Len(t, s.(*captureSession).f.smb2.pending, 1)
		})
	}
}

func TestProtocolSessionSMB1WildcardIsNotSelectedDialect(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, smb1UpgradeOffer("SMB 2.???")).Err)
	wildcard := smb2NegotiateResp(0x02ff)
	binary.LittleEndian.PutUint64(wildcard[28:36], 0)
	r := s.Feed(1, ts, wildcard)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, uint16(0x02ff), r.Events[0].Session["Dialect"])
	require.Equal(t, true, r.Events[0].Session["SMB2 Negotiate Required"])
	require.Equal(t, false, r.Events[0].Session["Dialect Negotiated"])
	require.Equal(t, true, r.Events[0].Session["Unmatched"], "an SMB1 MID is never installed as an SMB2 pending request")
	require.Zero(t, s.(*captureSession).f.smb2.dialect)
	r = s.Feed(0, ts, smb2NegotiateReq(0x0311))
	require.Nil(t, r.Err)
	require.NotContains(t, r.Events[0].Session, "Dialect", "wildcard must not stick as a selected version")
	r = s.Feed(1, ts, smb2NegotiateResp(0x0311))
	require.Nil(t, r.Err)
	require.Equal(t, uint16(0x0311), r.Events[0].Session["Dialect"])
	require.Equal(t, "NEGOTIATE", r.Events[0].Session["Matched Request"])
}

func TestSMB1UpgradeReorderingLossRetransmitAndTupleReuse(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, workers := range []int{1, 2} {
			for _, loss := range []bool{false, true} {
				t.Run(fmt.Sprintf("defer%v/workers%d/loss%v", deferred, workers, loss), func(t *testing.T) {
					offer := smb1UpgradeOffer("NT LM 0.12", "SMB 2.002", "SMB 2.???")
					req := smb2NegotiateReq(0x0311)
					steps := []tcpStep{
						{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500},
						{seq: 100, data: string(offer[:64])}, {seq: 164, data: string(offer[64:])},
					}
					base := uint32(100 + len(offer))
					steps = append(steps, tcpStep{seq: base + 32, data: string(req[32:])}, tcpStep{seq: base, data: string(req[:16])})
					if !loss {
						steps = append(steps, tcpStep{seq: base + 16, data: string(req[16:32])})
					}
					steps = append(steps, tcpStep{seq: base + 32, data: string(req[32:])})
					second := uint64(len(steps) + 1)
					steps = append(steps, tcpStep{seq: 9999, syn: true}, tcpStep{seq: 19999, ack: 10000, syn: true, synack: true, reverse: true}, tcpStep{seq: 10000, ack: 20000},
						tcpStep{seq: 10000, data: string(req)}, tcpStep{seq: 20000, data: string(smb2NegotiateResp(0x0202)), reverse: true})
					var mu sync.Mutex
					var events []*ProtocolEvent
					err := ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 50445, false, true)), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }))
					if loss {
						require.ErrorContains(t, err, "unfilled sequence gap")
					} else {
						require.NoError(t, err)
					}
					var legacyFlow, modernFlow, reusedFlow uint64
					legacyCount, modernCount, reusedCount := 0, 0, 0
					for _, e := range events {
						if e.Status != "decoded" && e.Status != "deferred" {
							continue
						}
						if e.Protocol == "smb" {
							legacyCount++
							legacyFlow = e.TCPFlowID
							fields, fieldErr := e.GetFields()
							require.NoError(t, fieldErr)
							require.Len(t, fields["Dialects"], 3)
						}
						if e.Protocol != "smb2" {
							continue
						}
						if e.SourceBytes.CompletedIn.Number < second {
							modernCount++
							modernFlow = e.TCPFlowID
							require.Equal(t, req, e.Raw)
							require.Len(t, e.SourceBytes.PacketRefs, 3, "a retransmitted suffix does not own a new occurrence")
						} else {
							reusedCount++
							reusedFlow = e.TCPFlowID
							if e.Session["Packet Name"] == "NEGOTIATE" && e.Session["Response"] == false {
								require.NotContains(t, e.Session, "Dialect")
							}
						}
					}
					require.Equal(t, 1, legacyCount)
					require.Equal(t, 2, reusedCount)
					require.NotEqual(t, legacyFlow, reusedFlow)
					if loss {
						require.Zero(t, modernCount)
					} else {
						require.Equal(t, 1, modernCount)
						require.Equal(t, legacyFlow, modernFlow)
					}
				})
			}
		}
	}
}
