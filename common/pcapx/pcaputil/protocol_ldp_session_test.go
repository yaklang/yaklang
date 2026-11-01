package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func ldpTestTLV(typ uint16, w []byte) []byte {
	return append(binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(nil, typ), uint16(len(w))), w...)
}
func ldpTestMessage(typ uint16, id uint32, w []byte) []byte {
	b := binary.BigEndian.AppendUint16(nil, typ)
	b = binary.BigEndian.AppendUint16(b, uint16(len(w)+4))
	b = binary.BigEndian.AppendUint32(b, id)
	return append(b, w...)
}
func ldpTestPDU(sender byte, messages ...[]byte) []byte {
	w := []byte{0, 1, 0, 0, 192, 0, 2, sender, 0, 0}
	for _, m := range messages {
		w = append(w, m...)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)-4))
	return w
}
func ldpTestInit(sender, receiver byte, keep, maximum uint16) []byte {
	w := []byte{0, 1}
	w = binary.BigEndian.AppendUint16(w, keep)
	w = append(w, 0, 0)
	w = binary.BigEndian.AppendUint16(w, maximum)
	w = append(w, 192, 0, 2, receiver, 0, 0)
	return ldpTestPDU(sender, ldpTestMessage(0x200, 1, ldpTestTLV(0x500, w)))
}
func ldpTestKA(sender byte) []byte { return ldpTestPDU(sender, ldpTestMessage(0x201, 2, nil)) }
func TestLDPSessionEverySplitAndObservedLimits(t *testing.T) {
	w := ldpTestInit(1, 2, 90, 5000)
	for cut := 1; cut < len(w); cut++ {
		s := observedTestSession(t, 646)
		require.Empty(t, s.Feed(0, time.Unix(1, 0), w[:cut]).Events)
		r := s.Feed(0, time.Unix(2, 0), w[cut:])
		require.Len(t, r.Events, 1)
		require.Empty(t, r.Events[0].Error)
		r = s.Feed(1, time.Unix(3, 0), ldpTestInit(2, 1, 60, 6000))
		require.Len(t, r.Events, 1)
		require.Empty(t, r.Events[0].Error)
		require.NotContains(t, r.Events[0].Session, "Observed Maximum PDU Length", "two proposals alone do not prove completed exchange")
		s.Feed(1, time.Unix(4, 0), ldpTestKA(2))
		r = s.Feed(0, time.Unix(5, 0), ldpTestKA(1))
		require.Len(t, r.Events, 1)
		require.Equal(t, uint64(5000), r.Events[0].Session["Observed Maximum PDU Length"])
		require.Equal(t, uint64(60), r.Events[0].Session["Observed KeepAlive Time"])
		values, err := r.Events[0].GetFields()
		require.NoError(t, err)
		r.Events[0].Session["Observed Maximum PDU Length"] = uint64(9999)
		copyFields, err := r.Events[0].GetFields()
		require.NoError(t, err)
		require.Equal(t, values, copyFields)
	}
}
func TestLDPSessionRepeatedInitFatalAndIdentifierReuseStayIsolated(t *testing.T) {
	s := observedTestSession(t, 646)
	s.Feed(0, time.Unix(1, 0), ldpTestInit(1, 2, 90, 0))
	r := s.Feed(0, time.Unix(2, 0), ldpTestInit(1, 2, 90, 0))
	require.Len(t, r.Events, 1)
	require.Equal(t, "context-required", r.Events[0].Status)
	r = s.Feed(1, time.Unix(3, 0), ldpTestInit(2, 1, 60, 0))
	require.Equal(t, "context-required", r.Events[0].Status)
	require.NotContains(t, r.Events[0].Session, "Observed Maximum PDU Length")
	fresh := observedTestSession(t, 646)
	r = fresh.Feed(0, time.Unix(4, 0), ldpTestInit(1, 2, 90, 0))
	require.Empty(t, r.Events[0].Error)
	r = fresh.Feed(1, time.Unix(5, 0), ldpTestInit(2, 99, 90, 0))
	require.Equal(t, "context-required", r.Events[0].Status)
	third := observedTestSession(t, 646)
	third.Feed(0, time.Unix(6, 0), ldpTestKA(1))
	status := ldpTestTLV(0x300, []byte{0x80, 0, 0, 9, 0, 0, 0, 0, 0, 0})
	msg := ldpTestMessage(1, 3, status)
	r = third.Feed(0, time.Unix(7, 0), ldpTestPDU(1, msg, msg))
	require.Len(t, r.Events, 1)
	require.Empty(t, r.Events[0].Error, "multiple fatal Notifications can be decoded without reopening the context")
	require.Contains(t, r.Events[0].Session["Connection State"], "closed")
	r = third.Feed(0, time.Unix(8, 0), ldpTestKA(1))
	require.Equal(t, "context-required", r.Events[0].Status)
}
func TestLDPSessionMultiMessagePrefixAndNativeAdmission(t *testing.T) {
	fec := ldpTestTLV(0x100, []byte{2, 0, 1, 24, 192, 0, 2, 2, 0, 2, 32, 0x20, 1, 0xd, 0xb8})
	label := ldpTestTLV(0x200, []byte{0, 0, 0x10, 7})
	mapping := ldpTestMessage(0x400, 0xffffffff, append(fec, label...))
	w := ldpTestPDU(1, ldpTestMessage(0x201, 8, nil), mapping)
	m, err := stream_parser.DecodeLDPSessionPDU(w, 128)
	require.NoError(t, err)
	ms := m["Messages"].([]map[string]any)
	require.Len(t, ms, 2)
	rows := ms[1]["TLVs"].([]map[string]any)[0]["FEC Elements"].([]map[string]any)
	require.Equal(t, "192.0.2.0/24", rows[0]["Prefix"])
	require.Equal(t, "2001:db8::/32", rows[1]["Prefix"])
	s := observedTestSession(t, 646)
	r := s.Feed(0, time.Unix(1, 0), w)
	require.Len(t, r.Events, 1)
	require.Empty(t, r.Events[0].Error)
	require.Equal(t, []any{uint64(0x201), uint64(0x400)}, r.Events[0].DisplayFields()["ldp.msg.type"])
	require.Equal(t, []any{uint64(4103)}, r.Events[0].DisplayFields()["ldp.msg.tlv.generic.label"])
	require.Equal(t, []any{"192.0.2.0", "2001:db8::"}, r.Events[0].DisplayFields()["ldp.msg.tlv.fec.pfval"])
	for i := 0; i < len(w); i++ {
		_, err = stream_parser.DecodeLDPSessionPDU(w[:i], 128)
		require.Error(t, err, "cut %d", i)
	}
	bad := append([]byte(nil), w...)
	bad[12] = 0xff
	bad[13] = 0xff
	require.NotEqual(t, ProbeAccept, probeLDP(bad, 65539).Verdict)
	_, err = stream_parser.DecodeLDPSessionPDU(w, 2)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireBudget)
}

