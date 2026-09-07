package stream_parser

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func ldapOperationTestMessage(tag byte, body []byte) []byte {
	return ldapFieldsTestTLV(0x30, append([]byte{2, 1, 1}, ldapFieldsTestTLV(tag, body)...))
}

func ldapOperationTestSearch(filter []byte) []byte {
	body := []byte{4, 0, 10, 1, 2, 10, 1, 0, 2, 1, 0, 2, 1, 0, 1, 1, 0}
	body = append(body, filter...)
	return ldapOperationTestMessage(0x63, append(body, 0x30, 0))
}

func ldapOperationTestFixtures() map[string][]byte {
	return map[string][]byte{
		"bind-response":    ldapOperationTestMessage(0x61, []byte{10, 1, 0, 4, 0, 4, 0, 0x87, 2, 0, 255}),
		"unbind-request":   ldapOperationTestMessage(0x42, nil),
		"search-request":   ldapOperationTestSearch([]byte{0x87, 2, 'c', 'n'}),
		"search-entry":     ldapOperationTestMessage(0x64, []byte{4, 0, 0x30, 12, 0x30, 10, 4, 2, 'c', 'n', 0x31, 4, 4, 0, 4, 0}),
		"search-done":      ldapOperationTestMessage(0x65, []byte{10, 1, 0, 4, 0, 4, 0}),
		"search-reference": ldapOperationTestMessage(0x73, append([]byte{4, 8}, []byte("ldap://x")...)),
	}
}

func TestLDAPOperationLayoutsAndPrefixes(t *testing.T) {
	for profile, wire := range ldapOperationTestFixtures() {
		t.Run(profile, func(t *testing.T) {
			fs, info, err := decodeLDAPFields(wire, profile)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fs, len(wire))
			require.Equal(t, uint64(1), info["Message ID"])
			require.Equal(t, false, info["Matching Rules Applied"])
			for cut := 0; cut < len(wire); cut++ {
				fs, info, err := decodeLDAPFields(wire[:cut], profile)
				require.Error(t, err, "prefix %d", cut)
				require.Nil(t, fs)
				require.Nil(t, info)
			}
			for _, tail := range [][]byte{{0}, wire} {
				_, _, err := decodeLDAPFields(append(bytes.Clone(wire), tail...), profile)
				require.Error(t, err)
			}
			for other := range ldapOperationTestFixtures() {
				if other != profile {
					_, _, err := decodeLDAPFields(wire, other)
					require.Error(t, err)
				}
			}
		})
	}
}

func TestLDAPOperationFilterChoicesAndMalformed(t *testing.T) {
	// Literal BER choices from the RFC 4511 ASN.1 definitions. No encoder
	// shared with the production decoder creates these nested filter values.
	for _, encoded := range []string{
		"8702636e", "a006870163870164", "a103870163", "a203870163",
		"a3070402636e040178", "a5070402636e040178", "a6070402636e040178", "a8070402636e040178",
		"a40f0402636e3009800161810162820163",
		"a9078202636e830178", "a9098103312e32830200ff", "a90a8202636e8301788401ff",
	} {
		filter, err := hex.DecodeString(encoded)
		require.NoError(t, err)
		wire := ldapOperationTestSearch(filter)
		fs, _, err := decodeLDAPFields(wire, "search-request")
		require.NoError(t, err, encoded)
		tlsCertificateTestCoverage(t, fs, len(wire))
	}
	for _, encoded := range []string{
		"a000", "a100", "a200", "a206870163870164", "a700", "860163",
		"a403040178", "a4060402636e3000", "a40c0402636e3006800161800162",
		"a403040178", "a90a8202636e830178840100", "a903830178", "a9098202636e8201638300",
	} {
		filter, err := hex.DecodeString(encoded)
		require.NoError(t, err)
		_, _, err = decodeLDAPFields(ldapOperationTestSearch(filter), "search-request")
		require.Error(t, err, encoded)
	}
	for _, body := range [][]byte{{10, 1, 10, 4, 0, 4, 0}, {10, 1, 0, 4, 0, 4, 0, 0xa3, 0}, {10, 1, 0, 4, 0, 4, 0, 0xa3, 3, 4, 1, 'x'}} {
		_, _, err := decodeLDAPFields(ldapOperationTestMessage(0x65, body), "search-done")
		require.Error(t, err)
	}
	_, _, err := decodeLDAPFields(ldapOperationTestMessage(0x42, []byte{0}), "unbind-request")
	require.Error(t, err)
}

func TestLDAPOperationResourcesAndTransactions(t *testing.T) {
	for _, depth := range []int{64, 65} {
		filter := []byte{0x87, 1, 'x'}
		for i := 0; i < depth; i++ {
			filter = ldapFieldsTestTLV(0xa2, filter)
		}
		_, _, err := decodeLDAPFields(ldapOperationTestSearch(filter), "search-request")
		if depth == 64 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	for _, count := range []int{4095, 4096} {
		filter := ldapFieldsTestTLV(0xa0, bytes.Repeat([]byte{0x87, 1, 'x'}, count))
		_, _, err := decodeLDAPFields(ldapOperationTestSearch(filter), "search-request")
		if count == 4095 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	for _, body := range []string{strings.Repeat("x", 130), "目录"} {
		wire := ldapOperationTestMessage(0x64, append(ldapFieldsTestTLV(4, []byte(body)), 0x30, 0))
		fs, _, err := decodeLDAPFields(wire, "search-entry")
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fs, len(wire))
	}
	profiles := []string{"bind-response", "unbind-request", "search-request", "search-entry", "search-done", "search-reference"}
	testExactByteFieldsBridgeTransactions(t, profiles, func(p string) []byte { return ldapOperationTestFixtures()[p] }, parseLDAPFields)
}
