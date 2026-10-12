package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func opcuaBrowseAll(t *testing.T) {
	t.Run("Codec", func(t *testing.T) { opcuaSelectedCodecCases(t, "opcua-browse", "Browse", true) })
	t.Run("Sealed", func(t *testing.T) { opcuaSelectedSealed(t, "opcua-browse", "Browse", 40, true) })
	t.Run("Resources", opcuaBrowseResources)
	t.Run("MixedProfiles", opcuaBrowseMixedProfiles)
}
func opcuaBrowseResources(t *testing.T) {
	b, e := opcuaSelectedInput("opcua-browse", "material/cases.json")
	require.NoError(t, e)
	// Use the same complete JSON-number preserving golden loader as the matrix.
	var cases []uaReadCase
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&cases))
	for _, step := range cases[0].Steps[1:] {
		w, e := hex.DecodeString(step.Hex)
		require.NoError(t, e)
		_, n, e := uaService(w[24:])
		require.NoError(t, e)
		body := w[24+n:]
		response := step.Service == 530
		for i := 0; i < len(body); i++ {
			_, e = decodeOPCUABrowse(body[:i], response, 1024, nil)
			require.Error(t, e, "cut%d response%t", i, response)
		}
		var cost int64
		fields, e := decodeOPCUABrowse(body, response, 1024, func(n int64) error { cost = n; return nil })
		require.NoError(t, e)
		requireExactProtocolFields(t, step.Read, fields)
		_, e = decodeOPCUABrowse(body, response, 1024, func(n int64) error {
			if n >= cost {
				return protocolError(ErrResourceExceeded, "one byte below projection cost")
			}
			return nil
		})
		rocTypedError(t, "ResourceExceeded", e)
		_, e = decodeOPCUABrowse(body, response, 1024, func(n int64) error {
			if n > cost {
				return protocolError(ErrResourceExceeded, "exact projection cost")
			}
			return nil
		})
		require.NoError(t, e)
		if response {
			_, e = decodeOPCUABrowse(body, true, 1, nil)
			rocTypedError(t, "ResourceExceeded", e)
			_, e = decodeOPCUABrowse(body, true, 2, nil)
			require.NoError(t, e)
		}
		_, e = decodeOPCUABrowse(body, response, 0, nil)
		rocTypedError(t, "ResourceExceeded", e)
	}
	// Direct decoding retires an exchange rejected by allocation. The public
	// capture API additionally quarantines its flow, covered by the sealed case.
	s := &binOPCUA{browse: true}
	for _, step := range cases[0].Steps[:2] {
		w, err := hex.DecodeString(step.Hex)
		require.NoError(t, err)
		_, err = s.consumeEvent(0, &ProtocolEvent{Raw: w}, 1024, 1<<20)
		require.NoError(t, err)
	}
	require.Len(t, s.appPending, 1)
	response, err := hex.DecodeString(cases[0].Steps[2].Hex)
	require.NoError(t, err)
	s.appReserve = func(int64) error { return protocolError(ErrResourceExceeded, "test Browse response allocation") }
	_, err = s.consumeEvent(1, &ProtocolEvent{Raw: response}, 1024, 1<<20)
	rocTypedError(t, "ResourceExceeded", err)
	require.Empty(t, s.appPending)
	require.Equal(t, map[uaKey]uint32{{0, 6, 1}: 446}, s.pending) // Adjacent observed OPN request is retained.
	require.Empty(t, s.chunks)
	s.appReserve = nil
	later := bytes.Clone(response)
	binary.LittleEndian.PutUint32(later[16:20], 2)
	out, err := s.consumeEvent(1, &ProtocolEvent{Raw: later}, 1024, 1<<20)
	require.NoError(t, err)
	require.Equal(t, false, out["Matched"])
	requireExactProtocolFields(t, cases[0].Steps[2].Read, out["Browse"].(map[string]any))

}

// Shared channel state must not confuse interleaved Read and Browse services.
func opcuaBrowseMixedProfiles(t *testing.T) { opcuaSelectedMixedProfiles(t, "opcua-browse", "Browse") }
func opcuaSelectedMixedProfiles(t *testing.T, root, field string) {
	load := func(root string) []uaReadCase {
		raw, err := opcuaSelectedInput(root, "material/cases.json")
		require.NoError(t, err)
		var cases []uaReadCase
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		require.NoError(t, d.Decode(&cases))
		return cases
	}
	r, b := load("opcua-read-scalar")[0], load(root)[0]
	s := &binOPCUA{read: true, browse: field == "Browse", write: field == "Write", next: field == "BrowseNext", call: field == "Call"}
	wire := func(hexWire string, seq, id uint32) []byte {
		v, err := hex.DecodeString(hexWire)
		require.NoError(t, err)
		if seq != 0 {
			binary.LittleEndian.PutUint32(v[16:20], seq)
		}
		if id != 0 {
			binary.LittleEndian.PutUint32(v[20:24], id)
		}
		return v
	}
	opn := wire(r.Steps[0].Hex, 0, 0)
	_, err := s.consumeEvent(0, &ProtocolEvent{Raw: opn}, 1024, 1<<20)
	require.NoError(t, err)
	_, err = s.consumeEvent(0, &ProtocolEvent{Raw: wire(r.Steps[1].Hex, 0, 0)}, 1024, 1<<20)
	require.NoError(t, err)
	_, err = s.consumeEvent(0, &ProtocolEvent{Raw: wire(b.Steps[1].Hex, 3, 3)}, 1024, 1<<20)
	require.NoError(t, err)
	require.Len(t, s.appPending, 2)
	for _, response := range []struct {
		field    string
		w        []byte
		expected map[string]any
	}{
		{field, wire(b.Steps[2].Hex, 1, 3), b.Steps[2].Read},
		{"Read", wire(r.Steps[2].Hex, 2, 0), r.Steps[2].Read},
	} {
		out, err := s.consumeEvent(1, &ProtocolEvent{Raw: response.w}, 1024, 1<<20)
		require.NoError(t, err)
		require.Equal(t, true, out["Matched"])
		requireExactProtocolFields(t, response.expected, out[response.field].(map[string]any))
	}
	require.Empty(t, s.appPending)
	// Reuse the completed Read ID for Browse, then give it a Read response:
	// service mismatch is refused and must retire its selected association.
	_, err = s.consumeEvent(0, &ProtocolEvent{Raw: wire(b.Steps[1].Hex, 4, 2)}, 1024, 1<<20)
	require.NoError(t, err)
	_, err = s.consumeEvent(1, &ProtocolEvent{Raw: wire(r.Steps[2].Hex, 3, 2)}, 1024, 1<<20)
	rocTypedError(t, "Desynchronized", err)
	require.Empty(t, s.appPending)
}
