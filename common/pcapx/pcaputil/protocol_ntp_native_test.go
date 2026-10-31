package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	binparser "github.com/yaklang/yaklang/common/bin-parser/parser"
)

func ntpControlWire(seq uint16, flags byte, off uint16, data []byte) []byte {
	w := make([]byte, 12+len(data))
	w[0], w[1] = 2<<3|6, flags
	binary.BigEndian.PutUint16(w[2:4], seq)
	binary.BigEndian.PutUint16(w[8:10], off)
	binary.BigEndian.PutUint16(w[10:12], uint16(len(data)))
	copy(w[12:], data)
	return w
}

func ntpNativeParser() *binParser {
	return &binParser{budget: DefaultParserBudget(), config: BinParserConfig{MaxMessageBytes: 1 << 20, MaxBufferedBytes: 1 << 20}}
}
func ntpNativeFeed(t *testing.T, a *binParser, response bool, domain CaptureDomain, w []byte) *ProtocolEvent {
	physical := a.ids.Load() + 1
	e := &ProtocolEvent{Transport: "udp", Domain: domain, Source: "127.0.0.1:20000", Destination: "127.0.0.1:123"}
	e.Timestamp = time.Unix(1600000000, int64(physical)*int64(time.Millisecond))
	src, dst := uint16(20000), uint16(123)
	if response {
		e.Source, e.Destination, src, dst = e.Destination, e.Source, dst, src
	}
	e.SourceBytes = ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: physical, Domain: domain}}}
	require.True(t, a.decodeNTPDatagram(e, w, src, dst, false))
	return e
}

func TestNativeNTPControlRealIngressOwnsIDsBeforePublication(t *testing.T) {
	one, two := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
	wires := [][]byte{ntpControlWire(37, 2, 0, nil), ntpControlWire(37, 128|2, 3, []byte("def")), ntpControlWire(37, 128|32|2, 0, []byte("abc")), ntpControlWire(37, 128|2, 3, []byte("def"))}
	var capture []byte
	for i, w := range wires {
		src, dst, sp, dp := one, two, uint16(20000), uint16(123)
		if i > 0 {
			src, dst, sp, dp = dst, src, dp, sp
		}
		part := t20UDPCapture(t, src, dst, sp, dp, w)
		if i == 0 {
			capture = part
		} else {
			capture = append(capture, part[24:]...)
		}
	}
	for _, deferred := range []bool{false, true} {
		var es []*ProtocolEvent
		require.NoError(t, ReplayPcap(bytes.NewReader(capture), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) })))
		require.Len(t, es, 4)
		require.Positive(t, es[0].ID)
		require.Equal(t, es[0].ID, es[0].TransactionID)
		require.Equal(t, es[0].ID, es[1].ResponseTo)
		require.Equal(t, es[0].ID, es[2].ResponseTo)
		require.Equal(t, []byte("abcdef"), es[2].Session["Reassembled Control Data"])
		require.Nil(t, es[3].Session["Reassembled Control Data"])
		require.Equal(t, []uint64{es[1].ID, es[2].ID}, es[2].Session["Reassembled Byte Source"].(map[string]any)["Parent PDUs"])
	}
}

func TestNativeNTPRejectedDatagramCannotFallBackToLegacyProbe(t *testing.T) {
	// A large unrelated UDP message can share the old probe's leading version,
	// mode and stratum. The native admission decision must survive dispatch.
	w := make([]byte, 780)
	w[0], w[2], w[3] = 0x63, 0x2d, 0x97
	for _, deferred := range []bool{false, true} {
		var es []*ProtocolEvent
		capture := t20UDPCapture(t, net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2), 13416, 6635, w)
		require.NoError(t, ReplayPcap(bytes.NewReader(capture), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) })))
		require.Len(t, es, 1)
		require.Empty(t, es[0].Protocol, "unselected extended NTP-like prefix must remain unknown UDP")
		require.Zero(t, es[0].ResponseTo)
		capture = t20UDPCapture(t, net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2), 13416, 123, w)
		es = nil
		require.NoError(t, ReplayPcap(bytes.NewReader(capture), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) })))
		require.Len(t, es, 1)
		require.Equal(t, "ntp", es[0].Protocol)
		require.Equal(t, "ntp-native", es[0].Profile)
		require.Equal(t, "context-required", es[0].Status, "service selection can expose a partial header without inventing extension semantics")
	}
}

