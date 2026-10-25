package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func isisNativeWire(t testing.TB, text string) []byte {
	t.Helper()
	w, err := hex.DecodeString(text)
	require.NoError(t, err)
	return w
}

func isisNativeEthernet(w []byte, vlans int) []byte {
	raw := []byte{1, 0x80, 0xc2, 0, 0, 0x14, 2, 0, 0, 0, 0, 1}
	for i := 0; i < vlans; i++ {
		raw = append(raw, 0x81, 0, 0, byte(i+1))
	}
	raw = binary.BigEndian.AppendUint16(raw, uint16(3+len(w)))
	raw = append(raw, 0xfe, 0xfe, 3)
	return append(raw, w...)
}

func TestISISNativeCarriersNoInheritedIdentity(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		var es []*ProtocolEvent
		a, err := NewPacketAnalyzer(WithProtocolDeferred(deferred), WithCaptureReplayClock(), WithOnProtocolMessage(func(e *ProtocolEvent) {
			if e.Protocol == "isis" {
				es = append(es, e)
			}
		}))
		require.NoError(t, err)
		w := isisNativeWire(t, "831401001101000003010001000100001e001401")
		captureErrorRetained := false
		feed := func(raw []byte, link layers.LinkType, iface int) {
			err := a.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(raw), Length: len(raw), InterfaceIndex: iface}, link)
			if captureErrorRetained {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			clear(raw)
		}
		for vlan := 0; vlan <= 4; vlan++ {
			feed(isisNativeEthernet(w, vlan), layers.LinkTypeEthernet, vlan)
		}
		// Verified mPacket framing has a different physical origin; never strip
		// a guessed eight-byte prefix from ordinary Ethernet input.
		eth := isisNativeEthernet(w, 0)
		mp := append([]byte{0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0xd5}, eth...)
		mp = binary.LittleEndian.AppendUint32(mp, crc32.ChecksumIEEE(eth))
		badFCS := bytes.Clone(mp)
		badFCS[len(badFCS)-1] ^= 1
		require.Error(t, a.Feed(badFCS, gopacket.CaptureInfo{CaptureLength: len(badFCS), Length: len(badFCS)}, 274))
		captureErrorRetained = true // Feed/Close retain earlier capture errors.
		feed(mp, 274, 0)
		for _, index := range []int{14, 15, 16, 17} {
			bad := isisNativeEthernet(w, 0)
			bad[index] ^= 1
			feed(bad, layers.LinkTypeEthernet, 0)
		}
		changed := bytes.Clone(w)
		changed[9] = 2
		feed(isisNativeEthernet(changed, 0), layers.LinkTypeEthernet, 0)
		feed(isisNativeEthernet(w, 0), layers.LinkTypeEthernet, 0)
		require.Error(t, a.Close())
		require.Len(t, es, 8)
		for i, e := range es {
			require.Contains(t, []string{"decoded", "deferred"}, e.Status)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, "ISISFields", e.Entry)
			m, err := e.GetFields()
			require.NoError(t, err)
			want := "0100.0100.0100"
			if i == 6 {
				want = "0200.0100.0100"
			}
			require.Equal(t, want, m["Source System ID"])
			require.EqualValues(t, 30, m["Holding Timer Seconds"])
			require.Equal(t, false, m["Authentication Verified"])
		}
		f, err := CompileDisplayFilter("isis.type == 17 and isis.hello.holding_timer == 30 and isis.hello.source_id == \"0100.0100.0100\"")
		require.NoError(t, err)
		require.True(t, f.EvaluateEvents(es[:1]).Matched)
		es[0].Session["Source System ID"] = "changed"
		require.True(t, f.EvaluateEvents(es[:1]).Matched, "public edits cannot replace frozen evidence")
	}
}

