package bin_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestProtocolCorpusTLSCertificateStatusCanonicalOwnedVector(t *testing.T) {
	// RFC 6066 section 8. Minimal DER OCSPResponse tryLater; not a good-status
	// assertion. Same literal was independently stapled on real OpenSSL traffic.
	wire := []byte{22, 0, 0, 9, 1, 0, 0, 5, 0x30, 3, 0x0a, 1, 3}
	n := protocolCorpusRequireBoundedRuleParse(t, wire, tlsControlTestRule, "TLS12CertificateStatus")
	for _, f := range []struct {
		name, typ  string
		start, end uint64
		value      any
	}{
		{"Handshake Type", "uint8", 0, 8, uint64(22)},
		{"Handshake Length", "uint32", 8, 32, uint64(9)},
		{"Status Type", "uint8", 32, 40, uint64(1)},
		{"OCSP Response Length", "uint32", 40, 64, uint64(5)},
		{"OCSP Response", "raw", 64, 104, wire[8:]},
	} {
		latTestField(t, n, f.name, f.typ, f.start, f.end, f.value)
	}
	for cut := 0; cut < len(wire); cut++ {
		_, _, err := stream_parser.DecodeTLS12CertificateStatusLayout(wire[:cut])
		require.Error(t, err)
	}
	for _, body := range [][]byte{{1, 0, 0, 0}, {1, 0, 0, 2, 1}, {1, 0, 0, 1, 1, 2}, {2, 0, 0, 1, 1}} {
		candidate := append([]byte{22, 0, 0, byte(len(body))}, body...)
		_, _, err := stream_parser.DecodeTLS12CertificateStatusLayout(candidate)
		require.Error(t, err)
	}
	// The grammar owns no source bytes and does not need to understand DER.
	opaque := []byte{22, 0, 0, 5, 1, 0, 0, 1, 0xff}
	fields, info, err := stream_parser.DecodeTLS12CertificateStatusLayout(opaque)
	require.NoError(t, err)
	require.Len(t, fields, 5)
	require.Equal(t, false, info["OCSP DER Decoded"])
	require.Equal(t, false, info["OCSP Response Validated"])
	tooLarge := bytes.Repeat([]byte{0}, stream_parser.TLS12CertificateStatusMaxBytes+1)
	_, _, err = stream_parser.DecodeTLS12CertificateStatusLayout(tooLarge)
	require.Error(t, err)
}
