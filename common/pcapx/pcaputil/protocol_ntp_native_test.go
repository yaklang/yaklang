package pcaputil

import (
	"bytes"
	"encoding/binary"
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
	e := &ProtocolEvent{ID: a.ids.Add(1), Transport: "udp", Domain: domain, Source: "127.0.0.1:20000", Destination: "127.0.0.1:123"}
	e.Timestamp = time.Unix(1600000000, int64(e.ID)*int64(time.Millisecond))
	src, dst := uint16(20000), uint16(123)
	if response {
		e.Source, e.Destination, src, dst = e.Destination, e.Source, dst, src
	}
	e.SourceBytes = ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: e.ID, Domain: domain}}}
	require.True(t, a.decodeNTPDatagram(e, w, src, dst, false))
	return e
}

func TestNativeNTPModesAndLiteralTimeFields(t *testing.T) {
	for version := byte(1); version <= 4; version++ {
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
			require.Equal(t, float64(-0.5), e.Session["Root Delay Seconds"])
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
