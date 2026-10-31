package pcaputil

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// Frozen JSON used text for binary header fields. Keep its inputs and answers
// intact, bind the historical lossy spelling to the literal old wire value,
// and separately review only the new byte type and clear-header provenance.
func quicReviewedLegacyExpectations(t *testing.T, e *ProtocolEvent, session, fields map[string]any) (map[string]any, map[string]any) {
	t.Helper()
	session, fields = cloneSession(session), cloneSession(fields)
	w := e.Raw
	if e.Session["Wire Field Ranges"] == nil {
		return session, fields
	}
	require.NotNil(t, session, "new metadata cannot invent an absent historical session")
	require.NotEmpty(t, w)
	ranges := map[string]any{}
	span := func(name string, lo, hi int) { ranges[name] = []any{lo, hi} }
	reviewOctets := func(m map[string]any, name string, octets []byte) {
		old, present := m[name]
		if !present {
			return
		}
		if b, ok := old.([]byte); ok {
			require.Equal(t, octets, b)
		} else {
			encoded, err := json.Marshal(string(octets))
			require.NoError(t, err)
			var historical string
			require.NoError(t, json.Unmarshal(encoded, &historical))
			require.Equal(t, historical, old, "immutable legacy spelling must still match that exact captured byte sequence")
		}
		m[name] = bytes.Clone(octets)
	}
	if w[0]&128 != 0 {
		require.GreaterOrEqual(t, len(w), 7)
		dl := int(w[5])
		require.LessOrEqual(t, 7+dl, len(w))
		sl := int(w[6+dl])
		require.LessOrEqual(t, 7+dl+sl, len(w))
		span("Version", 1, 5)
		span("DCID Length", 5, 6)
		span("DCID", 6, 6+dl)
		span("SCID Length", 6+dl, 7+dl)
		span("SCID", 7+dl, 7+dl+sl)
		if binary.BigEndian.Uint32(w[1:5]) == 1 {
			span("Long Packet Type", 0, 1)
		}
		reviewOctets(fields, "DCID", w[6:6+dl])
		reviewOctets(fields, "SCID", w[7+dl:7+dl+sl])
		if token, ok := fields["QUICToken"].(map[string]any); ok {
			off := 7 + dl + sl
			require.Less(t, off, len(w))
			n := int(w[off])
			require.Less(t, n, 64)
			require.LessOrEqual(t, off+1+n, len(w))
			reviewOctets(token, "Token", w[off+1:off+1+n])
		}
		version, kind := binary.BigEndian.Uint32(w[1:5]), w[0]>>4&3
		off := 7 + dl + sl
		if version == 1 && kind == 0 || version == 0x6b3343cf && kind == 1 {
			require.Less(t, off, len(w))
			width := 1 << (w[off] >> 6)
			require.LessOrEqual(t, off+width, len(w))
			length := uint64(w[off] & 63)
			for _, b := range w[off+1 : off+width] {
				length = length<<8 | uint64(b)
			}
			require.LessOrEqual(t, length, uint64(len(w)-off-width))
			end := off + width + int(length)
			for key, value := range map[string]any{"Initial Token Length": length, "Initial Token Length Encoding Bytes": width, "Initial Token": bytes.Clone(w[off+width : end])} {
				require.NotContains(t, session, key, "these are newly reviewed literal header facts, never old expected values")
				session[key] = value
			}
			span("Initial Token Length", off, off+width)
			span("Initial Token", off+width, end)
		} else if version == 1 && kind == 3 || version == 0x6b3343cf && kind == 0 {
			require.LessOrEqual(t, off+16, len(w))
			for key, value := range map[string]any{"Retry Token": bytes.Clone(w[off : len(w)-16]), "Retry Integrity Tag": bytes.Clone(w[len(w)-16:])} {
				require.NotContains(t, session, key)
				session[key] = value
			}
			span("Retry Token", off, len(w)-16)
			span("Retry Integrity Tag", len(w)-16, len(w))
		}
	} else {
		cid, ok := e.Session["DCID"].([]byte)
		require.True(t, ok)
		require.LessOrEqual(t, 1+len(cid), len(w))
		require.Equal(t, cid, w[1:1+len(cid)])
		span("DCID", 1, 1+len(cid))
	}
	rocEqualFields(t, ranges, e.Session["Wire Field Ranges"].(map[string]any))
	session["Wire Field Ranges"] = ranges
	// The frozen expected CRYPTO plaintext independently supplies these new
	// handshake facts; neither the actual parser tree nor ciphertext labels do.
	tlsFrozenQUICMetadata(t, session)
	return session, fields
}

