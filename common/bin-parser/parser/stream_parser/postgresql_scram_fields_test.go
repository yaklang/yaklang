package stream_parser

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostgreSQLSCRAMLayoutsAndBounds(t *testing.T) {
	proof := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, tc := range []struct{ phase, mechanism, wire string }{
		{"client-first", "SCRAM-SHA-256", "n,,n=,r=nonce"},
		{"client-first", "SCRAM-SHA-256", "y,a=one=2Ctwo,n=user=3Dname,r=nonce,X=extra"},
		{"client-first", "SCRAM-SHA-256-PLUS", "p=tls-server-end-point,,n=user,r=nonce"},
		{"server-first", "", "r=nonce,s=c2FsdA==,i=4096,X=extra"},
		{"client-final", "", "c=biws,r=nonce,p=" + proof},
		{"client-final", "", "c=biws,r=nonce,X=extra,p=" + proof},
		{"server-final", "", "v=" + proof},
		{"server-final", "", "e=other-error,X=extra"},
	} {
		w := []byte(tc.wire)
		r := &postgresqlFieldsReader{wire: w, end: len(w)}
		info := map[string]any{"Mechanism": tc.mechanism}
		r.scram(info, tc.phase)
		require.NoError(t, r.err, tc.wire)
		tlsCertificateTestCoverage(t, r.fields, len(w))
		require.Equal(t, false, info["SCRAM Proof Verified"])
		attrs := info["SCRAM Attributes"].([]map[string]any)
		for _, a := range attrs {
			span := a["Relative Byte Range"].([2]int)
			require.Equal(t, w[span[0]:span[1]], a["Value"])
			require.NotContains(t, a, "Decoded Value")
		}
	}
	for _, tc := range []struct{ phase, mechanism, wire string }{
		{"client-first", "SCRAM-SHA-256", "p=tls-server-end-point,,n=user,r=nonce"},
		{"client-first", "SCRAM-SHA-256-PLUS", "n,,n=user,r=nonce"},
		{"client-first", "SCRAM-SHA-256-PLUS", "p=,,n=user,r=nonce"},
		{"client-first", "SCRAM-SHA-256", "n,a=,n=user,r=nonce"},
		{"client-first", "SCRAM-SHA-256", "n,,n=a=2c,r=nonce"},
		{"client-first", "SCRAM-SHA-256", "n,,m=required,n=user,r=nonce"},
		{"client-first", "SCRAM-SHA-256", "n,,r=nonce,n=user"},
		{"client-first", "SCRAM-SHA-256", "n,,n=user,r=nonce,n=duplicate"},
		{"server-first", "", "r=nonce,s=c2FsdA==,i=04096"},
		{"server-first", "", "r=nonce,s=c2FsdB==,i=4096"},
		{"server-first", "", "r=nonce,s=c2Fs\ndA==,i=4096"},
		{"server-first", "", "r=nonce,s=c2FsdA==,i=4294967296"},
		{"server-first", "", "r=nonce,s=c2FsdA==,i=+1"},
		{"server-first", "", "r=nonce,s=c2FsdA==,i=4096,"},
		{"server-first", "", "r=,s=c2FsdA==,i=4096"},
		{"server-first", "", "r=space nonce,s=c2FsdA==,i=4096"},
		{"client-final", "", "c=biws,r=nonce"},
		{"client-final", "", "c=biws,r=nonce,p="},
		{"client-final", "", "c=biws,r=nonce,p=eA=="},
		{"client-final", "", "c=biws,r=nonce,p=" + proof + ",X=afterproof"},
		{"server-final", "", "v=" + proof + ",e=twooutcomes"},
		{"server-final", "", "e="},
		{"server-final", "", "e=bad\x00value"},
		{"server-final", "", "e=bad\xffvalue"},
	} {
		r := &postgresqlFieldsReader{wire: []byte(tc.wire), end: len(tc.wire)}
		r.scram(map[string]any{"Mechanism": tc.mechanism}, tc.phase)
		require.Error(t, r.err, "%s", tc.wire)
	}
	// An unknown SASL mechanism remains an exact-length opaque response.
	p := append([]byte("EXAMPLE\x00\x00\x00\x00\x03"), 0, 255, 1)
	w := postgresqlFieldsTestTyped('p', p)
	fs, info, err := decodePostgreSQLFields(w, "sasl-initial")
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fs, len(w))
	require.NotContains(t, info, "SCRAM Attributes")
	w = postgresqlFieldsTestTyped('p', []byte("EXAMPLE\x00\xff\xff\xff\xff"))
	_, info, err = decodePostgreSQLFields(w, "sasl-initial")
	require.NoError(t, err)
	require.Equal(t, false, info["Initial Response Present"])
	// An iteration is just a bounded integer field, never a work factor here.
	r := &postgresqlFieldsReader{wire: []byte("r=nonce,s=c2FsdA==,i=4294967295"), end: len("r=nonce,s=c2FsdA==,i=4294967295")}
	info = map[string]any{}
	r.scram(info, "server-first")
	require.NoError(t, r.err)
	require.Equal(t, uint64(4294967295), info["SCRAM Attributes"].([]map[string]any)[2]["Integer"])
	// Shared data does not escape by alias through attribute metadata.
	w = []byte("r=nonce,s=c2FsdA==,i=4096")
	before := bytes.Clone(w)
	r = &postgresqlFieldsReader{wire: w, end: len(w)}
	info = map[string]any{}
	r.scram(info, "server-first")
	require.NoError(t, r.err)
	info["SCRAM Attributes"].([]map[string]any)[0]["Value"].([]byte)[0] = '!'
	require.Equal(t, before, w)
	_, _, err = decodePostgreSQLFields(postgresqlFieldsTestTyped('R', append([]byte{0, 0, 0, 11}, []byte(strings.Repeat("r=x,", 5000))...)), "backend-scram")
	require.Error(t, err)
}
