package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func nbnsTestQuery() []byte {
	w := make([]byte, 12)
	binary.BigEndian.PutUint16(w, 0x1234)
	binary.BigEndian.PutUint16(w[4:], 1)
	name := append([]byte("WORKSTATION    "), byte(0x20))
	w = append(w, 32)
	for _, b := range name {
		w = append(w, 'A'+b>>4, 'A'+b&15)
	}
	return append(w, 0, 0, 0x20, 0, 1)
}

func TestNBNSStrictDatagramBoundaries(t *testing.T) {
	q := nbnsTestQuery()
	fields, err := decodeNBNS(q, 64)
	require.NoError(t, err)
	questions := fields["Questions"].([]map[string]any)
	require.Equal(t, "WORKSTATION", questions[0]["Name"])
	require.Equal(t, byte(0x20), questions[0]["Suffix"])
	for i := 0; i < len(q); i++ {
		_, err := decodeNBNS(q[:i], 64)
		require.Error(t, err, "cut %d", i)
	}
	for _, bad := range [][]byte{append(bytes.Clone(q), 0), bytes.Repeat([]byte{0}, 12)} {
		_, err := decodeNBNS(bad, 64)
		require.Error(t, err)
	}
	bad := bytes.Clone(q)
	bad[13] = 'Z'
	_, err = decodeNBNS(bad, 64)
	require.Error(t, err)
	bad = bytes.Clone(q)
	bad[len(bad)-3] = 1
	_, err = decodeNBNS(bad, 64)
	require.Error(t, err)
	// NBSTAT type 33 has a node-name/statistics layout, not DNS SRV RDATA.
	r := bytes.Clone(q)
	binary.BigEndian.PutUint16(r[2:], 0x8500)
	binary.BigEndian.PutUint16(r[6:], 1)
	r = append(r, 0xc0, 12, 0, 0x21, 0, 1, 0, 0, 0, 0, 0, 47)
	r = append(r, make([]byte, 47)...)
	fields, err = decodeNBNS(r, 64)
	require.NoError(t, err)
	answer := fields["Answers"].([]map[string]any)[0]
	require.Contains(t, answer, "Node Names")
	require.NotContains(t, answer, "SRV")
	populated := bytes.Clone(r[:len(q)+12])
	binary.BigEndian.PutUint16(populated[len(populated)-2:], 65)
	data := make([]byte, 65)
	data[0] = 1
	copy(data[1:16], []byte("WORKSTATION    "))
	data[16] = 0x20
	binary.BigEndian.PutUint16(data[17:19], 0x400)
	populated = append(populated, data...)
	decoded, err := decodeNBNS(populated, 64)
	require.NoError(t, err)
	entry := decoded["Answers"].([]map[string]any)[0]["Node Names"].([]map[string]any)[0]
	require.Equal(t, byte(0x20), entry["Suffix"])
	require.Equal(t, uint16(0x400), entry["Flags"])
	for i := len(q); i < len(r); i++ {
		_, err := decodeNBNS(r[:i], 64)
		require.Error(t, err, "response cut %d", i)
	}
}

func FuzzNBNSDatagram(f *testing.F) {
	f.Add(nbnsTestQuery())
	f.Add([]byte{0xc0, 0})
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 65535 {
			return
		}
		decodeNBNS(w, 64)
	})
}