func TestISISNativeMalformedRepairAndUnknownFieldScope(t *testing.T) {
	var es []*ProtocolEvent
	a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) {
		if e.Protocol == "isis" {
			es = append(es, e)
		}
	}))
	require.NoError(t, err)
	w := isisNativeWire(t, "831b010012010000001b04b0010001000100000000000001dd1d01")
	bad := bytes.Clone(w)
	bad[23] ^= 1
	partial := bytes.Clone(w)
	partial[8], partial[9] = 0, 26
	hello := isisNativeWire(t, "831401001101000003010001000100001e001801fa020102")
	for _, raw := range [][]byte{isisNativeEthernet(bad, 0), isisNativeEthernet(partial, 0), isisNativeEthernet(hello, 0), isisNativeEthernet(w, 0)} {
		require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	}
	cut := isisNativeEthernet(w, 0)
	require.Error(t, a.Feed(cut, gopacket.CaptureInfo{CaptureLength: len(cut), Length: len(cut) + 1}, layers.LinkTypeEthernet))
	require.Error(t, a.Close())
	require.Len(t, es, 5)
	require.Equal(t, "malformed", es[0].Status)
	require.Equal(t, true, es[0].Session["Wire Grammar Complete"])
	require.Equal(t, true, es[0].Session["Checksum Failure"])
	require.Empty(t, es[1].Session)
	require.Equal(t, true, es[2].Session["Unselected Extensions"])
	require.Equal(t, "decoded", es[3].Status)
	require.Equal(t, true, es[3].Session["Checksum Verified"])
	require.Empty(t, es[4].Session)
	for _, i := range []int{0, 1, 2, 4} {
		f, err := CompileDisplayFilter("not isis.lsp.hostname")
		require.NoError(t, err)
		require.False(t, f.EvaluateEvents(es[i:i+1]).Complete)
	}
}

func TestISISTypedNeighborMetricsKeepDistinctWireRanges(t *testing.T) {
	// Complete LSP structure with an intentionally absent checksum; no integrity
	// success follows merely from parsing these values.
	w := isisNativeWire(t, "831b010012010000002904b0010001000100000000000001000001020c00018142c301020304050607")
	var es []*ProtocolEvent
	a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) {
		if e.Protocol == "isis" {
			es = append(es, e)
		}
	}))
	require.NoError(t, err)
	raw := isisNativeEthernet(w, 0)
	require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	require.NoError(t, a.Close())
	require.Len(t, es, 1)
	require.Equal(t, "decoded", es[0].Status)
	snap := es[0].DisplayFieldValues()
	for i, name := range []string{"default", "delay", "expense", "error"} {
		found := false
		for _, f := range snap.Values {
			if f.Name == "isis.lsp.eis_neighbors."+name+"_metric" {
				require.EqualValues(t, []uint64{1, 1, 2, 3}[i], f.Value)
				require.Equal(t, &DisplayFieldRange{Offset: 30 + i, Length: 1}, f.Range)
				found = true
			}
		}
		require.True(t, found, name)
	}
	require.Equal(t, false, es[0].Session["Checksum Verified"])
}

func TestISISTypedAreaEnvelopeAndReservedBitValues(t *testing.T) {
	w := isisNativeWire(t, "831b01008f01000083010001000100001e00218101000100010001010403490001")
	var es []*ProtocolEvent
	a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) {
		if e.Protocol == "isis" {
			es = append(es, e)
		}
	}))
	require.NoError(t, err)
	raw := isisNativeEthernet(w, 0)
	require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	require.NoError(t, a.Close())
	require.Len(t, es, 1)
	snap := es[0].DisplayFieldValues()
	values := snap.FieldMap()
	require.Equal(t, []any{uint64(4)}, values["isis.type.reserved"])
	require.Equal(t, []any{uint64(32), uint64(1)}, values["isis.hello.reserved"])
	require.Equal(t, []any{"03:49:00:01"}, values["isis.hello.area_address"])
	require.Equal(t, []any{"49.0001"}, values["isis.hello.observed.area_address"])
	for _, f := range snap.Values {
		if f.Name == "isis.hello.area_address" {
			require.Equal(t, &DisplayFieldRange{Offset: 29, Length: 4}, f.Range)
		}
	}
}