func TestNativeNTPModesAndLiteralTimeFields(t *testing.T) {
	for version := byte(2); version <= 4; version++ {
		for mode := byte(1); mode <= 5; mode++ {
			w := make([]byte, 48)
			w[0], w[1], w[2], w[3] = version<<3|mode, 1, 255, 128
			binary.BigEndian.PutUint32(w[4:8], 0xffff8000)
			binary.BigEndian.PutUint32(w[40:44], 0xffffffff)
			binary.BigEndian.PutUint32(w[44:48], 0xfffffff1)
			e := ntpNativeFeed(t, ntpNativeParser(), false, CaptureDomain{}, w)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Equal(t, -1, e.Session["Poll"])
			require.Equal(t, -128, e.Session["Precision"])
			if version == 2 {
				require.Equal(t, float64(65535.5), e.Session["Synchronizing Distance Seconds"])
				require.Nil(t, e.Session["Root Delay Seconds"])
			} else {
				require.Equal(t, float64(-0.5), e.Session["Root Delay Seconds"])
			}
			require.Equal(t, uint32(0xffffffff), e.Session["Transmit Timestamp Seconds"])
			require.Equal(t, uint32(0xfffffff1), e.Session["Transmit Timestamp Fraction"])
			require.Contains(t, e.Session["Timestamp Scope"], "era")
			require.Zero(t, e.ResponseTo)
		}
	}
	a := ntpNativeParser()
	w := make([]byte, 48)
	w[0] = 4<<3 | 1
	require.False(t, a.decodeNTPDatagram(&ProtocolEvent{}, w, 20000, 20001, false), "symmetric mode requires full grammar plus service selection")
	require.False(t, a.decodeNTPDatagram(&ProtocolEvent{}, ntpControlWire(1, 2, 0, nil), 20000, 20001, false), "no port-only/control-prefix discovery on arbitrary UDP")
}

func TestNativeNTPv1UsesReservedBitsDistanceAndDrift(t *testing.T) {
	w := make([]byte, 48)
	w[0], w[1] = 8, 1
	binary.BigEndian.PutUint32(w[4:8], 0x80000000)
	binary.BigEndian.PutUint32(w[8:12], 0x80000000)
	e := ntpNativeFeed(t, ntpNativeParser(), false, CaptureDomain{}, w)
	require.Equal(t, "decoded", e.Status, e.Error)
	require.Nil(t, e.Session["Mode"])
	require.Nil(t, e.Session["Root Dispersion Raw"])
	require.Equal(t, float64(32768), e.Session["Synchronizing Distance Seconds"])
	require.Equal(t, float64(.5), e.Session["Estimated Drift Rate"])
	require.Nil(t, e.DisplayFields()["ntp.flags.mode"])
	require.Equal(t, []any{uint64(0)}, e.DisplayFields()["ntp.v1.reserved"])
	_, err := binparser.ParseBinary(&liveBoundedReader{bytes.NewReader(w), uint64(len(w)) * 8}, "application-layer.ntp", "NTP")
	require.NoError(t, err)
	w[0] = 0x0b
	_, err = decodeNTPDatagramWire(w)
	require.Error(t, err)
	_, err = binparser.ParseBinary(&liveBoundedReader{bytes.NewReader(w), uint64(len(w)) * 8}, "application-layer.ntp", "NTP")
	require.Error(t, err)
}

