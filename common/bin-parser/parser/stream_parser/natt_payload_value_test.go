package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func nattNativePayload(next, flags byte, body []byte) []byte {
	b := append([]byte{next, flags, 0, 0}, body...)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	return b
}

type nattNativeCase struct {
	name       string
	major, typ int
	body       []byte
	spans      map[int][2]int
	layout     string
	inner      int
}

func nattNativeCases() []nattNativeCase {
	return []nattNativeCase{
		{"opaque-critical", 2, 250, nattNativePayload(0, 0xff, []byte{0xab, 0xcd}), map[int][2]int{nattPayloadData: {4, 6}}, "opaque", -1},
		{"opaque-empty", 2, 250, nattNativePayload(0, 0x7f, nil), nil, "opaque", -1},
		{"v1-type-is-not-v2-sk", 1, 46, nattNativePayload(0, 0xff, []byte{7}), map[int][2]int{nattPayloadData: {4, 5}}, "opaque", -1},
		{"v2-notification", 2, 41, nattNativePayload(0, 0x7f, []byte{3, 2, 0x40, 1, 0xaa, 0xbb, 0xcc}), map[int][2]int{nattProtocolID: {4, 5}, nattSPISize: {5, 6}, nattNotifyType: {6, 8}, nattNotificationSPI: {8, 10}, nattNotificationData: {10, 11}}, "notification", -1},
		{"v1-notification", 1, 11, nattNativePayload(0, 0xff, []byte{0, 0, 0, 1, 3, 2, 0x40, 1, 0xaa, 0xbb, 0xcc}), map[int][2]int{nattDomainOfInterpretation: {4, 8}, nattProtocolID: {8, 9}, nattSPISize: {9, 10}, nattNotifyType: {10, 12}, nattNotificationSPI: {12, 14}, nattNotificationData: {14, 15}}, "notification", -1},
		{"notification-no-spi-or-data", 2, 41, nattNativePayload(0, 0, []byte{0, 0, 0, 7}), map[int][2]int{nattProtocolID: {4, 5}, nattSPISize: {5, 6}, nattNotifyType: {6, 8}}, "notification", -1},
		{"key-exchange", 2, 34, nattNativePayload(0, 0x7f, []byte{0, 14, 0xab, 0xcd, 0xef}), map[int][2]int{nattDHGroup: {4, 6}, nattKEReserved: {6, 8}, nattKeyExchangeData: {8, 9}}, "key-exchange", -1},
		{"nonce-minimum", 2, 40, nattNativePayload(0, 0, bytes.Repeat([]byte{0xaa}, 16)), map[int][2]int{nattNonceData: {4, 20}}, "nonce", -1},
		{"nonce-maximum", 2, 40, nattNativePayload(0, 0, bytes.Repeat([]byte{0xaa}, 256)), map[int][2]int{nattNonceData: {4, 260}}, "nonce", -1},
		{"v1-nonce-empty", 1, 10, nattNativePayload(0, 0, nil), nil, "nonce", -1},
		{"v1-nonce", 1, 10, nattNativePayload(0, 0, []byte{5, 6}), map[int][2]int{nattNonceData: {4, 6}}, "nonce", -1},
		{"v2-vendor", 2, 43, nattNativePayload(0, 0, []byte{5, 6}), map[int][2]int{nattVendorID: {4, 6}}, "vendor-id", -1},
		{"v1-vendor-empty", 1, 13, nattNativePayload(0, 0, nil), nil, "vendor-id", -1},
		{"encrypted-sk", 2, 46, nattNativePayload(40, 0, []byte{0xaa}), map[int][2]int{nattEncryptedPayloadData: {4, 5}}, "encrypted", 40},
		{"fragment-first", 2, 53, nattNativePayload(40, 0, []byte{0, 1, 0, 2, 0xaa}), map[int][2]int{nattFragmentNumber: {4, 6}, nattTotalFragments: {6, 8}, nattEncryptedPayloadData: {8, 9}}, "encrypted", 40},
		{"fragment-first-empty-inner", 2, 53, nattNativePayload(0, 0, []byte{0, 1, 0, 2, 0xaa}), map[int][2]int{nattFragmentNumber: {4, 6}, nattTotalFragments: {6, 8}, nattEncryptedPayloadData: {8, 9}}, "encrypted", 0},
		{"fragment-later", 2, 53, nattNativePayload(0, 0, []byte{0, 2, 0, 2, 0xaa}), map[int][2]int{nattFragmentNumber: {4, 6}, nattTotalFragments: {6, 8}, nattEncryptedPayloadData: {8, 9}}, "encrypted", 0},
	}
}

