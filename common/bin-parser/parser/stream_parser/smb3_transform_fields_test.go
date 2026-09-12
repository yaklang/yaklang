package stream_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func smb3TransformTestWire() []byte {
	w := make([]byte, 116)
	copy(w, []byte{0xfd, 'S', 'M', 'B'})
	binary.LittleEndian.PutUint32(w[36:], 64)
	w[42] = 1
	w[44] = 7
	for i := 52; i < len(w); i++ {
		w[i] = byte(i)
	}
	return w
}

func TestSMB3TransformFieldsBoundariesAndTransactions(t *testing.T) {
	w := smb3TransformTestWire()
	fs, m, err := decodeSMB3TransformFields(w)
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fs, len(w))
	require.Equal(t, false, m["Decrypted"])
	for cut := 0; cut < len(w); cut++ {
		_, _, err := decodeSMB3TransformFields(w[:cut])
		require.Error(t, err)
	}
	for _, at := range []int{0, 36, 42} {
		bad := bytes.Clone(w)
		bad[at] ^= 255
		_, _, err := decodeSMB3TransformFields(bad)
		require.Error(t, err)
	}
	_, _, err = decodeSMB3TransformFields(append(bytes.Clone(w), 0))
	require.Error(t, err)
	// Reserved bytes are ignored by receivers, and no cipher can be inferred
	// for 3.1.1 from this header. Preserve them without sender-only rejection.
	w[40] = 255
	w[35] = 255
	_, _, err = decodeSMB3TransformFields(w)
	require.NoError(t, err)
	testExactByteFieldsBridgeTransactions(t, []string{"transform"}, func(string) []byte { return smb3TransformTestWire() }, func(n *base.Node, p func(*base.Node) (func(bool), error), _ string) error {
		return parseSMB3TransformFields(n, p)
	})
}