func TestNativeNTPControlSelectedPeerAndStatusMeaning(t *testing.T) {
	a := ntpNativeParser()
	r := ntpNativeFeed(t, a, false, CaptureDomain{}, ntpControlWire(4, 2, 0, nil))
	require.Equal(t, "request-reserved", r.Session["Status Word Type"])
	require.Nil(t, r.Session["Reported Clock Source"])
	head := ntpControlWire(4, 128|32|2, 0, []byte("abc"))
	binary.BigEndian.PutUint16(head[6:8], 7)
	one := ntpNativeFeed(t, a, true, CaptureDomain{}, head)
	require.Equal(t, r.ID, one.ResponseTo)
	tail := ntpControlWire(4, 128|2, 3, []byte("def"))
	binary.BigEndian.PutUint16(tail[6:8], 7)
	e := ntpNativeFeed(t, a, true, CaptureDomain{}, tail)
	require.Equal(t, []byte("abcdef"), e.Session["Reassembled Control Data"])
	require.Equal(t, uint16(0), e.Session["Requested Association ID"])
	require.Equal(t, uint16(7), e.Session["Association ID"])
	a = ntpNativeParser()
	ntpNativeFeed(t, a, false, CaptureDomain{}, ntpControlWire(4, 2, 0, nil))
	ntpNativeFeed(t, a, true, CaptureDomain{}, head)
	binary.BigEndian.PutUint16(tail[6:8], 9)
	e = ntpNativeFeed(t, a, true, CaptureDomain{}, tail)
	require.Zero(t, e.ResponseTo, "selected peer cannot change between response fragments")
	require.Equal(t, "refused", e.Session["Control Reassembly"])
	clock := ntpControlWire(5, 128|4, 0, nil)
	binary.BigEndian.PutUint16(clock[4:6], 0x0102)
	m, err := decodeNTPDatagramWire(clock)
	require.NoError(t, err)
	require.Equal(t, 1, m["Clock Status High Byte"])
	require.Nil(t, m["Reported Clock Status"], "no guessed hardware state from a status-word variant")
	trap := ntpControlWire(6, 128|6, 0, nil)
	m, err = decodeNTPDatagramWire(trap)
	require.NoError(t, err)
	require.Equal(t, "insignificant-set-trap-response", m["Status Word Type"])
}

func TestNativeNTPRefusedRequestCannotOwnLaterErrorAndFieldsAreExplicitlyPartial(t *testing.T) {
	for _, pending := range []bool{false, true} {
		a := ntpNativeParser()
		d := CaptureDomain{}
		if pending {
			ntpNativeFeed(t, a, false, d, ntpControlWire(8, 2, 0, nil))
		}
		bad := ntpControlWire(8, 2, 0, nil)
		binary.BigEndian.PutUint16(bad[10:12], 469)
		_, err := binparser.ParseBinary(&liveBoundedReader{bytes.NewReader(bad), uint64(len(bad)) * 8}, "application-layer.ntp", "NTP")
		require.Error(t, err)
		require.Equal(t, "malformed", ntpNativeFeed(t, a, false, d, bad).Status)
		r := ntpNativeFeed(t, a, false, d, ntpControlWire(8, 2, 0, nil))
		require.Zero(t, r.TransactionID)
		e := ntpNativeFeed(t, a, true, d, ntpControlWire(8, 128|64|2, 0, nil))
		require.Zero(t, e.ResponseTo)
	}
	a := ntpNativeParser()
	a.budget.MaxCollectionElements = 1
	ntpNativeFeed(t, a, false, CaptureDomain{}, ntpControlWire(9, 1, 0, nil))
	e := ntpNativeFeed(t, a, true, CaptureDomain{}, ntpControlWire(9, 128|1, 0, make([]byte, 8)))
	require.Equal(t, "limited", e.Completeness)
	require.Equal(t, 2, e.Session["Reported Association Count"])
	require.Nil(t, e.Session["Reported Associations"])
	require.True(t, e.DisplayFieldValues().Incomplete)
	for _, w := range [][]byte{{0x23}, {0x16, 2, 0, 1}} {
		r := &ProtocolEvent{Transport: "udp"}
		require.True(t, a.decodeNTPDatagram(r, w, 20000, 123, true))
		require.NotContains(t, r.Summary, "<nil>")
	}
}

