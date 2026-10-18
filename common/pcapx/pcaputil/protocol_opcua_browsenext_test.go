package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func opcuaBrowseNextAll(t *testing.T) {
	t.Run("Codec", func(t *testing.T) { opcuaSelectedCodecCases(t, "opcua-browsenext", "BrowseNext", false) })
	t.Run("Sealed", func(t *testing.T) { opcuaSelectedSealed(t, "opcua-browsenext", "BrowseNext", 39, false) })
	t.Run("Resources", opcuaBrowseNextResources)
	t.Run("MixedProfiles", func(t *testing.T) { opcuaSelectedMixedProfiles(t, "opcua-browsenext", "BrowseNext") })
}

func opcuaBrowseNextResources(t *testing.T) {
	raw, err := opcuaSelectedInput("opcua-browsenext", "material/cases.json")
	require.NoError(t, err)
	var cases []uaReadCase
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&cases))
	for _, step := range cases[0].Steps[1:] {
		w, err := hex.DecodeString(step.Hex)
		require.NoError(t, err)
		_, n, err := uaService(w[24:])
		require.NoError(t, err)
		body := w[24+n:]
		response := step.Service == 536
		for cut := 0; cut < len(body); cut++ {
			_, err = decodeOPCUABrowseNext(body[:cut], response, 1024, nil)
			require.Error(t, err, "cut%d response%t", cut, response)
		}
		var cost int64
		out, err := decodeOPCUABrowseNext(body, response, 1024, func(n int64) error { cost = n; return nil })
		require.NoError(t, err)
		requireExactProtocolFields(t, step.Read, out)
		for _, limit := range []int64{cost - 1, cost} {
			_, err = decodeOPCUABrowseNext(body, response, 1024, func(n int64) error {
				if n > limit {
					return protocolError(ErrResourceExceeded, "BrowseNext allocation boundary")
				}
				return nil
			})
			if limit < cost {
				rocTypedError(t, "ResourceExceeded", err)
			} else {
				require.NoError(t, err)
			}
		}
		_, err = decodeOPCUABrowseNext(body, response, 0, nil)
		rocTypedError(t, "ResourceExceeded", err)
	}
	s := &binOPCUA{next: true}
	for _, step := range cases[0].Steps[:2] {
		w, err := hex.DecodeString(step.Hex)
		require.NoError(t, err)
		_, err = s.consumeEvent(0, &ProtocolEvent{Raw: w}, 1024, 1<<20)
		require.NoError(t, err)
	}
	require.Len(t, s.appPending, 1)
	response, err := hex.DecodeString(cases[0].Steps[2].Hex)
	require.NoError(t, err)
	s.appReserve = func(int64) error { return protocolError(ErrResourceExceeded, "BrowseNext response refusal") }
	_, err = s.consumeEvent(1, &ProtocolEvent{Raw: response}, 1024, 1<<20)
	rocTypedError(t, "ResourceExceeded", err)
	require.Empty(t, s.appPending)
	require.Equal(t, map[uaKey]uint32{{0, 6, 1}: 446}, s.pending)
	s.appReserve = nil
	later := bytes.Clone(response)
	binary.LittleEndian.PutUint32(later[16:20], 2)
	out, err := s.consumeEvent(1, &ProtocolEvent{Raw: later}, 1024, 1<<20)
	require.NoError(t, err)
	require.Equal(t, false, out["Matched"])
	requireExactProtocolFields(t, cases[0].Steps[2].Read, out["BrowseNext"].(map[string]any))
}
