package stream_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func tlsCertificateTestLength(n int) []byte { return []byte{byte(n >> 16), byte(n >> 8), byte(n)} }

func tlsCertificateTestMessage(certificates ...[]byte) []byte {
	var list []byte
	for _, certificate := range certificates {
		list = append(list, tlsCertificateTestLength(len(certificate))...)
		list = append(list, certificate...)
	}
	body := append(tlsCertificateTestLength(len(list)), list...)
	wire := append([]byte{11}, tlsCertificateTestLength(len(body))...)
	return append(wire, body...)
}

func tlsCertificateTestCoverage(t *testing.T, fields []tlsCertificateField, size int) {
	t.Helper()
	var walk func([]tlsCertificateField, int, int)
	walk = func(fields []tlsCertificateField, start, end int) {
		at := start
		for _, field := range fields {
			require.Equal(t, at, field.Start, field.Name)
			require.GreaterOrEqual(t, field.End, field.Start, field.Name)
			if field.Type == "" {
				walk(field.Children, field.Start, field.End)
			}
			at = field.End
		}
		require.Equal(t, end, at)
	}
	walk(fields, 0, size)
}

func TestTLSCertificateHandshakeLayoutsAndLimits(t *testing.T) {
	for name, certificates := range map[string][][]byte{"empty": nil, "one": {{0x30, 0}}, "two": {{0x30, 0}, {0x30, 1, 0}}, "opaque not DER": {{0xff}}} {
		t.Run(name, func(t *testing.T) {
			wire := tlsCertificateTestMessage(certificates...)
			fields, info, err := decodeTLSCertificateHandshake(wire)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fields, len(wire))
			require.Equal(t, len(certificates), info["Certificate Count"])
			require.Equal(t, len(certificates) == 0, info["Empty Certificate List"])
			require.Equal(t, true, info["Version Is Caller Supplied"])
			for _, key := range []string{"DER Parsed", "Sender Role Validated", "Server Certificate Requirements Validated", "Certificate Chain Validated", "Certificate Trust Validated", "Peer Identity Validated", "Handshake Completion Validated", "TCP Reassembly Performed", "Record Reassembly Performed"} {
				require.Equal(t, false, info[key], key)
			}
			for cut := 0; cut < len(wire); cut++ {
				_, _, err = decodeTLSCertificateHandshake(wire[:cut])
				require.Error(t, err, "cut %d", cut)
			}
			for at := 0; at < 7; at++ {
				bad := append([]byte(nil), wire...)
				bad[at] ^= 1
				_, _, err = decodeTLSCertificateHandshake(bad)
				require.Error(t, err, "header byte %d", at)
			}
			_, _, err = decodeTLSCertificateHandshake(append(append([]byte(nil), wire...), 0))
			require.Error(t, err)
		})
	}
	for _, count := range []int{1024, 1025} {
		certs := make([][]byte, count)
		for i := range certs {
			certs[i] = []byte{1}
		}
		_, _, err := decodeTLSCertificateHandshake(tlsCertificateTestMessage(certs...))
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "1024")
		}
	}
	for _, size := range []int{tlsCertificateMaxBytes, tlsCertificateMaxBytes + 1} {
		wire := tlsCertificateTestMessage(bytes.Repeat([]byte{0xa5}, size-10))
		require.Len(t, wire, size)
		fields, _, err := decodeTLSCertificateHandshake(wire)
		if size == tlsCertificateMaxBytes {
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fields, size)
		} else {
			require.Error(t, err)
		}
	}
}

func TestTLSCertificateHandshakeMalformedVectors(t *testing.T) {
	for name, wire := range map[string][]byte{
		"empty certificate":        tlsCertificateTestMessage(nil),
		"empty second certificate": tlsCertificateTestMessage([]byte{1}, nil),
		"partial entry length":     {11, 0, 0, 5, 0, 0, 2, 0, 1},
		"entry exceeds list":       {11, 0, 0, 7, 0, 0, 4, 0, 0, 2, 1},
		"24 bit maximal length":    {11, 0, 0, 7, 0, 0, 4, 0xff, 0xff, 0xff, 1},
		"TLS record wrapper":       {22, 3, 3, 0, 7, 11, 0, 0, 3, 0, 0, 0},
		// This ordinary TLS 1.3 empty-context/list encoding has a different body.
		"TLS13 context vector": {11, 0, 0, 4, 0, 0, 0, 0},
	} {
		t.Run(name, func(t *testing.T) { _, _, err := decodeTLSCertificateHandshake(wire); require.Error(t, err) })
	}
	for n := 0; n < 256; n++ {
		wire := tlsCertificateTestMessage([]byte{1, 2, 3})
		wire[9] = byte(n)
		_, _, err := decodeTLSCertificateHandshake(wire)
		if n == 3 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