func FuzzNTPWireBoundsAndSemanticIsolation(f *testing.F) {
	f.Add(ntpControlWire(1, 2, 0, nil))
	f.Add(ntpControlWire(1, 128|2, 0, []byte("abc")))
	f.Add(append([]byte{0x23}, make([]byte, 47)...))
	f.Add([]byte{0x16})
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 2048 {
			return
		}
		m, err := decodeNTPDatagramWire(w)
		if m == nil {
			require.Error(t, err)
			return
		}
		for _, raw := range m["Wire Field Ranges"].(map[string]any) {
			span := raw.([]any)
			lo, hi := span[0].(int), span[1].(int)
			require.True(t, 0 <= lo && lo <= hi && hi <= len(w))
		}
		if err != nil {
			return
		}
		if w[0]>>3&7 != 1 && w[0]&7 == 6 {
			require.GreaterOrEqual(t, len(w), 12)
			n := int(binary.BigEndian.Uint16(w[10:12]))
			require.LessOrEqual(t, n, 468)
			require.Equal(t, w[12:12+n], m["Control Data"])
			require.Nil(t, m["Transmit Timestamp"])
		} else {
			require.Len(t, w, 48)
			require.Nil(t, m["Control Data"])
		}
	})
}

func assertNativeNTPLegacyVersionReview(t testing.TB, events []*ProtocolEvent) {
	t.Helper()
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "ntp", e.Protocol)
	require.Equal(t, "ntp-native", e.Profile)
	require.Empty(t, e.Error)
	require.Equal(t, 2, e.Session["Version"])
	require.Equal(t, 3, e.Session["Mode"])
	require.Len(t, e.Raw, 48)
	require.Equal(t, byte(0x13), e.Raw[0])
	require.LessOrEqual(t, e.Raw[1], byte(16))
	fields, err := e.GetFields()
	require.NoError(t, err)
	for i, name := range []string{"Reference", "Origin", "Receive", "Transmit"} {
		at := 16 + 8*i
		require.Equal(t, e.Raw[at:at+8], fields[name+" Timestamp"])
		require.Equal(t, binary.BigEndian.Uint32(e.Raw[at:at+4]), fields[name+" Timestamp Seconds"])
		require.Equal(t, binary.BigEndian.Uint32(e.Raw[at+4:at+8]), fields[name+" Timestamp Fraction"])
	}
	require.Zero(t, e.ResponseTo, "v2 wire support does not fabricate a time exchange")
}

func TestNativeNTPControlOutOfOrderRecoveryDuplicateAndSequenceReuse(t *testing.T) {
	a := ntpNativeParser()
	d := CaptureDomain{Interface: 3}
	req := ntpControlWire(7, 2, 0, nil)
	r := ntpNativeFeed(t, a, false, d, req)
	require.Equal(t, "decoded", r.Status, r.Error)
	tail := ntpControlWire(7, 128|2, 3, []byte("def"))
	e := ntpNativeFeed(t, a, true, d, tail)
	require.Equal(t, r.ID, e.ResponseTo)
	require.Equal(t, false, e.Session["Reassembly Complete"])
	require.Nil(t, e.Session["Reassembled Control Data"])
	head := ntpControlWire(7, 128|32|2, 0, []byte("abc"))
	e = ntpNativeFeed(t, a, true, d, head)
	require.Equal(t, true, e.Session["Reassembly Complete"])
	require.Equal(t, []byte("abcdef"), e.Session["Reassembled Control Data"])
	require.Equal(t, true, e.Session["Reassembled Byte Source"].(map[string]any)["Packet Coverage Complete"])
	dup := ntpNativeFeed(t, a, true, d, tail)
	require.Equal(t, "duplicate-no-replay", dup.Session["Control Reassembly"])
	require.Nil(t, dup.Session["Reassembled Control Data"])
	require.Zero(t, dup.ResponseTo, "identical observed wire cannot prove a later UDP generation")
	reuse := ntpNativeFeed(t, a, false, d, req)
	require.Equal(t, "ambiguous-sequence", reuse.Session["Association"])
	require.Zero(t, ntpNativeFeed(t, a, true, d, head).ResponseTo)
	fresh := ntpNativeFeed(t, a, false, d, ntpControlWire(8, 2, 0, nil))
	require.Equal(t, fresh.ID, ntpNativeFeed(t, a, true, d, ntpControlWire(8, 128|2, 0, []byte("new"))).ResponseTo)
	before := a.buffered.Load()
	require.Positive(t, before)
	a.closeUDPSessions()
	require.Zero(t, a.buffered.Load())
}

