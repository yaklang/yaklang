package stream_parser

import (
	"bytes"
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

func x509PublicKeyTestCertificate(t *testing.T, algorithm asn1.ObjectIdentifier, parameter, bits []byte) []byte {
	t.Helper()
	oid, err := asn1.Marshal(algorithm)
	require.NoError(t, err)
	parts, alg, sig := x509TestParts()
	parts[6] = x509TestTLV(0x30, x509TestTLV(0x30, oid, parameter), x509TestTLV(3, bits))
	return x509TestCertificate(parts, alg, sig)
}

var x509PublicKeyTestRSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
var x509PublicKeyTestEC = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}

func TestX509PublicKeyRSAFieldsAndBoundaries(t *testing.T) {
	rsa := x509TestTLV(0x30, []byte{2, 2, 0, 128}, []byte{2, 3, 1, 0, 1})
	wire := x509PublicKeyTestCertificate(t, x509PublicKeyTestRSA, []byte{5, 0}, append([]byte{0}, rsa...))
	fields, info, err := decodeX509CertificateDERPublicKey(wire)
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fields, len(wire))
	d := info["Public Key Details"].(map[string]any)
	require.Equal(t, 8, d["Modulus Bit Length"])
	require.Equal(t, "65537", d["Public Exponent Decimal"])
	for cut := 0; cut < len(wire); cut++ {
		f, m, e := decodeX509CertificateDERPublicKey(wire[:cut])
		require.Error(t, e)
		require.Nil(t, f)
		require.Nil(t, m)
	}
	_, _, err = decodeX509CertificateDERPublicKey(append(bytes.Clone(wire), 0))
	require.Error(t, err)
	invalid := [][]byte{
		x509TestTLV(0x30, []byte{2, 1, 0}, []byte{2, 1, 3}),
		x509TestTLV(0x30, []byte{2, 1, 255}, []byte{2, 1, 3}),
		x509TestTLV(0x30, []byte{2, 2, 0, 1}, []byte{2, 1, 3}),
		x509TestTLV(0x30, []byte{2, 1, 1}, []byte{2, 1, 0}),
		x509TestTLV(0x30, []byte{2, 1, 1}),
		x509TestTLV(0x30, []byte{2, 1, 1}, []byte{2, 1, 3}, []byte{5, 0}),
		x509TestTLV(0x30, []byte{2, 1, 1}, []byte{4, 1, 3}),
		append(bytes.Clone(rsa), 0), nil,
	}
	for _, bad := range invalid {
		w := x509PublicKeyTestCertificate(t, x509PublicKeyTestRSA, []byte{5, 0}, append([]byte{0}, bad...))
		f, m, e := decodeX509CertificateDERPublicKey(w)
		require.Error(t, e)
		require.Nil(t, f)
		require.Nil(t, m)
		// Old extension-only entry still treats these bits as opaque.
		_, _, e = decodeX509CertificateDERExtensions(w)
		require.NoError(t, e)
	}
	_, _, err = decodeX509CertificateDERPublicKey(x509PublicKeyTestCertificate(t, x509PublicKeyTestRSA, []byte{5, 0}, []byte{1, 0}))
	require.ErrorContains(t, err, "whole-octet")
	// No primality, parity, strength, or exponent compatibility is inferred.
	small := x509TestTLV(0x30, []byte{2, 1, 4}, []byte{2, 1, 2})
	_, info, err = decodeX509CertificateDERPublicKey(x509PublicKeyTestCertificate(t, x509PublicKeyTestRSA, []byte{5, 0}, append([]byte{0}, small...)))
	require.NoError(t, err)
	require.Equal(t, false, info["Public Key Validated"])
	for _, size := range []int{8193, 8194} {
		for _, index := range []int{0, 1} {
			integers := [][]byte{{2, 1, 1}, {2, 1, 3}}
			integers[index] = x509TestTLV(2, bytes.Repeat([]byte{1}, size))
			w := x509PublicKeyTestCertificate(t, x509PublicKeyTestRSA, []byte{5, 0}, append([]byte{0}, x509TestTLV(0x30, integers...)...))
			f, _, e := decodeX509CertificateDERPublicKey(w)
			if size == 8193 {
				require.NoError(t, e)
				tlsCertificateTestCoverage(t, f, len(w))
			} else {
				require.ErrorContains(t, e, "8193")
			}
		}
	}
}

func TestX509PublicKeyECFieldsAndBoundaries(t *testing.T) {
	curves := []struct {
		oid   asn1.ObjectIdentifier
		width int
	}{{asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7}, 32}, {asn1.ObjectIdentifier{1, 3, 132, 0, 34}, 48}, {asn1.ObjectIdentifier{1, 3, 132, 0, 35}, 66}}
	for _, curve := range curves {
		parameter, e := asn1.Marshal(curve.oid)
		require.NoError(t, e)
		for _, format := range []byte{2, 3, 4} {
			width := curve.width
			if format == 4 {
				width *= 2
			}
			point := append([]byte{0, format}, bytes.Repeat([]byte{0}, width)...)
			wire := x509PublicKeyTestCertificate(t, x509PublicKeyTestEC, parameter, point)
			f, info, e := decodeX509CertificateDERPublicKey(wire)
			require.NoError(t, e)
			tlsCertificateTestCoverage(t, f, len(wire))
			d := info["Public Key Details"].(map[string]any)
			require.Equal(t, curve.oid.String(), d["Named Curve OID"])
			require.Equal(t, curve.width, d["Coordinate Bytes"])
			require.Equal(t, format != 4, d["Compressed"])
			require.Equal(t, false, d["Key Mathematics Validated"])
			require.Equal(t, false, d["Point Decompressed"])
			for cut := 0; cut < len(wire); cut++ {
				_, _, e := decodeX509CertificateDERPublicKey(wire[:cut])
				require.Error(t, e)
			}
			for _, bad := range [][]byte{point[:len(point)-1], append(bytes.Clone(point), 0), {0}, append([]byte{0, 6}, point[2:]...), append([]byte{1, format}, point[2:]...)} {
				_, _, e := decodeX509CertificateDERPublicKey(x509PublicKeyTestCertificate(t, x509PublicKeyTestEC, parameter, bad))
				require.Error(t, e)
			}
		}
	}
}

func TestX509PublicKeyUnknownProfilesStayOpaque(t *testing.T) {
	unknownCurve, err := asn1.Marshal(asn1.ObjectIdentifier{1, 2, 3, 4})
	require.NoError(t, err)
	for _, tc := range []struct {
		oid       asn1.ObjectIdentifier
		parameter []byte
	}{
		{asn1.ObjectIdentifier{1, 2, 3, 4}, nil}, {x509PublicKeyTestRSA, nil}, {x509PublicKeyTestRSA, unknownCurve},
		{x509PublicKeyTestEC, nil}, {x509PublicKeyTestEC, []byte{5, 0}}, {x509PublicKeyTestEC, unknownCurve},
	} {
		wire := x509PublicKeyTestCertificate(t, tc.oid, tc.parameter, []byte{0, 255, 42})
		f, info, e := decodeX509CertificateDERPublicKey(wire)
		require.NoError(t, e)
		tlsCertificateTestCoverage(t, f, len(wire))
		require.Equal(t, false, info["Public Key Fields Decoded"])
		d := info["Public Key Details"].(map[string]any)
		require.Equal(t, false, d["Parameters Supported"])
		require.NotEmpty(t, d["Opaque Reason"])
	}
}
