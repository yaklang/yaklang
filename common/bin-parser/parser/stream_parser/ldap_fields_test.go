package stream_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func ldapFieldsTestTLV(tag byte, body []byte) []byte {
	header := []byte{tag}
	size := len(body)
	if size < 128 {
		header = append(header, byte(size))
	} else {
		var encoded []byte
		for size > 0 {
			encoded = append([]byte{byte(size)}, encoded...)
			size >>= 8
		}
		header = append(header, byte(128+len(encoded)))
		header = append(header, encoded...)
	}
	return append(header, body...)
}
func ldapFieldsTestBind(id, name, choice, controls []byte) []byte {
	body := append([]byte{2, 1, 3}, ldapFieldsTestTLV(4, name)...)
	body = append(body, choice...)
	message := append(ldapFieldsTestTLV(2, id), ldapFieldsTestTLV(0x60, body)...)
	if controls != nil {
		message = append(message, ldapFieldsTestTLV(0xa0, controls)...)
	}
	return ldapFieldsTestTLV(0x30, message)
}
func ldapFieldsTestFixtures() [][]byte {
	simple := func(b []byte) []byte { return ldapFieldsTestTLV(0x80, b) }
	sasl := func(name, data []byte) []byte {
		body := ldapFieldsTestTLV(4, name)
		if data != nil {
			body = append(body, ldapFieldsTestTLV(4, data)...)
		}
		return ldapFieldsTestTLV(0xa3, body)
	}
	control := ldapFieldsTestTLV(0x30, append(ldapFieldsTestTLV(4, []byte("1.2.840.113556.1.4.319")), 1, 1, 255, 4, 0))
	return [][]byte{
		ldapFieldsTestBind([]byte{1}, nil, simple(nil), nil),
		ldapFieldsTestBind([]byte{0x7f, 255, 255, 255}, []byte("cn=例子,dc=example"), simple([]byte{0, 255, 128}), control),
		ldapFieldsTestBind([]byte{0, 128}, bytes.Repeat([]byte{'x'}, 130), simple(bytes.Repeat([]byte{255}, 140)), []byte{}),
		ldapFieldsTestBind([]byte{1}, nil, sasl(nil, nil), nil), // empty mechanism can request negotiation abort
		ldapFieldsTestBind([]byte{1}, nil, sasl([]byte("EXAMPLE"), []byte{}), nil),
		ldapFieldsTestBind([]byte{1}, nil, sasl([]byte("EXAMPLE"), []byte{0, 255}), control),
	}
}
func TestLDAPFieldsLayoutsPrefixesAndMutations(t *testing.T) {
	for _, wire := range ldapFieldsTestFixtures() {
		fs, info, err := decodeLDAPFields(wire, "bind-request")
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fs, len(wire))
		require.Equal(t, false, info["Session State Validated"])
		for cut := 0; cut < len(wire); cut++ {
			fs, info, err := decodeLDAPFields(wire[:cut], "bind-request")
			require.Error(t, err, "prefix %d", cut)
			require.Nil(t, fs)
			require.Nil(t, info)
		}
		for index := range wire {
			for _, value := range []byte{0, 1, 127, 128, 255} {
				w := bytes.Clone(wire)
				w[index] = value
				fs, info, err := decodeLDAPFields(w, "bind-request")
				if err != nil {
					require.Nil(t, fs)
					require.Nil(t, info)
				} else {
					tlsCertificateTestCoverage(t, fs, len(w))
				}
			}
		}
	}
	// BER, unlike DER, permits a definite nonminimal long-form length.
	wire := []byte{0x30, 0x82, 0, 15, 2, 0x81, 1, 1, 0x60, 0x81, 8, 2, 1, 3, 4, 0, 0x80, 0x81, 0}
	fs, _, err := decodeLDAPFields(wire, "bind-request")
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fs, len(wire))
}
func TestLDAPFieldsMalformedGrammar(t *testing.T) {
	valid := ldapFieldsTestFixtures()[0]
	for _, tc := range []struct {
		at    int
		value byte
	}{{0, 0x31}, {1, 0x80}, {2, 4}, {3, 0}, {4, 0}, {5, 0x63}, {7, 4}, {8, 0}, {9, 2}, {10, 0x24}, {12, 0xa0}} {
		w := bytes.Clone(valid)
		w[tc.at] = tc.value
		_, _, err := decodeLDAPFields(w, "bind-request")
		require.Error(t, err, "byte %d", tc.at)
	}
	for _, id := range [][]byte{nil, {0}, {128}, {0, 1}, {0, 0, 128}, {0x80, 0, 0, 0}, {0, 0x80, 0, 0, 0}} {
		_, _, err := decodeLDAPFields(ldapFieldsTestBind(id, nil, []byte{0x80, 0}, nil), "bind-request")
		require.Error(t, err)
	}
	for _, choice := range [][]byte{{0xa3, 0}, {0xa3, 2, 4, 1}, {0xa3, 4, 4, 0, 5, 0}, {0xa3, 6, 4, 0, 4, 0, 4, 0}, {0xa3, 3, 4, 1, 255}, {0x80, 0, 0}, {0xa1, 0}, {0x80, 0x85, 0, 0, 0, 0, 0}} {
		_, _, err := decodeLDAPFields(ldapFieldsTestBind([]byte{1}, nil, choice, nil), "bind-request")
		require.Error(t, err)
	}
	for _, oid := range []string{"", "1", "1..2", "01.2", "1.02", "a.2", "1.2."} {
		control := ldapFieldsTestTLV(0x30, ldapFieldsTestTLV(4, []byte(oid)))
		_, _, err := decodeLDAPFields(ldapFieldsTestBind([]byte{1}, nil, []byte{128, 0}, control), "bind-request")
		require.Error(t, err)
	}
	for _, rest := range [][]byte{{1, 1, 0}, {1, 1, 1}, {1, 0}, {1, 2, 255, 255}, {4, 0, 4, 0}, {0x24, 0}, {1, 1, 255, 1, 1, 255}} {
		control := ldapFieldsTestTLV(0x30, append(ldapFieldsTestTLV(4, []byte("1.2")), rest...))
		_, _, err := decodeLDAPFields(ldapFieldsTestBind([]byte{1}, nil, []byte{128, 0}, control), "bind-request")
		require.Error(t, err)
	}
	_, _, err := decodeLDAPFields(ldapFieldsTestBind([]byte{1}, []byte{255}, []byte{128, 0}, nil), "bind-request")
	require.Error(t, err)
	_, _, err = decodeLDAPFields(append(bytes.Clone(valid), valid...), "bind-request")
	require.Error(t, err)
}
func TestLDAPFieldsLimitsAndIsolation(t *testing.T) {
	control := ldapFieldsTestTLV(0x30, ldapFieldsTestTLV(4, []byte("1.2")))
	for _, count := range []int{4096, 4097} {
		w := ldapFieldsTestBind([]byte{1}, nil, []byte{128, 0}, bytes.Repeat(control, count))
		fs, info, err := decodeLDAPFields(w, "bind-request")
		if count == 4096 {
			require.NoError(t, err)
			require.Len(t, info["Controls"], count)
			tlsCertificateTestCoverage(t, fs, len(w))
		} else {
			require.ErrorContains(t, err, "resource limit")
			require.Nil(t, fs)
			require.Nil(t, info)
		}
	}
	for _, size := range []int{ldapFieldsMaxBytes, ldapFieldsMaxBytes + 1} {
		w := ldapFieldsTestBind([]byte{1}, nil, ldapFieldsTestTLV(128, make([]byte, size-23)), nil)
		require.Len(t, w, size)
		fs, _, err := decodeLDAPFields(w, "bind-request")
		if size == ldapFieldsMaxBytes {
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fs, len(w))
		} else {
			require.Error(t, err)
		}
	}
	w := ldapFieldsTestFixtures()[1]
	original := bytes.Clone(w)
	_, info, err := decodeLDAPFields(w, "bind-request")
	require.NoError(t, err)
	info["Directory Name"].(map[string]any)["Value"].([]byte)[0] = '!'
	info["Controls"].([]map[string]any)[0]["OID"].(map[string]any)["Value"].([]byte)[0] = '!'
	require.Equal(t, original, w)
	_, _, err = decodeLDAPFields(w, "unknown")
	require.Error(t, err)
}
func TestLDAPFieldsBridgeTransactions(t *testing.T) {
	testExactByteFieldsBridgeTransactions(t, []string{"bind-request"}, func(string) []byte { return bytes.Clone(ldapFieldsTestFixtures()[1]) }, parseLDAPFields)
}
