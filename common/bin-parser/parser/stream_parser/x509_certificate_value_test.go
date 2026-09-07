package stream_parser

import (
	"bytes"
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

func x509TestTLV(tag byte, parts ...[]byte) []byte {
	var value []byte
	for _, part := range parts {
		value = append(value, part...)
	}
	length := len(value)
	header := []byte{tag}
	if length < 128 {
		header = append(header, byte(length))
	} else {
		var encoded []byte
		for n := length; n > 0; n >>= 8 {
			encoded = append([]byte{byte(n)}, encoded...)
		}
		header = append(header, byte(128+len(encoded)))
		header = append(header, encoded...)
	}
	return append(header, value...)
}

// Deliberately syntactic fixtures, not issued certificates or usable keys.
// No key generation or signature verification is involved in these controls.
func x509TestParts() ([][]byte, []byte, []byte) {
	algorithm := x509TestTLV(0x30, []byte{6, 3, 0x2a, 3, 4}, []byte{5, 0})
	name := x509TestTLV(0x30, x509TestTLV(0x31, x509TestTLV(0x30, []byte{6, 3, 0x55, 4, 3}, x509TestTLV(12, []byte("example")))))
	validity := x509TestTLV(0x30, x509TestTLV(23, []byte("500101000000Z")), x509TestTLV(24, []byte("20500101000000Z")))
	spki := x509TestTLV(0x30, algorithm, []byte{3, 2, 0, 0x80})
	return [][]byte{x509TestTLV(0xa0, []byte{2, 1, 2}), {2, 1, 1}, algorithm, name, validity, name, spki}, algorithm, []byte{3, 2, 0, 0x80}
}

func x509TestCertificate(parts [][]byte, algorithm, signature []byte) []byte {
	return x509TestTLV(0x30, x509TestTLV(0x30, parts...), algorithm, signature)
}

func TestX509CertificateDERLayouts(t *testing.T) {
	parts, algorithm, signature := x509TestParts()
	for _, version := range []int{1, 2, 3} {
		p := append([][]byte(nil), parts...)
		if version == 1 {
			p = p[1:]
		} else {
			p[0] = x509TestTLV(0xa0, []byte{2, 1, byte(version - 1)})
		}
		wire := x509TestCertificate(p, algorithm, signature)
		fields, info, err := decodeX509CertificateDER(wire)
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fields, len(wire))
		require.Equal(t, version, info["Certificate Version"])
		require.Equal(t, "1950-01-01T00:00:00Z", info["Not Before UTC"])
		require.Equal(t, "2050-01-01T00:00:00Z", info["Not After UTC"])
		require.Equal(t, "1.2.3.4", info["Public Key Algorithm OID"])
		require.Equal(t, false, info["Public Key Validated"])
		require.Equal(t, false, info["Current Validity Checked"])
		for cut := 0; cut < len(wire); cut++ {
			fields, info, err := decodeX509CertificateDER(wire[:cut])
			require.Error(t, err)
			require.Nil(t, fields)
			require.Nil(t, info)
		}
		_, _, err = decodeX509CertificateDER(append(bytes.Clone(wire), 0))
		require.Error(t, err)
	}
	for _, literal := range [][]byte{{0}, {0xff}, {0x80}, {0, 0x80}} {
		p := append([][]byte(nil), parts...)
		p[1] = x509TestTLV(2, literal)
		_, info, err := decodeX509CertificateDER(x509TestCertificate(p, algorithm, signature))
		require.NoError(t, err)
		want := map[byte]string{0: "0", 0xff: "-1", 0x80: "-128"}[literal[0]]
		if len(literal) == 2 {
			want = "128"
		}
		require.Equal(t, want, info["Serial Number Decimal"])
	}
	// Unknown algorithm parameters and optional unique IDs remain visible.
	p := append(append([][]byte(nil), parts...), []byte{0x81, 2, 3, 0xa0}, []byte{0x82, 1, 0})
	fields, _, err := decodeX509CertificateDER(x509TestCertificate(p, algorithm, signature))
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fields, len(x509TestCertificate(p, algorithm, signature)))
	// Algorithm equality is observed metadata, never signature verification.
	otherAlgorithm := x509TestTLV(0x30, []byte{6, 3, 0x2a, 3, 5}, []byte{5, 0})
	_, info, err := decodeX509CertificateDER(x509TestCertificate(parts, otherAlgorithm, signature))
	require.NoError(t, err)
	require.Equal(t, false, info["Signature Algorithm Encodings Match"])
	require.Equal(t, false, info["Signature Verified"])
	// An unknown high-tag parameter is retained, not mistaken for a length.
	parameterAlgorithm := x509TestTLV(0x30, []byte{6, 3, 0x2a, 3, 4}, []byte{0x9f, 0x81, 0, 1, 0x42})
	_, _, err = decodeX509CertificateDER(x509TestCertificate(parts, parameterAlgorithm, signature))
	require.NoError(t, err)
}