func TestNativeNTPControlConflictOrphanDomainAndBudgetIsolation(t *testing.T) {
	for _, kind := range []string{"overlap", "status", "final", "opcode"} {
		t.Run(kind, func(t *testing.T) {
			a := ntpNativeParser()
			d := CaptureDomain{}
			ntpNativeFeed(t, a, false, d, ntpControlWire(1, 2, 0, nil))
			ntpNativeFeed(t, a, true, d, ntpControlWire(1, 128|32|2, 0, []byte("abc")))
			bad := ntpControlWire(1, 128|2, 2, []byte("cd"))
			switch kind {
			case "overlap":
				bad[12] = 'x'
			case "status":
				bad[4] = 1
			case "final":
				bad = ntpControlWire(1, 128|2, 0, []byte("a"))
			case "opcode":
				bad[1] = 128 | 4
			}
			e := ntpNativeFeed(t, a, true, d, bad)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, "refused", e.Session["Control Reassembly"])
			require.Zero(t, ntpNativeFeed(t, a, true, d, ntpControlWire(1, 128|2, 3, []byte("d"))).ResponseTo)
		})
	}
	a := ntpNativeParser()
	d := CaptureDomain{}
	ntpNativeFeed(t, a, true, d, ntpControlWire(2, 128|2, 0, nil))
	require.Zero(t, ntpNativeFeed(t, a, false, d, ntpControlWire(2, 2, 0, nil)).TransactionID, "orphan response history cannot be adopted by a later request")
	r := ntpNativeFeed(t, a, false, CaptureDomain{Interface: 1}, ntpControlWire(2, 2, 0, nil))
	require.Equal(t, r.ID, ntpNativeFeed(t, a, true, CaptureDomain{Interface: 1}, ntpControlWire(2, 128|2, 0, nil)).ResponseTo)
	b := ntpNativeParser()
	b.config.MaxBufferedBytes = 1
	require.Equal(t, "untracked-control-history", ntpNativeFeed(t, b, false, d, ntpControlWire(3, 2, 0, nil)).Session["Association"])
	b.config.MaxBufferedBytes = 1 << 20
	require.Zero(t, ntpNativeFeed(t, b, false, d, ntpControlWire(3, 2, 0, nil)).TransactionID, "unretained history does not become fresh after a budget change")
}

func TestNativeNTPControlBoundsAndBinParserEntry(t *testing.T) {
	w := ntpControlWire(1, 128|2, 468, []byte("abc"))
	w = append(w, 0)
	m, err := decodeNTPDatagramWire(w)
	require.NoError(t, err)
	require.Equal(t, []byte("abc"), m["Control Data"])
	require.Nil(t, m["Transmit Timestamp"], "control sequence and status are not timestamps")
	_, err = binparser.ParseBinary(&liveBoundedReader{bytes.NewReader(w), uint64(len(w)) * 8}, "application-layer.ntp", "NTP")
	require.NoError(t, err)
	bad := bytes.Clone(w)
	binary.BigEndian.PutUint16(bad[10:12], 469)
	_, err = decodeNTPDatagramWire(bad)
	require.Error(t, err)
	_, err = binparser.ParseBinary(&liveBoundedReader{bytes.NewReader(bad), uint64(len(bad)) * 8}, "application-layer.ntp", "NTP")
	require.Error(t, err)
	for n := 0; n < 12; n++ {
		_, err = decodeNTPDatagramWire(w[:n])
		require.Error(t, err)
	}
	bad = bytes.Clone(w)
	bad[len(bad)-1] = 1
	_, err = decodeNTPDatagramWire(bad)
	require.Error(t, err)
	_, err = binparser.ParseBinary(&liveBoundedReader{bytes.NewReader(bad), uint64(len(bad)) * 8}, "application-layer.ntp", "NTP")
	require.Error(t, err)
	_, err = decodeNTPDatagramWire(append(ntpControlWire(1, 2, 0, nil), 1))
	require.Error(t, err)
}

