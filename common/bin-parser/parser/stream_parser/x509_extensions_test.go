package stream_parser

import (
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func x509ExtensionTestCertificate(t *testing.T, oid string, body []byte) []byte {
	t.Helper()
	var id asn1.ObjectIdentifier
	for _, s := range strings.Split(oid, ".") {
		v, err := strconv.Atoi(s)
		require.NoError(t, err)
		id = append(id, v)
	}
	encoded, err := asn1.Marshal(id)
	require.NoError(t, err)
	parts, alg, sig := x509TestParts()
	parts = append(parts, x509TestTLV(0xa3, x509TestTLV(0x30, x509TestTLV(0x30, encoded, x509TestTLV(4, body)))))
	return x509TestCertificate(parts, alg, sig)
}

func x509ExtensionTestSCT(entries ...[]byte) []byte {
	var vector []byte
	for _, entry := range entries {
		vector = append(vector, byte(len(entry)>>8), byte(len(entry)))
		vector = append(vector, entry...)
	}
	return x509TestTLV(4, []byte{byte(len(vector) >> 8), byte(len(vector))}, vector)
}

func TestX509ExtensionsLayoutsAndMalformed(t *testing.T) {
	seq := func(p ...[]byte) []byte { return x509TestTLV(0x30, p...) }
	oid := []byte{6, 3, 0x2a, 3, 4}
	uri := x509TestTLV(0x86, []byte("https://example.invalid/record"))
	fixtures := map[string][]byte{
		"2.5.29.14": {4, 3, 1, 2, 3}, "2.5.29.15": {3, 2, 7, 0x80},
		"2.5.29.16": seq(x509TestTLV(0x80, []byte("20260101000000Z")), x509TestTLV(0x81, []byte("20270101000000Z"))),
		"2.5.29.17": seq(uri, x509TestTLV(0x87, []byte{192, 0, 2, 1}), x509TestTLV(0x88, []byte{0x2a, 3, 4})),
		"2.5.29.19": seq([]byte{1, 1, 255}, []byte{2, 1, 0}),
		"2.5.29.31": seq(seq(x509TestTLV(0xa0, x509TestTLV(0xa0, uri)), []byte{0x81, 2, 7, 0x80})),
		"2.5.29.32": seq(seq(oid, seq(seq(oid, []byte{5, 0})))),
		"2.5.29.35": seq([]byte{0x80, 1, 42}, x509TestTLV(0xa1, uri), []byte{0x82, 1, 1}),
		"2.5.29.37": seq(oid), "1.3.6.1.5.5.7.1.1": seq(seq(oid, uri)),
		"1.2.840.113533.7.65.0":   seq(x509TestTLV(27, []byte("V1.0")), []byte{3, 2, 2, 0xc0}),
		"1.3.6.1.4.1.11129.2.4.2": x509ExtensionTestSCT(make([]byte, 47), []byte{9, 42}),
	}
	for oid, body := range fixtures {
		t.Run(oid, func(t *testing.T) {
			wire := x509ExtensionTestCertificate(t, oid, body)
			fields, info, err := decodeX509CertificateDERExtensions(wire)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fields, len(wire))
			require.Equal(t, 1, info["Decoded Extension Count"])
			require.Equal(t, 0, info["Opaque Extension Count"])
			for cut := 0; cut < len(wire); cut++ {
				f, m, e := decodeX509CertificateDERExtensions(wire[:cut])
				require.Error(t, e)
				require.Nil(t, f)
				require.Nil(t, m)
			}
			for _, bad := range [][]byte{nil, append(bytes.Clone(body), 0), {5, 0}, {0x30, 0x80, 0, 0}} {
				malformed := x509ExtensionTestCertificate(t, oid, bad)
				_, _, e := decodeX509CertificateDERExtensions(malformed)
				require.Error(t, e)
				_, baseInfo, e := decodeX509CertificateDER(malformed)
				require.NoError(t, e)
				require.Equal(t, false, baseInfo["Extension Contents Decoded"])
			}
		})
	}
	invalid := []struct {
		oid  string
		body []byte
	}{
		{"2.5.29.15", []byte{3, 2, 7, 1}},
		{"2.5.29.19", seq([]byte{1, 1, 0})}, {"2.5.29.19", seq([]byte{2, 1, 255})},
		{"2.5.29.17", seq()}, {"2.5.29.17", seq([]byte{0x87, 3, 1, 2, 3})}, {"2.5.29.17", seq([]byte{0x82, 1, 255})},
		{"2.5.29.35", seq(x509TestTLV(0xa1, uri))}, {"2.5.29.35", seq([]byte{0x82, 1, 1})},
		{"2.5.29.35", seq([]byte{0x80, 1, 1}, []byte{0x80, 1, 1})},
		{"2.5.29.16", seq()}, {"2.5.29.16", seq(x509TestTLV(0x80, []byte("20260230000000Z")))},
		{"2.5.29.31", seq(seq([]byte{0x81, 1, 0}))}, {"2.5.29.37", seq()},
		{"1.3.6.1.5.5.7.1.1", seq(seq(oid))},
	}
	for _, tc := range invalid {
		_, _, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, tc.oid, tc.body))
		require.Error(t, err, tc.oid)
	}
	// Unknown OIDs have no assumed inner ASN.1 grammar.
	raw := []byte{255, 0, 128, 17}
	f, info, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "1.2.3.99", raw))
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, f, len(x509ExtensionTestCertificate(t, "1.2.3.99", raw)))
	require.Equal(t, 1, info["Opaque Extension Count"])
	require.Equal(t, false, info["Extension Contents Decoded"])
	// Entrust flags are optional, not an invented mandatory byte.
	_, info, err = decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "1.2.840.113533.7.65.0", seq(x509TestTLV(27, []byte("V1.0")))))
	require.NoError(t, err)
	require.Equal(t, false, info["Extension Details"].([]map[string]any)[0]["Flags Present"])
}

