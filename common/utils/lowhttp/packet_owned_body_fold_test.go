package lowhttp

import (
	"bytes"
	"fmt"
	"testing"
)

func TestReplaceHTTPPacketBodyOwnedFoldsOnlyOwnedCapacity(t *testing.T) {
	header := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1\r\n\r\n")
	payload := bytes.Repeat([]byte(`{"status":"ok"}`), 1024)
	want := ReplaceHTTPPacketBodyEx(header, payload, false, true)

	owned := make([]byte, len(payload), len(payload)+bytes.MinRead)
	copy(owned, payload)
	ownedStart := &owned[0]
	got := replaceHTTPPacketBodyExOwned(header, owned, false, true)
	if !bytes.Equal(got, want) {
		t.Fatal("owned packet fold changed packet bytes")
	}
	if &got[0] != ownedStart {
		t.Fatal("owned packet fold did not reuse the decoded body allocation")
	}
	_, gotBody := SplitHTTPHeadersAndBodyFromPacketView(got)
	if !bytes.Equal(gotBody, payload) {
		t.Fatal("owned packet fold changed body bytes")
	}

	insufficient := make([]byte, len(payload), len(payload))
	copy(insufficient, payload)
	insufficientStart := &insufficient[0]
	got = replaceHTTPPacketBodyExOwned(header, insufficient, false, true)
	if !bytes.Equal(got, want) {
		t.Fatal("owned packet fold fallback changed packet bytes")
	}
	if &got[0] == insufficientStart {
		t.Fatal("owned packet fold reused capacity that could not fit the header")
	}

	borrowed := make([]byte, len(payload), len(payload)+bytes.MinRead)
	copy(borrowed, payload)
	borrowedBefore := bytes.Clone(borrowed)
	got = ReplaceHTTPPacketBodyEx(header, borrowed, false, true)
	if !bytes.Equal(borrowed, borrowedBefore) {
		t.Fatal("public packet replacement consumed a borrowed body")
	}
	borrowed[0] ^= 0xff
	_, gotBody = SplitHTTPHeadersAndBodyFromPacketView(got)
	if !bytes.Equal(gotBody, payload) {
		t.Fatal("public packet replacement result aliases a borrowed body")
	}
}

func TestDecodedPacketFoldPreservesInputAndCacheOwnership(t *testing.T) {
	payload := bytes.Repeat([]byte("decoded-owned-body"), 16*1024)
	compressed := gzipDecodeAllocationFixture(t, payload)
	wire := append([]byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n",
		len(compressed),
	)), compressed...)
	wireBefore := bytes.Clone(wire)

	plain, independentlyOwned := DeletePacketEncodingWithOwnership(wire)
	if !independentlyOwned {
		t.Fatal("decoded packet did not report independent ownership")
	}
	if !bytes.Equal(wire, wireBefore) {
		t.Fatal("decoded packet fold modified the wire input")
	}
	_, plainBody := SplitHTTPHeadersAndBodyFromPacketView(plain)
	if !bytes.Equal(plainBody, payload) {
		t.Fatal("decoded packet fold changed the decoded body")
	}

	fixed, err := FixHTTPResponsePacket(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wire, wireBefore) {
		t.Fatal("fixed packet fold modified the wire input")
	}
	_, fixedBody := SplitHTTPHeadersAndBodyFromPacketView(fixed)
	if !bytes.Equal(fixedBody, payload) {
		t.Fatal("fixed packet fold changed the decoded body")
	}
}

var benchmarkOwnedFoldPacket []byte

func BenchmarkGzipDecodeOwnedPacketFold256K(b *testing.B) {
	payload := bytes.Repeat([]byte(`{"status":"ok","payload":"aaaaaaaaaaaaaaaa"}`), 262144/44+1)
	payload = payload[:262144]
	compressed := gzipDecodeAllocationFixture(b, payload)
	header := []byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n",
		len(compressed),
	))

	for _, tc := range []struct {
		name  string
		owned bool
	}{
		{name: "decoded-body-copy", owned: false},
		{name: "decoded-body-fold", owned: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				decoded, ok := _decodeBody(_contentAlgoGzip, compressed, _autoUnzipMaxDecodedBodyBytes)
				if !ok {
					b.Fatal("gzip decode failed")
				}
				if tc.owned {
					benchmarkOwnedFoldPacket = replaceHTTPPacketBodyExOwned(header, decoded, false, true)
				} else {
					benchmarkOwnedFoldPacket = ReplaceHTTPPacketBodyEx(header, decoded, false, true)
				}
			}
		})
	}
}
