package parser

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

func cifsSetupSource(response bool, blob []byte, texts [2]string, tail []byte) []byte {
	n := 63
	if response {
		n = 47
	}
	w := make([]byte, n)
	copy(w[4:8], []byte{0xff, 'S', 'M', 'B'})
	w[8], w[36], w[37] = 0x73, 12, 0xff
	binary.LittleEndian.PutUint16(w[14:16], 0xc800)
	binary.LittleEndian.PutUint16(w[16:18], 0x1234)
	binary.LittleEndian.PutUint16(w[30:32], 0x5678)
	binary.LittleEndian.PutUint16(w[34:36], 2)
	if response {
		w[13], w[36] = 0x80, 4
		binary.LittleEndian.PutUint16(w[39:41], 412) // ignored when terminal
		binary.LittleEndian.PutUint16(w[43:45], uint16(len(blob)))
	} else {
		binary.LittleEndian.PutUint16(w[41:43], 12288)
		binary.LittleEndian.PutUint16(w[43:45], 10)
		binary.LittleEndian.PutUint16(w[51:53], uint16(len(blob)))
		binary.LittleEndian.PutUint32(w[57:61], 0x80000000)
	}
	w = append(w, blob...)
	if len(w)%2 != 0 {
		w = append(w, 0xa5)
	} // ignored pad is not a magic-zero test
	for _, text := range texts {
		for _, unit := range append(utf16.Encode([]rune(text)), 0) {
			w = append(w, byte(unit), byte(unit>>8))
		}
	}
	w = append(w, tail...)
	binary.LittleEndian.PutUint16(w[n-2:n], uint16(len(w)-n))
	w[1], w[2], w[3] = byte((len(w)-4)>>16), byte((len(w)-4)>>8), byte(len(w)-4)
	return w
}

func cifsSetupEntry(response bool) string {
	if response {
		return "CIFSDirectTCPSessionSetupExtendedResponse"
	}
	return "CIFSDirectTCPSessionSetupExtendedRequest"
}

func TestCIFSSessionSetupOwnedTypedProjection(t *testing.T) {
	for _, response := range []bool{false, true} {
		for _, empty := range []bool{false, true} {
			t.Run(cifsSetupEntry(response)+map[bool]string{true: "/empty", false: "/strings"}[empty], func(t *testing.T) {
				blob, texts, tail := []byte{0x11, 0x22, 0x33}, [2]string{"Windows 🔒", "source server"}, []byte{0x91, 0x92}
				if empty {
					blob, texts, tail = []byte{}, [2]string{}, []byte{0x91}
				}
				w := cifsSetupSource(response, blob, texts, tail)
				r := &structuredReader{Reader: bytes.NewReader(w), bits: uint64(len(w)) * 8}
				node, err := ParseBinary(r, "application-layer.cifs", cifsSetupEntry(response))
				require.NoError(t, err)
				require.Zero(t, r.Len())
				fields, err := ProjectCIFSSessionSetupFields(node, w)
				require.NoError(t, err)
				body := fields["Body"].(map[string]any)
				require.Equal(t, blob, body["Security Blob"])
				require.Equal(t, texts[0], body["Native OS"])
				require.Equal(t, texts[1], body["Native LAN Manager"])
				require.Equal(t, tail, body["Trailing Extension Bytes"])
				require.Equal(t, uint16(0x1234), fields["Header"].(map[string]any)["PID High"])
				require.Equal(t, true, fields["AndX Offset Ignored"])
				for _, key := range []string{"Authentication Verified", "Signature Verified", "Session Established"} {
					require.Equal(t, false, fields[key])
				}
				structured, err := ParseStructured(w, "application-layer.cifs", cifsSetupEntry(response))
				require.NoError(t, err)
				require.Equal(t, fields, structured["fields"])
				if len(w) > 50 {
					w[50] ^= 1
				}
				_, err = ProjectCIFSSessionSetupFields(node, w)
				require.Error(t, err, "a mutable Node cannot be projected against changed source")
				clear(w)
				require.Equal(t, blob, body["Security Blob"])
				require.Equal(t, tail, body["Trailing Extension Bytes"])
			})
		}
	}
}

func TestCIFSSessionSetupStrictCompleteUnicodeBoundary(t *testing.T) {
	for _, response := range []bool{false, true} {
		w := cifsSetupSource(response, []byte{1}, [2]string{"A", "B"}, nil)
		for end := 0; end < len(w); end++ {
			_, err := ParseStructured(w[:end], "application-layer.cifs", cifsSetupEntry(response))
			require.Error(t, err, "truncation %d/%d response=%v", end, len(w), response)
		}
		_, err := ParseStructured(w, "application-layer.cifs", cifsSetupEntry(response))
		require.NoError(t, err, "41 00 00 00 must terminate at a code-unit boundary")
		for _, mutation := range []func([]byte){
			func(b []byte) { b[37] = 0x75 },
			func(b []byte) { b[15] &^= 8 },
			func(b []byte) { b[15] &^= 0x80 },
			func(b []byte) { b[36]++ },
			func(b []byte) { b[3]++ },
			func(b []byte) {
				base := 63
				if response {
					base = 47
				}
				base++ // one blob byte, followed by aligned UTF-16 content
				binary.LittleEndian.PutUint16(b[base:base+2], 0xd800)
			},
		} {
			bad := bytes.Clone(w)
			mutation(bad)
			_, err = ParseStructured(bad, "application-layer.cifs", cifsSetupEntry(response))
			require.Error(t, err)
		}
	}
}
