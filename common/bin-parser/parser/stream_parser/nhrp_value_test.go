package stream_parser

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func nhrpValueTestClient(a, s, p byte) []byte {
	b := []byte{7, 29, 0xa1, 0x23, 5, 0xcd, 1, 0x9d, a, s, p, 0xab}
	for i := 0; i < int(a&63)+int(s&63)+int(p); i++ {
		b = append(b, byte(i*13+7))
	}
	return b
}

func TestNHRPValueClientBoundaries(t *testing.T) {
	for _, lengths := range [][3]byte{{0, 0, 0}, {0x40, 0x40, 0}, {4, 3, 5}, {63, 63, 255}, {127, 127, 255}} {
		body := nhrpValueTestClient(lengths[0], lengths[1], lengths[2])
		records, err := decodeNHRPClientsBody(body)
		require.NoError(t, err)
		require.Equal(t, []nhrpClientRecord{{AddressLengths: [3]int{int(lengths[0] & 63), int(lengths[1] & 63), int(lengths[2])}}}, records)
		for cut := 1; cut < len(body); cut++ {
			r, err := decodeNHRPClientsBody(body[:cut])
			require.Error(t, err)
			require.Nil(t, r)
		}
		for field := 8; field <= 9; field++ {
			invalid := bytes.Clone(body)
			invalid[field] |= 128
			r, err := decodeNHRPClientsBody(invalid)
			require.ErrorContains(t, err, "reserved client address type bit")
			require.Nil(t, r)
		}
	}
	for _, count := range []int{0, 1, 2, 4096, 4097} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			body := bytes.Repeat(nhrpValueTestClient(0, 0, 0), count)
			r, err := decodeNHRPClientsBody(body)
			if count == 4097 {
				require.ErrorContains(t, err, "too many client entries")
				require.Nil(t, r)
				return
			}
			require.NoError(t, err)
			require.Len(t, r, count)
			for i := range r {
				require.Equal(t, i*12, r[i].Offset)
			}
		})
	}
	r, err := decodeNHRPClientsBody(make([]byte, 65536))
	require.Error(t, err)
	require.Nil(t, r)
	// A valid first record must never escape as a partial result on a bad tail.
	for _, tail := range [][]byte{{0}, nhrpValueTestClient(128, 0, 0), nhrpValueTestClient(1, 0, 0)[:12]} {
		r, err := decodeNHRPClientsBody(append(nhrpValueTestClient(4, 3, 5), tail...))
		require.Error(t, err)
		require.Nil(t, r)
	}
}

func TestNHRPValueConcurrentIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := nhrpValueTestClient(byte(i), byte(15-i), byte(i+1))
			before := bytes.Clone(body)
			for n := 0; n < 16; n++ {
				r, err := decodeNHRPClientsBody(body)
				require.NoError(t, err)
				require.Equal(t, [3]int{i, 15 - i, i + 1}, r[0].AddressLengths)
				require.Equal(t, before, body)
			}
		}(i)
	}
	wg.Wait()
}
