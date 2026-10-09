package pcaputil

import (
	"encoding/binary"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func svNativeWitness() []byte {
	fields := append(berPut(0x80, []byte("MVP")), berPut(0x82, []byte{255, 255})...)
	fields = append(fields, berPut(0x83, []byte{255, 255, 255, 255})...)
	fields = append(fields, berPut(0x87, []byte{0, 1, 2, 3})...)
	body := append(berPut(0x80, []byte{1}), berPut(0xa2, berPut(0x30, fields))...)
	w := append([]byte{0x40, 0, 0, 0, 0, 0, 0, 0}, berPut(0x60, body)...)
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	return w
}
func TestSVNativeIngressMissingOptionalSynchronization(t *testing.T) {
	events := replayGOOSECapture(t, oneFrameEthernetCapture(t, 0x88ba, svNativeWitness()))
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "sv", e.Protocol)
	require.Empty(t, e.Error)
	want := map[string]any{"APPID": 16384, "Length": len(svNativeWitness()), "Reserved 1": 0, "Reserved 2": 0, "Simulation": false, "ASDU Count": 1, "ASDUs": []any{map[string]any{"SV ID": []byte("MVP"), "Sample Counter": 65535, "Configuration Revision": uint64(4294967295), "Sample Data": []byte{0, 1, 2, 3}}}, "Link Padding": make([]byte, max(0, 46-len(svNativeWitness())))}
	wb, _ := json.Marshal(want)
	require.NoError(t, json.Unmarshal(wb, &want))
	f, err := e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want, f)
	f["APPID"] = 0
	f, err = e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want, f)
}
