package tls

import (
	"bytes"
	"io"
	"testing"
)

func TestCookieExtensionEncoding(t *testing.T) {
	for _, tc := range []struct {
		name   string
		size   int
		header []byte
	}{
		{"reported HRR cookie", 67, []byte{0, 44, 0, 69, 0, 67}},
		{"two-byte cookie length", 256, []byte{0, 44, 1, 2, 1, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cookie := bytes.Repeat([]byte{0xa5}, tc.size)
			ext := &CookieExtension{Cookie: cookie}
			wire := make([]byte, ext.Len())
			n, err := ext.Read(wire)
			if n != len(wire) || err != io.EOF {
				t.Fatalf("Read returned (%d, %v), want (%d, EOF)", n, err, len(wire))
			}
			if want := append(tc.header, cookie...); !bytes.Equal(wire, want) {
				t.Fatalf("encoded cookie extension = %x, want %x", wire, want)
			}
		})
	}
}