func TestNativeQUICEnvelopeBinaryFieldsRemainLosslessAcrossJSON(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		w := []byte{0xc0, 0, 0, 0, 1, 3, 0xff, 0, 0xc3, 2, 0xfe, 0x80, 2, 0xff, 0xfe, 2, 0, 0}
		if v2 {
			copy(w[:5], []byte{0xd0, 0x6b, 0x33, 0x43, 0xcf})
		}
		f, err := quicWireEnvelope(w)
		// This tests the bounded layout/ownership only; authenticated v2 wire
		// ingress is separately covered by RFC9369 and actual peer captures.
		require.NoError(t, err)
		require.Equal(t, []byte{0xff, 0, 0xc3}, f["DCID"])
		require.Equal(t, []byte{0xfe, 0x80}, f["SCID"])
		require.Equal(t, []byte{0xff, 0xfe}, f["QUICToken"].(map[string]any)["Token"])
		encoded, err := json.Marshal(f)
		require.NoError(t, err)
		var ipc map[string]any
		require.NoError(t, json.Unmarshal(encoded, &ipc))
		b, err := base64.StdEncoding.DecodeString(ipc["DCID"].(string))
		require.NoError(t, err)
		require.Equal(t, []byte{0xff, 0, 0xc3}, b)
		clear(w)
		require.Equal(t, []byte{0xff, 0, 0xc3}, f["DCID"])
	}
}

func TestNativeQUICHeaderAliasesOwnPrivateLiteralFields(t *testing.T) {
	var events []*ProtocolEvent
	w := rfc9001ClientInitial()
	capture := t20UDPCapture(t, net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2), 40000, 443, w)
	require.NoError(t, ReplayPcap(bytes.NewReader(capture), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "quic", e.Protocol)
	require.Equal(t, true, e.Session["Authentication Verified"])
	fields, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, rfcHex("8394c8f03e515708"), fields["DCID"])
	want := e.DisplayFieldValues()
	require.Equal(t, []any{"8394c8f03e515708"}, want.FieldMap()["quic.dcid"])
	require.Equal(t, []any{""}, want.FieldMap()["quic.scid"], "zero-length SCID is present in a long header")
	require.Equal(t, []any{uint64(1)}, want.FieldMap()["quic.version"])
	require.Equal(t, []any{uint64(0)}, want.FieldMap()["quic.long.packet_type"])
	require.Empty(t, want.FieldMap()["quic.long.packet_type_v2"])
	require.Equal(t, []any{uint64(2)}, want.FieldMap()["quic.packet_number"])
	for _, v := range want.Values {
		switch v.Name {
		case "quic.dcid":
			require.Equal(t, &DisplayFieldRange{Offset: 6, Length: 8}, v.Range)
		case "quic.packet_number":
			require.Nil(t, v.Range, "decoded protected number has no literal ciphertext value span")
		}
	}
	e.Session = map[string]any{"DCID": []byte("forged"), "Version": uint32(2), "Packet Number": uint64(99), "Wire Field Ranges": map[string]any{"DCID": []any{0, 1}}}
	require.Equal(t, want, e.DisplayFieldValues(), "editable labels and wire ranges do not rewrite native observations")
	e.Raw[6] ^= 1
	require.Empty(t, e.DisplayFields()["quic.dcid"], "a literal CID must agree with its retained native octets")
}

func FuzzQUICHeaderCIDRangeAndAbsence(f *testing.F) {
	f.Add([]byte{0xff, 0, 0xc3}, []byte{0xfe, 0x80}, true)
	f.Add([]byte{}, []byte{}, false)
	f.Fuzz(func(t *testing.T, dcid, scid []byte, long bool) {
		if len(dcid) > 20 || len(scid) > 20 {
			return
		}
		h := quicHdr{long: long, version: 1, dcid: bytes.Clone(dcid), scid: bytes.Clone(scid)}
		w := []byte{0x40}
		m := map[string]any{"Header Form": "short", "Version": uint32(0), "DCID": bytes.Clone(dcid), "SCID": []byte(nil)}
		if long {
			w = []byte{0xc0, 0, 0, 0, 1, byte(len(dcid))}
			w = append(w, dcid...)
			w = append(w, byte(len(scid)))
			w = append(w, scid...)
			m["Header Form"], m["Version"], m["SCID"] = "long", uint32(1), bytes.Clone(scid)
		} else {
			w = append(w, dcid...)
		}
		w = append(w, make([]byte, 32)...)
		m["Wire Field Ranges"] = quicHeaderWireRanges(h, w)
		e := &ProtocolEvent{Protocol: "quic", Raw: w, nativeQUICObservation: true, semanticSession: m}
		s := e.DisplayFieldValues()
		require.Equal(t, []any{hex.EncodeToString(dcid)}, s.FieldMap()["quic.dcid"])
		if long {
			require.Equal(t, []any{hex.EncodeToString(scid)}, s.FieldMap()["quic.scid"])
		} else {
			for _, name := range []string{"quic.scid", "quic.scil", "quic.version", "quic.dcil"} {
				require.Empty(t, s.FieldMap()[name], "not carried in a short header")
			}
		}
		for _, v := range s.Values {
			if v.Range != nil {
				require.True(t, v.Range.Offset >= 0 && v.Range.Offset+v.Range.Length <= len(w))
			}
		}
		forged := &ProtocolEvent{Protocol: "quic", Raw: w, Session: m}
		require.Empty(t, forged.DisplayFields()["quic.dcid"], "public maps cannot establish native wire ownership")
	})
}
