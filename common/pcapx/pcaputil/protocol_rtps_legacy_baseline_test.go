package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRTPS20DataExistingAPI(t *testing.T) {
	w := make([]byte, 60)
	copy(w, []byte("RTPS"))
	w[4] = 2
	w[5] = 0
	w[20] = 2
	w[21] = 0x19
	binary.LittleEndian.PutUint16(w[22:], 36)
	copy(w[24:], []byte{0, 0, 1, 7, 0, 0, 2, 2})
	binary.LittleEndian.PutUint32(w[36:], 7)
	copy(w[40:], []byte("abcdefghijklmnop"))
	binary.LittleEndian.PutUint32(w[56:], 3)
	f, err := decodeRTPSDatagram(w, 32)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Protocol Version": "2.0", "Vendor ID": "0000", "Source GUID Prefix": "000000000000000000000000", "Observation": "unverified-data-object", "Changes": []map[string]any{{"Submessage": "DATA", "Flags": byte(0x19), "Reader ID": "00000107", "Writer ID": "00000202", "Writer Sequence High": int32(0), "Writer Sequence Low": uint32(7), "Key Hash Prefix": "6162636465666768696a6b6c", "Key Hash Prefix Known": true, "Key Hash Suffix": "6d6e6f70", "Status Info Present": true, "Status Info": uint32(3), "Disposed": true, "Unregistered": true}}}, f)
}

func TestRTPS20AdjacentSPDPExistingAPI(t *testing.T) {
	for _, c := range rtpsControls(t) {
		_, a := rtpsInput(t, c)
		if len(a.PureErrors) == 1 && a.PureErrors[0] == "" {
			w, err := hex.DecodeString(a.Wires[0])
			require.NoError(t, err)
			f, err := decodeRTPSDatagram(w, 1024)
			require.NoError(t, err)
			rocEqualFields(t, a.Events[0].Fields, f)
			return
		}
	}
	t.Fatal("no adjacent sealed SPDP positive")
}
