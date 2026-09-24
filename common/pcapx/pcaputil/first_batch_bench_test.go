package pcaputil

import (
	"bytes"
	"testing"
)

func BenchmarkFirstBatch(b *testing.B) {
	b.Run("dns-name", func(b *testing.B) {
		wire := []byte{3, 'w', 'w', 'w', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			name, _, err := dnsParseName(wire, 0)
			if err != nil || name != "www.example.com" {
				b.Fatal(name, err)
			}
		}
	})
	b.Run("pcapng-replay", func(b *testing.B) {
		raw := binTestPcap(b, []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"}, {seq: 138, fin: true}}, 80, false, true)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := ReplayPcap(bytes.NewReader(raw)); err != nil {
				b.Fatal(err)
			}
		}
	})
}