func TestX509CertificateDERMalformedGrammar(t *testing.T) {
	parts, algorithm, signature := x509TestParts()
	for _, spec := range []struct {
		index int
		value []byte
	}{
		{0, x509TestTLV(0xa0, []byte{2, 1, 0})}, {0, x509TestTLV(0xa0, []byte{2, 1, 3})},
		{0, x509TestTLV(0xa0, []byte{2, 2, 1, 1})}, {1, []byte{2, 0}}, {1, []byte{2, 2, 0, 1}},
		{2, []byte{0x30, 0}}, {2, x509TestTLV(0x30, []byte{5, 0})},
		{3, x509TestTLV(0x30, []byte{0x31, 0})},
		{4, x509TestTLV(0x30, x509TestTLV(23, []byte("240230000000Z")), x509TestTLV(23, []byte("490101000000Z")))},
		{4, x509TestTLV(0x30, x509TestTLV(23, []byte("2401010000Z")), x509TestTLV(23, []byte("490101000000Z")))},
		{6, x509TestTLV(0x30, algorithm, []byte{3, 2, 1, 1})},
	} {
		p := append([][]byte(nil), parts...)
		p[spec.index] = spec.value
		fields, info, err := decodeX509CertificateDER(x509TestCertificate(p, algorithm, signature))
		require.Error(t, err, "field %d", spec.index)
		require.Nil(t, fields)
		require.Nil(t, info)
	}
	for _, optional := range [][]byte{
		{0x81, 0}, {0x81, 2, 8, 0}, {0x81, 2, 1, 1}, {0xa1, 0},
		x509TestTLV(0xa3, []byte{0x30, 0}),
		x509TestTLV(0xa3, x509TestTLV(0x30, x509TestTLV(0x30, []byte{6, 3, 0x55, 0x1d, 19}, []byte{1, 1, 0}, []byte{4, 0}))),
	} {
		p := append(append([][]byte(nil), parts...), optional)
		_, _, err := decodeX509CertificateDER(x509TestCertificate(p, algorithm, signature))
		require.Error(t, err)
	}
	p := append(append([][]byte(nil), parts...), []byte{0x82, 1, 0}, []byte{0x81, 1, 0})
	_, _, err := decodeX509CertificateDER(x509TestCertificate(p, algorithm, signature))
	require.Error(t, err)
	ext := x509TestTLV(0x30, []byte{6, 3, 0x55, 0x1d, 19}, []byte{1, 1, 255}, []byte{4, 0})
	p = append(append([][]byte(nil), parts...), x509TestTLV(0xa3, x509TestTLV(0x30, ext, ext)))
	_, _, err = decodeX509CertificateDER(x509TestCertificate(p, algorithm, signature))
	require.ErrorContains(t, err, "duplicate extension")
	p = append(append([][]byte(nil), parts...), x509TestTLV(0xa3, x509TestTLV(0x30, ext)))
	p[0] = x509TestTLV(0xa0, []byte{2, 1, 1})
	_, _, err = decodeX509CertificateDER(x509TestCertificate(p, algorithm, signature))
	require.ErrorContains(t, err, "v3")
}

func TestX509CertificateDERFramingAndLimits(t *testing.T) {
	for _, wire := range [][]byte{
		{0x30, 0x80, 0, 0}, {4, 0x81, 1, 0}, {4, 0x82, 0, 128}, {4, 0xff},
		{0x1f, 0x80, 0x1f, 0}, {0x1f, 0x1e, 0}, {0x1f, 0x81},
		{1, 1, 1}, {2, 2, 0xff, 0x80}, {3, 1, 1}, {5, 1, 0},
		{6, 2, 0x80, 0}, {6, 1, 0x80}, {12, 1, 0xff}, {0x24, 0},
		{0x31, 6, 2, 1, 2, 2, 1, 1},
	} {
		r := &x509DERReader{wire: wire}
		_, err := r.element(0, len(wire), 0)
		require.Error(t, err)
	}
	for _, depth := range []int{32, 33} {
		wire := []byte{5, 0}
		for i := 0; i < depth; i++ {
			wire = x509TestTLV(0x30, wire)
		}
		r := &x509DERReader{wire: wire}
		_, err := r.element(0, len(wire), 0)
		if depth == 32 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, nodes := range []int{16384, 16385} {
		wire := x509TestTLV(0x30, bytes.Repeat([]byte{5, 0}, nodes-1))
		r := &x509DERReader{wire: wire}
		_, err := r.element(0, len(wire), 0)
		if nodes == 16384 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	parts, algorithm, signature := x509TestParts()
	for _, count := range []int{1024, 1025} {
		var extensions [][]byte
		for i := 0; i < count; i++ {
			oid, err := asn1.Marshal(asn1.ObjectIdentifier{1, 2, 3, i})
			require.NoError(t, err)
			extensions = append(extensions, x509TestTLV(0x30, oid, []byte{4, 0}))
		}
		p := append(append([][]byte(nil), parts...), x509TestTLV(0xa3, x509TestTLV(0x30, extensions...)))
		wire := x509TestCertificate(p, algorithm, signature)
		fields, info, err := decodeX509CertificateDER(wire)
		if count == 1024 {
			require.NoError(t, err)
			require.Equal(t, count, info["Extension Count"])
			tlsCertificateTestCoverage(t, fields, len(wire))
		} else {
			require.Error(t, err)
		}
	}
	for _, length := range []int{1024, 1025} {
		p := append([][]byte(nil), parts...)
		p[1] = x509TestTLV(2, bytes.Repeat([]byte{1}, length))
		_, _, err := decodeX509CertificateDER(x509TestCertificate(p, algorithm, signature))
		if length == 1024 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, size := range []int{tlsCertificateMaxBytes, tlsCertificateMaxBytes + 1} {
		length := size - 200
		var wire []byte
		for i := 0; i < 3; i++ {
			ext := x509TestTLV(0x30, []byte{6, 3, 0x2a, 3, 4}, x509TestTLV(4, make([]byte, length)))
			p := append(append([][]byte(nil), parts...), x509TestTLV(0xa3, x509TestTLV(0x30, ext)))
			wire = x509TestCertificate(p, algorithm, signature)
			length += size - len(wire)
		}
		require.Len(t, wire, size)
		fields, _, err := decodeX509CertificateDER(wire)
		if size == tlsCertificateMaxBytes {
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fields, size)
		} else {
			require.Error(t, err)
		}
	}
}
