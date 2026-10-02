package stream_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func memcachedFieldsTestFixtures() map[string][]byte {
	return map[string][]byte{
		"stats-request":      []byte("stats\r\n"),
		"stats-response":     []byte("STAT pid 123\r\nSTAT same 001\r\nSTAT same text value\r\nSTAT version 1.4.15\r\nSTAT rusage_user 0.005382\r\nEND\r\n"),
		"binary-get-request": append([]byte{0x80, 0, 0, 3, 0, 0, 0x12, 0x34, 0, 0, 0, 3, 1, 2, 3, 4, 1, 2, 3, 4, 5, 6, 7, 8}, []byte{0, 0xff, ' '}...),
	}
}
func TestMemcachedFieldsLayoutsPrefixesAndIsolation(t *testing.T) {
	for profile, w := range memcachedFieldsTestFixtures() {
		fs, m, e := decodeMemcachedFields(w, profile)
		require.NoError(t, e)
		tlsCertificateTestCoverage(t, fs, len(w))
		require.Equal(t, false, m["Command Executed"])
		require.Equal(t, false, m["TCP Reassembly Performed"])
		for cut := 0; cut < len(w); cut++ {
			fs, m, e := decodeMemcachedFields(w[:cut], profile)
			require.Error(t, e, "profile %s cut %d", profile, cut)
			require.Nil(t, fs)
			require.Nil(t, m)
		}
		for other := range memcachedFieldsTestFixtures() {
			if other != profile {
				_, _, e := decodeMemcachedFields(w, other)
				require.Error(t, e)
			}
		}
		for _, tail := range [][]byte{{0}, w} {
			_, _, e := decodeMemcachedFields(append(bytes.Clone(w), tail...), profile)
			require.Error(t, e)
		}
		for at := range w {
			for _, v := range []byte{0, 10, 13, 32, 127, 255} {
				bad := bytes.Clone(w)
				bad[at] = v
				fs, m, e := decodeMemcachedFields(bad, profile)
				if e != nil {
					require.Nil(t, fs)
					require.Nil(t, m)
				} else {
					tlsCertificateTestCoverage(t, fs, len(w))
				}
			}
		}
	}
	w := memcachedFieldsTestFixtures()["stats-response"]
	_, m, e := decodeMemcachedFields(w, "stats-response")
	require.NoError(t, e)
	stats := m["Statistics"].([]map[string]any)
	require.Len(t, stats, 5)
	require.Equal(t, uint64(123), stats[0]["Value"].(map[string]any)["Unsigned Decimal"])
	require.Equal(t, uint64(1), stats[1]["Value"].(map[string]any)["Unsigned Decimal"])
	require.Equal(t, "001", stats[1]["Value"].(map[string]any)["Text"])
	require.Equal(t, "same", stats[2]["Name"].(map[string]any)["Text"])
	require.Equal(t, "text value", stats[2]["Value"].(map[string]any)["Text"])
	require.NotContains(t, stats[3]["Value"], "Unsigned Decimal")
	require.NotContains(t, stats[4]["Value"], "Unsigned Decimal")
	clear(w)
	require.Equal(t, []byte("123"), stats[0]["Value"].(map[string]any)["Bytes"])
	binaryWire := memcachedFieldsTestFixtures()["binary-get-request"]
	_, m, e = decodeMemcachedFields(binaryWire, "binary-get-request")
	require.NoError(t, e)
	clear(binaryWire)
	require.Equal(t, []byte{0, 255, 32}, m["Key"].(map[string]any)["Bytes"])
}
func TestMemcachedFieldsBoundariesAndResources(t *testing.T) {
	for _, w := range [][]byte{[]byte("END\r\n"), []byte("STAT x 18446744073709551615\r\nEND\r\n"), []byte("STAT x 18446744073709551616\r\nEND\r\n")} {
		fs, m, e := decodeMemcachedFields(w, "stats-response")
		require.NoError(t, e)
		tlsCertificateTestCoverage(t, fs, len(w))
		if bytes.Contains(w, []byte("51615")) {
			require.Equal(t, ^uint64(0), m["Statistics"].([]map[string]any)[0]["Value"].(map[string]any)["Unsigned Decimal"])
		}
		if bytes.Contains(w, []byte("51616")) {
			require.NotContains(t, m["Statistics"].([]map[string]any)[0]["Value"], "Unsigned Decimal")
		}
	}
	for _, w := range []string{"END\n", "STAT x 1\r\n", "STAT  1\r\nEND\r\n", "STAT x \r\nEND\r\n", "STAT x a\nb\r\nEND\r\n", "STAT x a\rb\r\nEND\r\n", "STAT x 名\r\nEND\r\n", "END\r\nSTAT x 1\r\nEND\r\n", "ERROR\r\n"} {
		fs, m, e := decodeMemcachedFields([]byte(w), "stats-response")
		require.Error(t, e)
		require.Nil(t, fs)
		require.Nil(t, m)
	}
	for _, n := range []int{4096, 4097} {
		w := append(bytes.Repeat([]byte("STAT x 1\r\n"), n), []byte("END\r\n")...)
		fs, m, e := decodeMemcachedFields(w, "stats-response")
		if n == 4096 {
			require.NoError(t, e)
			tlsCertificateTestCoverage(t, fs, len(w))
			require.Equal(t, uint64(n), m["Statistic Count"])
		} else {
			require.Error(t, e)
			require.Nil(t, fs)
			require.Nil(t, m)
		}
	}
	for _, n := range []int{memcachedFieldsMaxBytes, memcachedFieldsMaxBytes + 1} {
		w := append([]byte("STAT x "), bytes.Repeat([]byte{'a'}, n-14)...)
		w = append(w, []byte("\r\nEND\r\n")...)
		require.Len(t, w, n)
		_, _, e := decodeMemcachedFields(w, "stats-response")
		if n == memcachedFieldsMaxBytes {
			require.NoError(t, e)
		} else {
			require.Error(t, e)
		}
	}
	for _, n := range []int{0, 1, 250, 251, 65535} {
		w := make([]byte, 24+n)
		w[0] = 0x80
		binary.BigEndian.PutUint16(w[2:], uint16(n))
		binary.BigEndian.PutUint32(w[8:], uint32(n))
		fs, _, e := decodeMemcachedFields(w, "binary-get-request")
		if n == 1 || n == 250 {
			require.NoError(t, e)
			tlsCertificateTestCoverage(t, fs, len(w))
		} else {
			require.Error(t, e)
		}
	}
	for _, mutation := range []struct {
		at    int
		value byte
	}{{0, 0x81}, {1, 1}, {4, 1}, {5, 1}, {3, 2}, {3, 4}, {8, 255}, {11, 2}, {11, 4}} {
		w := memcachedFieldsTestFixtures()["binary-get-request"]
		w[mutation.at] = mutation.value
		_, _, e := decodeMemcachedFields(w, "binary-get-request")
		require.Error(t, e)
	}
	_, _, e := decodeMemcachedFields([]byte("stats\r\n"), "unknown")
	require.Error(t, e)
}
func TestMemcachedFieldsBridgeTransactions(t *testing.T) {
	testExactByteFieldsBridgeTransactions(t, []string{"stats-request", "stats-response", "binary-get-request"}, func(p string) []byte { return memcachedFieldsTestFixtures()[p] }, parseMemcachedFields)
}