func TestLDPSessionCompleteBadPDUKeepsBoundariesAndQuarantinesNegotiation(t *testing.T) {
	s := observedTestSession(t, 646)
	s.Feed(0, time.Unix(1, 0), ldpTestInit(1, 2, 90, 0))
	bad := ldpTestMessage(0x400, 3, ldpTestTLV(0x100, []byte{2, 0, 1, 33, 192, 0, 2, 0, 0}))
	r := s.Feed(0, time.Unix(2, 0), append(ldpTestPDU(1, bad), ldpTestKA(1)...))
	require.Len(t, r.Events, 2, "framed malformed body must not erase next complete PDU")
	require.Equal(t, "malformed", r.Events[0].Status)
	require.Equal(t, "context-required", r.Events[1].Status)
	require.NotContains(t, r.Events[1].Session, "Observed Maximum PDU Length")
	r = s.Feed(1, time.Unix(3, 0), ldpTestInit(2, 1, 90, 0))
	require.Len(t, r.Events, 1)
	require.Equal(t, "context-required", r.Events[0].Status)
	// Independent TCP generation recovers; a quarantined old exchange is not
	// cleared by treating a second Initialization as a new session in place.
	fresh := observedTestSession(t, 646)
	r = fresh.Feed(0, time.Unix(4, 0), ldpTestInit(1, 2, 90, 0))
	require.Empty(t, r.Events[0].Error)
}