func TestX509ExtensionsSCTBoundaries(t *testing.T) {
	const oid = "1.3.6.1.4.1.11129.2.4.2"
	entry := make([]byte, 52)
	binary.BigEndian.PutUint64(entry[33:41], ^uint64(0))
	entry[42] = 2
	entry[43] = 12
	entry[44] = 34
	entry[45] = 4
	entry[46] = 3
	entry[48] = 3
	copy(entry[49:], []byte{1, 2, 3})
	wire := x509ExtensionTestCertificate(t, oid, x509ExtensionTestSCT(entry, []byte{1}))
	fields, info, err := decodeX509CertificateDERExtensions(wire)
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fields, len(wire))
	d := info["Extension Details"].([]map[string]any)[0]
	require.Equal(t, 1, d["SCT V1 Count"])
	require.Equal(t, 1, d["Opaque SCT Count"])
	for cut := 0; cut < len(entry); cut++ {
		_, _, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, oid, x509ExtensionTestSCT(entry[:cut])))
		require.Error(t, err)
	}
	for _, position := range []int{41, 42, 47, 48} {
		bad := bytes.Clone(entry)
		bad[position] = 255
		_, _, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, oid, x509ExtensionTestSCT(bad)))
		require.Error(t, err)
	}
	for _, body := range [][]byte{x509ExtensionTestSCT(), {4, 3, 0, 1, 0}, {4, 5, 0, 3, 0, 0, 1}} {
		_, _, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, oid, body))
		require.Error(t, err)
	}
	entries := make([][]byte, 1025)
	for i := range entries {
		entries[i] = []byte{1}
	}
	_, _, err = decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, oid, x509ExtensionTestSCT(entries[:1024]...)))
	require.NoError(t, err)
	_, _, err = decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, oid, x509ExtensionTestSCT(entries...)))
	require.ErrorContains(t, err, "resource limit")
	// Payload field budgets must not be reported as DER element counts.
	_, a, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, oid, x509ExtensionTestSCT([]byte{1})))
	require.NoError(t, err)
	_, b, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, oid, x509ExtensionTestSCT([]byte{1}, []byte{1})))
	require.NoError(t, err)
	require.Equal(t, a["DER Element Count"], b["DER Element Count"])
}