func TestNativeNTPFieldOwnershipAndLiteralRange(t *testing.T) {
	cfg := NewDefaultConfig()
	require.NoError(t, WithProtocolDecodeAs("udp", 20001, "ntp")(cfg))
	a := ntpNativeParser()
	e := ntpNativeFeed(t, a, false, CaptureDomain{}, ntpControlWire(19, 2, 0, nil))
	e.Session["Sequence Number"], e.Session["Wire Field Ranges"] = uint16(999), map[string]any{}
	values := e.DisplayFieldValues()
	require.Equal(t, []any{uint64(19)}, values.FieldMap()["ntp.ctrl.sequence"])
	for _, v := range values.Values {
		if v.Name == "ntp.ctrl.sequence" {
			require.Equal(t, &DisplayFieldRange{Offset: 2, Length: 2}, v.Range)
		}
	}
	e.Fields = map[string]any{"Sequence Number": uint16(999)}
	fields, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, uint16(19), fields["Sequence Number"])
	ntpNativeFeed(t, a, false, CaptureDomain{}, ntpControlWire(20, 1, 0, nil))
	bad := ntpNativeFeed(t, a, true, CaptureDomain{}, ntpControlWire(20, 128|1, 0, []byte("bad")))
	require.Equal(t, "malformed", bad.Status)
	require.Equal(t, true, bad.Session["Reassembly Complete"], "complete bytes do not mean valid list semantics")
	require.Zero(t, bad.ResponseTo)
}

func FuzzNTPControlReassemblyIsolation(f *testing.F) {
	f.Add(uint16(1), uint8(12), true)
	f.Add(uint16(65535), uint8(0), false)
	f.Fuzz(func(t *testing.T, seq uint16, n uint8, reverse bool) {
		if seq == 0 {
			seq = 1
		}
		a := ntpNativeParser()
		d := CaptureDomain{Interface: 3}
		r := ntpNativeFeed(t, a, false, d, ntpControlWire(seq, 2, 0, nil))
		body := bytes.Repeat([]byte{n}, int(n%96)+1)
		cut := len(body) / 2
		head := ntpControlWire(seq, 128|32|2, 0, body[:cut])
		tail := ntpControlWire(seq, 128|2, uint16(cut), body[cut:])
		if cut == 0 {
			head = nil
			tail = ntpControlWire(seq, 128|2, 0, body)
		}
		var last *ProtocolEvent
		if reverse && head != nil {
			ntpNativeFeed(t, a, true, d, tail)
			last = ntpNativeFeed(t, a, true, d, head)
		} else {
			if head != nil {
				ntpNativeFeed(t, a, true, d, head)
			}
			last = ntpNativeFeed(t, a, true, d, tail)
		}
		require.Equal(t, r.ID, last.ResponseTo)
		require.Equal(t, body, last.Session["Reassembled Control Data"])
		require.Nil(t, ntpNativeFeed(t, a, true, d, tail).Session["Reassembled Control Data"], "no replay")
		ntpNativeFeed(t, a, false, d, ntpControlWire(seq, 2, 0, nil))
		require.Zero(t, ntpNativeFeed(t, a, true, d, tail).ResponseTo, "no association across sequence reuse")
		a.closeUDPSessions()
		require.Zero(t, a.buffered.Load())
	})
}

func TestNativeNTPControlClockAndHistoryExhaustion(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 31 * time.Second} {
		a := ntpNativeParser()
		r := ntpNativeFeed(t, a, false, CaptureDomain{}, ntpControlWire(1, 2, 0, nil))
		e := &ProtocolEvent{ID: a.ids.Add(1), Timestamp: r.Timestamp.Add(delta), Transport: "udp", Source: r.Destination, Destination: r.Source}
		require.True(t, a.decodeNTPDatagram(e, ntpControlWire(1, 128|2, 0, []byte("late")), 123, 20000, false))
		require.Zero(t, e.ResponseTo)
		require.Equal(t, "refused", e.Session["Control Reassembly"])
		require.Zero(t, ntpNativeFeed(t, a, true, CaptureDomain{}, ntpControlWire(1, 128|2, 0, []byte("old"))).ResponseTo)
	}
	a := ntpNativeParser()
	a.budget.MaxCollectionElements = 1
	r := ntpNativeFeed(t, a, false, CaptureDomain{}, ntpControlWire(1, 2, 0, nil))
	require.Zero(t, ntpNativeFeed(t, a, false, CaptureDomain{}, ntpControlWire(2, 2, 0, nil)).TransactionID)
	require.Equal(t, r.ID, ntpNativeFeed(t, a, true, CaptureDomain{}, ntpControlWire(1, 128|2, 0, nil)).ResponseTo, "already retained ownership survives an unrelated capacity refusal")
}
