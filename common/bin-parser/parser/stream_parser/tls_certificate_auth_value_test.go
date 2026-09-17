package stream_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func tlsAuthTestMessage(kind byte, body []byte) []byte {
	return append([]byte{kind, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
}

func tlsAuthTestRequest(types, algorithms []byte, names ...[]byte) []byte {
	body := append([]byte{byte(len(types))}, types...)
	body = append(body, byte(len(algorithms)>>8), byte(len(algorithms)))
	body = append(body, algorithms...)
	var namesBody []byte
	for _, name := range names {
		namesBody = append(namesBody, byte(len(name)>>8), byte(len(name)))
		namesBody = append(namesBody, name...)
	}
	body = append(body, byte(len(namesBody)>>8), byte(len(namesBody)))
	return tlsAuthTestMessage(13, append(body, namesBody...))
}

func tlsAuthTestVerify(algorithm [2]byte, signature []byte) []byte {
	body := append([]byte{algorithm[0], algorithm[1], byte(len(signature) >> 8), byte(len(signature))}, signature...)
	return tlsAuthTestMessage(15, body)
}

func tlsAuthTestCoverage(t *testing.T, fields []tlsCertificateAuthField, start, end int) {
	t.Helper()
	at := start
	for _, field := range fields {
		require.Equal(t, at, field.Start, field.Name)
		require.GreaterOrEqual(t, field.End, field.Start, field.Name)
		if field.Type == "" {
			tlsAuthTestCoverage(t, field.Children, field.Start, field.End)
		}
		at = field.End
	}
	require.Equal(t, end, at)
}

func TestTLS12CertificateAuthLayouts(t *testing.T) {
	for name, wire := range map[string][]byte{
		"one pair empty authorities": tlsAuthTestRequest([]byte{1}, []byte{4, 1}),
		"multiple pairs and names":   tlsAuthTestRequest([]byte{64, 1, 2}, []byte{6, 3, 6, 1, 4, 1}, []byte{0x30, 0}, []byte{0x30, 1, 0}),
		"duplicates preserved":       tlsAuthTestRequest([]byte{1, 1}, []byte{4, 1, 4, 1}, []byte{1}, []byte{1}),
		"opaque name not DER":        tlsAuthTestRequest([]byte{255}, []byte{255, 255}, []byte{255}),
		"empty signature":            tlsAuthTestVerify([2]byte{4, 1}, nil),
		"signature":                  tlsAuthTestVerify([2]byte{6, 1}, []byte{1, 2, 3}),
		"unassigned identifiers":     tlsAuthTestVerify([2]byte{255, 255}, []byte{0}),
	} {
		t.Run(name, func(t *testing.T) {
			fields, info, err := decodeTLS12CertificateAuth(wire, wire[0])
			require.NoError(t, err)
			tlsAuthTestCoverage(t, fields, 0, len(wire))
			require.Equal(t, true, info["Version Is Caller Supplied"])
			for _, field := range []string{"Sender Conformance Validated", "Registry Values Validated", "Algorithm Usage Validated", "Certificate Compatibility Validated", "Request Correlation Validated", "Handshake Transcript Validated", "Signature Verified", "Peer Identity Validated", "Handshake Completion Validated", "TCP Reassembly Performed", "Record Reassembly Performed", "Distinguished Names DER Parsed"} {
				require.Equal(t, false, info[field], field)
			}
			for cut := 0; cut < len(wire); cut++ {
				_, _, err = decodeTLS12CertificateAuth(wire[:cut], wire[0])
				require.Error(t, err, "short prefix %d", cut)
			}
			_, _, err = decodeTLS12CertificateAuth(append(bytes.Clone(wire), 0), wire[0])
			require.Error(t, err)
		})
	}
	// These values are exposed numerically, not accepted as usable choices.
	for value := 0; value < 256; value++ {
		wire := tlsAuthTestRequest([]byte{byte(value)}, []byte{byte(value), byte(value)})
		_, info, err := decodeTLS12CertificateAuth(wire, 13)
		require.NoError(t, err)
		require.Equal(t, 1, info["Certificate Type Count"])
		require.Equal(t, 1, info["Signature Algorithm Count"])
		require.Equal(t, 0, info["Distinguished Name Count"])
		_, _, err = decodeTLS12CertificateAuth(tlsAuthTestVerify([2]byte{byte(value), byte(value)}, nil), 15)
		require.NoError(t, err)
	}
}

func TestTLS12CertificateAuthBounds(t *testing.T) {
	for _, count := range []int{1024, 1025} {
		_, _, err := decodeTLS12CertificateAuth(tlsAuthTestRequest([]byte{1}, bytes.Repeat([]byte{4, 1}, count)), 13)
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "signature algorithm count")
		}
		names := make([][]byte, count)
		for i := range names {
			names[i] = []byte{1}
		}
		_, _, err = decodeTLS12CertificateAuth(tlsAuthTestRequest([]byte{1}, []byte{4, 1}, names...), 13)
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "distinguished name count")
		}
	}
	for _, wire := range [][]byte{
		tlsAuthTestRequest(bytes.Repeat([]byte{1}, 255), []byte{4, 1}, bytes.Repeat([]byte{0xa5}, 65533)),
		tlsAuthTestVerify([2]byte{4, 1}, bytes.Repeat([]byte{0xa5}, 65535)),
	} {
		fields, _, err := decodeTLS12CertificateAuth(wire, wire[0])
		require.NoError(t, err)
		tlsAuthTestCoverage(t, fields, 0, len(wire))
	}
	_, _, err := decodeTLS12CertificateAuth(make([]byte, tlsCertificateAuthMaxBytes+1), 13)
	require.ErrorContains(t, err, "131333")
}

func TestTLS12CertificateAuthMalformedVectors(t *testing.T) {
	for name, body := range map[string][]byte{
		"empty types":              {0, 0, 2, 4, 1, 0, 0},
		"types exceed input":       {255, 1},
		"missing algorithm count":  {1, 1, 0},
		"empty algorithm list":     {1, 1, 0, 0, 0, 0},
		"odd algorithm list":       {1, 1, 0, 1, 4, 0, 0},
		"algorithms exceed input":  {1, 1, 0, 8, 4, 1},
		"missing authority count":  {1, 1, 0, 2, 4, 1, 0},
		"authority list overflow":  {1, 1, 0, 2, 4, 1, 0, 1},
		"authority list underflow": {1, 1, 0, 2, 4, 1, 0, 0, 1},
		"partial name length":      {1, 1, 0, 2, 4, 1, 0, 1, 0},
		"empty name":               {1, 1, 0, 2, 4, 1, 0, 2, 0, 0},
		"name overflow":            {1, 1, 0, 2, 4, 1, 0, 3, 0, 2, 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := decodeTLS12CertificateAuth(tlsAuthTestMessage(13, body), 13)
			require.Error(t, err)
		})
	}
	for _, wire := range [][]byte{
		tlsAuthTestMessage(15, []byte{4, 1, 0}),
		tlsAuthTestMessage(15, []byte{4, 1, 0, 2, 1}),
		tlsAuthTestMessage(15, []byte{4, 1, 0, 0, 1}),
	} {
		_, _, err := decodeTLS12CertificateAuth(wire, 15)
		require.Error(t, err)
	}
	valid := tlsAuthTestRequest([]byte{1}, []byte{4, 1})
	for kind := 0; kind < 256; kind++ {
		if kind == 13 {
			continue
		}
		_, _, err := decodeTLS12CertificateAuth(valid, uint8(kind))
		require.Error(t, err)
	}
}