func TestX509ExtensionsOptionalGrammarAndLimits(t *testing.T) {
	seq := func(p ...[]byte) []byte { return x509TestTLV(0x30, p...) }
	oid := []byte{6, 3, 0x2a, 3, 4}
	attribute := seq([]byte{6, 3, 0x55, 4, 3}, x509TestTLV(12, []byte("example")))
	name := seq(x509TestTLV(0x31, attribute))
	other := x509TestTLV(0xa0, oid, x509TestTLV(0xa0, x509TestTLV(12, []byte("value"))))
	directory := x509TestTLV(0xa4, name)
	edi := x509TestTLV(0xa5, x509TestTLV(0xa0, x509TestTLV(12, []byte("assigner"))), x509TestTLV(0xa1, x509TestTLV(30, []byte{0, 65})))
	x400 := x509TestTLV(0xa3, seq())
	san := seq(other, directory, edi, x400)
	f, info, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "2.5.29.17", san))
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, f, len(x509ExtensionTestCertificate(t, "2.5.29.17", san)))
	names := info["Extension Details"].([]map[string]any)[0]["Names"].([]map[string]any)
	require.Equal(t, false, names[0]["Type Specific Fields Decoded"])
	require.Equal(t, false, names[3]["Type Specific Fields Decoded"])
	for _, bad := range [][]byte{x509TestTLV(0xa0, oid), x509TestTLV(0xa4, []byte{5, 0}), x509TestTLV(0xa5, x509TestTLV(0xa0, []byte{12, 0}))} {
		_, _, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "2.5.29.17", seq(bad)))
		require.Error(t, err)
	}
	cpsOID, err := asn1.Marshal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 2, 1})
	require.NoError(t, err)
	noticeOID, err := asn1.Marshal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 2, 2})
	require.NoError(t, err)
	notice := seq(seq(x509TestTLV(12, []byte("organization")), seq([]byte{2, 1, 7})), x509TestTLV(26, []byte("notice")))
	policies := seq(seq(oid, seq(seq(cpsOID, x509TestTLV(22, []byte("https://example.invalid/policy"))), seq(noticeOID, notice))))
	_, _, err = decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "2.5.29.32", policies))
	require.NoError(t, err)
	for _, bad := range [][]byte{seq(cpsOID, []byte{12, 0}), seq(noticeOID, seq(seq([]byte{5, 0}, seq()))), seq(noticeOID, seq([]byte{5, 0}))} {
		_, _, err := decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "2.5.29.32", seq(seq(oid, seq(bad)))))
		require.Error(t, err)
	}
	relative := seq(seq(x509TestTLV(0xa0, x509TestTLV(0xa1, attribute)), x509TestTLV(0xa2, directory)))
	f, _, err = decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "2.5.29.31", relative))
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, f, len(x509ExtensionTestCertificate(t, "2.5.29.31", relative)))
	// Embedded values share the outer reader's depth and element budgets.
	deep := []byte{5, 0}
	for i := 0; i < x509DERMaxDepth; i++ {
		deep = seq(deep)
	}
	_, _, err = decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "2.5.29.32", seq(seq(oid, seq(seq(oid, deep))))))
	require.ErrorContains(t, err, "resource limit")
	many := seq(bytes.Repeat([]byte{5, 0}, x509DERMaxNodes))
	_, _, err = decodeX509CertificateDERExtensions(x509ExtensionTestCertificate(t, "2.5.29.32", seq(seq(oid, seq(seq(oid, many))))))
	require.ErrorContains(t, err, "resource limit")
}