func TestLDPSessionFirstPDUBeyondProbeWindowWaitsForCompleteGrammar(t *testing.T) {
	var prefixes []byte
	for i := 0; i < 24; i++ {
		prefixes = append(prefixes, 2, 0, 1, 24, 192, 0, byte(i))
	}
	body := append(ldpTestTLV(0x100, prefixes), ldpTestTLV(0x200, []byte{0, 0, 0, 17})...)
	w := ldpTestPDU(1, ldpTestMessage(0x400, 19, body))
	for _, cut := range []int{64, 80, len(w) - 1} {
		s := observedTestSession(t, 646)
		r := s.Feed(0, time.Unix(1, 0), w[:cut])
		require.Empty(t, r.Events, "partial service hint cannot admit or discard the PDU at %d", cut)
		r = s.Feed(0, time.Unix(2, 0), w[cut:])
		require.Len(t, r.Events, 1)
		require.Equal(t, "decoded", r.Events[0].Status, r.Events[0].Error)
		require.Len(t, r.Events[0].DisplayFields()["ldp.msg.tlv.fec.pfval"], 24)
	}
}

func FuzzLDPSessionWireAndNegotiation(f *testing.F) {
	f.Add(ldpTestInit(1, 2, 90, 0), byte(0))
	f.Add(ldpTestKA(2), byte(3))
	f.Add(ldpTestPDU(1, ldpTestMessage(0x400, 4, append(ldpTestTLV(0x100, []byte{2, 0, 1, 24, 192, 0, 2}), ldpTestTLV(0x200, []byte{0, 0, 0, 7})...))), byte(6))
	f.Fuzz(func(t *testing.T, w []byte, dir byte) {
		if len(w) > 65539 {
			return
		}
		a := &binParser{budget: DefaultParserBudget()}
		flow := &binFlow{a: a, ldp: &binLDP{}}
		feed := func(direction int, wire []byte) { _ = flow.consumeLDP(direction, &ProtocolEvent{Raw: wire}) }
		if dir&2 != 0 {
			feed(0, ldpTestInit(1, 2, 90, 0))
			feed(1, ldpTestInit(2, 1, 60, 0))
			feed(1, ldpTestKA(2))
			feed(0, ldpTestKA(1))
		} else if dir&4 != 0 {
			feed(0, ldpTestInit(1, 2, 90, 0))
		}
		if dir&8 != 0 {
			feed(0, ldpTestPDU(1, ldpTestMessage(1, 4, ldpTestTLV(0x300, []byte{0x80, 0, 0, 9, 0, 0, 0, 0, 0, 0}))))
		}
		if dir&16 != 0 {
			feed(0, ldpTestInit(1, 2, 90, 0))
			feed(0, ldpTestInit(1, 2, 90, 0))
		}
		quarantined := flow.ldp.blocked || flow.ldp.closed
		e := &ProtocolEvent{Raw: w}
		_ = flow.consumeLDP(int(dir&1), e)
		if quarantined || dir&6 == 0 {
			require.False(t, flow.ldp.exchangeObserved())
		}
		_, observed := e.Session["Observed Maximum PDU Length"]
		require.Equal(t, flow.ldp.exchangeObserved(), observed)
		fresh := &binFlow{a: a, ldp: &binLDP{}}
		ka := &ProtocolEvent{Raw: ldpTestKA(1)}
		_ = fresh.consumeLDP(0, ka)
		require.NotContains(t, ka.Session, "Observed Maximum PDU Length", "independent generation cannot inherit prior context")
	})
}