func TestNATTPayloadValueBoundaries(t *testing.T) {
	for _, tc := range nattNativeCases() {
		t.Run(tc.name, func(t *testing.T) {
			before := bytes.Clone(tc.body)
			r, err := decodeNATTPayloads(tc.body, tc.major, tc.typ)
			require.NoError(t, err)
			require.Len(t, r, 1)
			require.Equal(t, 0, r[0].Offset)
			require.Equal(t, len(tc.body), r[0].Length)
			require.Equal(t, tc.typ, r[0].Type)
			require.Equal(t, int(tc.body[0]), r[0].Next)
			require.Equal(t, int(tc.body[1]), r[0].Flags)
			require.Equal(t, before, tc.body)
			for cut := 0; cut < len(tc.body); cut++ {
				r, err := decodeNATTPayloads(tc.body[:cut], tc.major, tc.typ)
				require.Error(t, err, "cut %d", cut)
				require.Nil(t, r)
			}
		})
	}
	for _, tc := range []struct {
		name       string
		major, typ int
		body       []byte
		message    string
	}{
		{"major", 3, 250, []byte{0, 0, 0, 4}, "major version"},
		{"negative-type", 2, -1, []byte{0, 0, 0, 4}, "first type"},
		{"oversize-type", 2, 256, []byte{0, 0, 0, 4}, "first type"},
		{"oversize-body", 2, 250, make([]byte, 65528), "boundary"},
		{"zero-type-with-bytes", 1, 0, []byte{0}, "bytes remain"},
		{"length-below-four", 2, 250, []byte{0, 0, 0, 3}, "below four"},
		{"missing-next", 2, 250, []byte{250, 0, 0, 4}, "ends before"},
		{"v2-tail", 2, 250, []byte{0, 0, 0, 5, 1, 2, 3, 4}, "bytes remain"},
		{"v1-unaligned-tail", 1, 250, []byte{0, 0, 0, 5, 1, 2}, "bytes remain"},
		{"v1-aligned-tail-four", 1, 250, []byte{0, 0, 0, 4, 1, 2, 3, 4}, "bytes remain"},
		{"v2-nonce-empty", 2, 40, nattNativePayload(0, 0, nil), "16..256"},
		{"v2-nonce-short", 2, 40, nattNativePayload(0, 0, make([]byte, 15)), "16..256"},
		{"v2-nonce-long", 2, 40, nattNativePayload(0, 0, make([]byte, 257)), "16..256"},
		{"v2-notify-short", 2, 41, nattNativePayload(0, 0, make([]byte, 3)), "notification fields"},
		{"v1-notify-short", 1, 11, nattNativePayload(0, 0, make([]byte, 7)), "notification fields"},
		{"v2-notify-spi-short", 2, 41, nattNativePayload(0, 0, []byte{3, 1, 0, 0}), "SPI exceeds"},
		{"v1-notify-spi-short", 1, 11, nattNativePayload(0, 0, []byte{0, 0, 0, 1, 3, 1, 0, 0}), "SPI exceeds"},
		{"ke-empty", 2, 34, nattNativePayload(0, 0, make([]byte, 4)), "key exchange"},
		{"sk-empty", 2, 46, nattNativePayload(0, 0, nil), "payload is empty"},
		{"sk-extra-outer", 2, 46, append(nattNativePayload(250, 0, []byte{1}), 0, 0, 0, 4), "bytes remain"},
		{"fragment-empty", 2, 53, nattNativePayload(0, 0, []byte{0, 1, 0, 2}), "nonempty data"},
		{"fragment-zero", 2, 53, nattNativePayload(0, 0, []byte{0, 0, 0, 2, 1}), "fragment counters"},
		{"total-zero", 2, 53, nattNativePayload(0, 0, []byte{0, 1, 0, 0, 1}), "fragment counters"},
		{"fragment-over-total", 2, 53, nattNativePayload(0, 0, []byte{0, 3, 0, 2, 1}), "fragment counters"},
		{"later-fragment-next", 2, 53, nattNativePayload(40, 0, []byte{0, 2, 0, 2, 1}), "next payload zero"},
		{"later-fragment-prefix", 2, 250, append(nattNativePayload(53, 0, nil), nattNativePayload(0, 0, []byte{0, 2, 0, 2, 1})...), "preceding payloads"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := decodeNATTPayloads(tc.body, tc.major, tc.typ)
			require.ErrorContains(t, err, tc.message)
			require.Nil(t, r, "never publish valid prefixes")
		})
	}
	for padding := 1; padding <= 3; padding++ {
		body := append(nattNativePayload(0, 0, make([]byte, 4-padding)), bytes.Repeat([]byte{0xab}, padding)...)
		r, err := decodeNATTPayloads(body, 1, 250)
		require.NoError(t, err)
		require.Len(t, r, 1)
		require.Equal(t, len(body)-padding, r[0].Length)
	}
	firstFragment := append(nattNativePayload(53, 0, nil), nattNativePayload(40, 0, []byte{0, 1, 0, 2, 1})...)
	r, err := decodeNATTPayloads(firstFragment, 2, 250)
	require.NoError(t, err)
	require.Len(t, r, 2)
	r, err = decodeNATTPayloads(nil, 2, 0)
	require.NoError(t, err)
	require.Empty(t, r)
}

func nattNativeChain(count int) []byte {
	body := bytes.Repeat([]byte{250, 0xff, 0, 4}, count)
	if count > 0 {
		body[len(body)-4] = 0
	}
	return body
}

func TestNATTPayloadValueResourceBound(t *testing.T) {
	for _, count := range []int{1, 16, 4096, 4097} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			r, err := decodeNATTPayloads(nattNativeChain(count), 2, 250)
			if count > 4096 {
				require.ErrorContains(t, err, "count exceeds 4096")
				require.Nil(t, r)
				return
			}
			require.NoError(t, err)
			require.Len(t, r, count)
			for i := range r {
				require.Equal(t, i*4, r[i].Offset)
			}
		})
	}
	maximum := nattNativePayload(0, 0, make([]byte, 65523))
	r, err := decodeNATTPayloads(maximum, 2, 250)
	require.NoError(t, err)
	require.Len(t, r, 1)
	require.Equal(t, 65527, r[0].Length)
}

func BenchmarkNATTPayloadValue(b *testing.B) {
	for _, count := range []int{16, 256, 4096} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			body := nattNativeChain(count)
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := decodeNATTPayloads(body, 2, 250); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
