package pcaputil

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeQUICEnvelopeBinaryFieldsRemainLosslessAcrossJSON(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		w := []byte{0xc0, 0, 0, 0, 1, 3, 0xff, 0, 0xc3, 2, 0xfe, 0x80, 2, 0xff, 0xfe, 2, 0, 0}
		if v2 {
			copy(w[:5], []byte{0xd0, 0x6b, 0x33, 0x43, 0xcf})
		}
		f, err := quicWireEnvelope(w)
		if v2 {
			require.ErrorContains(t, err, "UnsupportedVersion", "native v2 remains explicitly unselected; a valid common layout is not authenticated v2 support")
			continue
		}
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
