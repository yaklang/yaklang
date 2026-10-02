package bin_parser

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const x509PublicKeyTestEntry = "X509CertificateDERWithPublicKey"

func x509PublicKeyTestOracle(t *testing.T, n *base.Node, wire []byte) string {
	t.Helper()
	cert, err := x509.ParseCertificate(wire)
	require.NoError(t, err)
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, true, info["Public Key Fields Decoded"])
	require.Equal(t, false, info["Public Key Validated"])
	detail := info["Public Key Details"].(map[string]any)
	require.Equal(t, true, detail["Decoded"])
	require.Equal(t, true, detail["Parameters Supported"])
	require.Equal(t, false, detail["Key Mathematics Validated"])
	require.Equal(t, false, detail["Point Decompressed"])
	switch key := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		require.Equal(t, "RSA", detail["Layout"])
		require.Equal(t, key.N.BitLen(), detail["Modulus Bit Length"])
		require.Equal(t, strconv.Itoa(key.E), detail["Public Exponent Decimal"])
		modulus := protocolCorpusFindNode(protocolCorpusFindNode(n, "RSA Modulus"), "Value")
		exponent := protocolCorpusFindNode(protocolCorpusFindNode(n, "RSA Public Exponent"), "Value")
		require.NotNil(t, modulus)
		require.NotNil(t, exponent)
		require.Zero(t, new(big.Int).SetBytes(stream_parser.GetBytesByNode(modulus)).Cmp(key.N))
		require.Equal(t, strconv.Itoa(key.E), new(big.Int).SetBytes(stream_parser.GetBytesByNode(exponent)).String())
		return "RSA"
	case *ecdsa.PublicKey:
		width := (key.Curve.Params().BitSize + 7) / 8
		require.Equal(t, "EC Point", detail["Layout"])
		require.Equal(t, width, detail["Coordinate Bytes"])
		require.Equal(t, false, detail["Compressed"])
		require.Equal(t, 4, detail["Point Format"])
		protocolCorpusRequireValue(t, n, "EC Point Format", uint64(4))
		require.Equal(t, key.X.FillBytes(make([]byte, width)), stream_parser.GetBytesByNode(protocolCorpusFindNode(n, "EC X Coordinate")))
		require.Equal(t, key.Y.FillBytes(make([]byte, width)), stream_parser.GetBytesByNode(protocolCorpusFindNode(n, "EC Y Coordinate")))
		curve := map[string]string{"P-256": "1.2.840.10045.3.1.7", "P-384": "1.3.132.0.34", "P-521": "1.3.132.0.35"}[key.Curve.Params().Name]
		require.NotEmpty(t, curve)
		require.Equal(t, curve, detail["Named Curve OID"])
		return "EC/" + key.Curve.Params().Name
	default:
		t.Fatalf("unaccounted original public-key type %T", key)
	}
	return ""
}

func TestProtocolCorpusX509PublicKeyAllOriginalRecords(t *testing.T) {
	x509TestExpandedAllOriginalRecords(t, x509PublicKeyTestEntry)
}
func TestProtocolCorpusX509PublicKeyOffsetsAndRollback(t *testing.T) {
	x509TestExpandedOffsetsAndRollback(t, x509PublicKeyTestEntry)
}
