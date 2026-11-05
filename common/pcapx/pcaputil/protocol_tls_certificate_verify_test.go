package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Public synthetic signature bytes exercise framing and ownership, not valid
// transcript signatures. Actual mTLS/PHA captures have a separate source oracle.
func certificateVerifyTestMessage(version, algorithm uint16, signature []byte) []byte {
	body := make([]byte, 2)
	if version >= 0x303 {
		body = append(body, 0, 0)
		binary.BigEndian.PutUint16(body, algorithm)
	}
	binary.BigEndian.PutUint16(body[len(body)-2:], uint16(len(signature)))
	body = append(body, signature...)
	return append([]byte{15, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
}

func TestTLSCertificateVerifyBodyLiteralsAndOwnership(t *testing.T) {
	for _, version := range []uint16{0x301, 0x302, 0x303, 0x304} {
		t.Run(fmt.Sprintf("%04x", version), func(t *testing.T) {
			wire := certificateVerifyTestMessage(version, 0x0804, bytes.Repeat([]byte{0x93}, 256))
			s := &binTLS{version: version, client: 0}
			m, err := s.handshakeMessage(0, wire, version == 0x304)
			require.NoError(t, err)
			require.Equal(t, version, m["Certificate Verify Version"])
			require.Equal(t, wire, m["Encoded Certificate Verify"])
			body, ok := m["Certificate Verify"].(map[string]any)
			require.True(t, ok, "complete CertificateVerify must expose its version-qualified body")
			require.Equal(t, uint16(256), body["Signature Length"])
			require.Equal(t, bytes.Repeat([]byte{0x93}, 256), body["Signature"])
			if version >= 0x303 {
				require.Equal(t, uint16(0x0804), body["Signature Algorithm"])
			} else {
				require.NotContains(t, body, "Signature Algorithm", "legacy TLS has no algorithm bytes here")
			}
			require.Contains(t, m["Certificate Verify Status"], "signature not evaluated")
			e := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Handshake Messages": []map[string]any{m}}}
			values := e.DisplayFields()
			require.Equal(t, []any{uint64(256)}, values["tls.handshake.client_cert_vrfy.sig_len"])
			require.Equal(t, []any{fmt.Sprintf("%x", bytes.Repeat([]byte{0x93}, 256))}, values["tls.handshake.client_cert_vrfy.sig"])
			clear(wire)
			require.Equal(t, bytes.Repeat([]byte{0x93}, 256), body["Signature"])
			require.Equal(t, byte(15), m["Encoded Certificate Verify"].([]byte)[0])
		})
	}
}

func TestTLSCertificateVerifyInvalidVectorsDoNotPublish(t *testing.T) {
	for _, version := range []uint16{0x301, 0x302, 0x303, 0x304} {
		wire := certificateVerifyTestMessage(version, 0x0403, []byte{0x30, 0x01, 0x00})
		for _, kind := range []string{"missing-length", "short-signature", "trailing-byte", "oversized-length"} {
			t.Run(fmt.Sprintf("%04x/%s", version, kind), func(t *testing.T) {
				bad := bytes.Clone(wire)
				switch kind {
				case "missing-length":
					bad = bad[:5]
				case "short-signature":
					bad = bad[:len(bad)-1]
				case "trailing-byte":
					bad = append(bad, 0xff)
				case "oversized-length":
					// A complete CV envelope with an invalid inner size.
					start := 4
					if version >= 0x303 {
						start = 6
					}
					binary.BigEndian.PutUint16(bad[start:], 0xffff)
				}
				bad[1], bad[2], bad[3] = 0, 0, byte(len(bad)-4)
				s := &binTLS{version: version, client: 0, seq: [2]uint64{3, 7}}
				before := *s
				m, err := s.handshakeMessage(0, bad, version == 0x304)
				require.Error(t, err)
				require.Nil(t, m)
				require.Equal(t, before, *s, "rejected body cannot alter connection state")
			})
		}
	}
}

func TestTLSCertificateVerifyVersionAuthenticationAndRole(t *testing.T) {
	wire := certificateVerifyTestMessage(0x304, 0x0804, []byte{1, 2, 3})
	for _, version := range []uint16{0, 0x300, 0x305} {
		s := &binTLS{version: version, client: -1}
		m, err := s.handshakeMessage(0, wire, true)
		require.NoError(t, err)
		require.NotContains(t, m, "Certificate Verify")
		require.NotContains(t, m, "Encoded Certificate Verify")
		require.Contains(t, m["Certificate Verify Status"], "uninterpreted")
	}
	for _, spec := range []struct {
		version uint16
		dir     int
		auth    bool
		app     bool
	}{{0x304, 0, false, false}, {0x303, 1, false, false}, {0x304, 1, true, true}} {
		s := &binTLS{version: spec.version, client: 0}
		s.app[spec.dir] = spec.app
		m, err := s.handshakeMessage(spec.dir, wire, spec.auth)
		require.Error(t, err)
		require.Nil(t, m)
	}
}

func TestTLSCertificateVerifyFragmentedApplicationEpoch(t *testing.T) {
	wire := certificateVerifyTestMessage(0x304, 0x0807, bytes.Repeat([]byte{0x72}, 64))
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, split := range []int{1, 3, 4, 5, 6, 7, 8, 9, 35, len(wire) - 1} {
			t.Run(fmt.Sprintf("%04x/%d", suite, split), func(t *testing.T) {
				f, s := boundaryTLSFlow(t, suite)
				key, secret := s.keys[0], bytes.Clone(s.secret[0])
				a := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, s, 0, 2, wire[:split], 22, 0)}
				_, err := f.consumeTLS(0, a)
				require.NoError(t, err)
				require.Empty(t, a.Session["Handshake Messages"])
				b := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, s, 0, 3, wire[split:], 22, 5)}
				_, err = f.consumeTLS(0, b)
				require.NoError(t, err)
				m := b.Session["Handshake Messages"].([]map[string]any)[0]
				require.Equal(t, wire, m["Encoded Certificate Verify"])
				require.Equal(t, "application", m["Certificate Verify Phase"])
				require.Same(t, key, s.keys[0])
				require.Equal(t, secret, s.secret[0])
				require.Equal(t, uint64(4), s.seq[0])
				require.Equal(t, uint64(2), s.seq[1])
			})
		}
	}
}

func TestTLSCertificateVerifyEnvelopeLimitsAndLiteralUnknownScheme(t *testing.T) {
	for _, version := range []uint16{0x301, 0x302, 0x303, 0x304} {
		for _, n := range []int{0, 1, 64, 65535} {
			t.Run(fmt.Sprintf("%04x/%d", version, n), func(t *testing.T) {
				wire := certificateVerifyTestMessage(version, 0xfe31, bytes.Repeat([]byte{0xa7}, n))
				view, err := DecodeTLSCertificateVerify(wire, version)
				require.NoError(t, err, "body grammar preserves an unknown scheme and an empty opaque vector without claiming validity")
				require.Equal(t, uint16(n), view.Fields["Signature Length"])
				require.Equal(t, n, len(view.Fields["Signature"].([]byte)))
				if version >= 0x303 {
					require.Equal(t, uint16(0xfe31), view.Fields["Signature Algorithm"])
				}
				for _, bad := range [][]byte{wire[:len(wire)-1], append(bytes.Clone(wire), 0), append([]byte{13}, wire[1:]...)} {
					_, err = DecodeTLSCertificateVerify(bad, version)
					require.Error(t, err)
				}
			})
		}
	}
	for _, version := range []uint16{0, 0x300, 0x305, 0xffff} {
		_, err := DecodeTLSCertificateVerify(certificateVerifyTestMessage(0x304, 0x0804, nil), version)
		require.Error(t, err)
	}
}

func TestTLSCertificateVerifySharedAlgorithmOccurrenceOrderAndRanges(t *testing.T) {
	a := certificateVerifyTestMessage(0x304, 0x0804, []byte{0x93})
	b := certificateVerifyTestMessage(0x304, 0xfefe, nil)
	va, err := DecodeTLSCertificateVerify(a, 0x304)
	require.NoError(t, err)
	vb, err := DecodeTLSCertificateVerify(b, 0x304)
	require.NoError(t, err)
	e := &ProtocolEvent{Protocol: "tls", Raw: bytes.Repeat([]byte{0xcc}, 512), Session: map[string]any{"Handshake Messages": []map[string]any{
		{"Certificate Verify": va.Fields},
		{"Certificate Request": map[string]any{"Signature Algorithms": []map[string]any{{"Algorithm": uint16(0x0403)}, {"Algorithm": uint16(0x0807)}}}},
		{"Certificate Verify": vb.Fields},
	}}}
	fields := e.DisplayFieldValues()
	require.Equal(t, []any{uint64(0x0804), uint64(0x0403), uint64(0x0807), uint64(0xfefe)}, fields.FieldMap()["tls.handshake.sig_hash_alg"])
	require.Equal(t, []any{"93", ""}, fields.FieldMap()["tls.handshake.client_cert_vrfy.sig"])
	for _, v := range fields.Values {
		if v.Name == "tls.handshake.sig_hash_alg" || v.Name == "tls.handshake.client_cert_vrfy.sig_len" || v.Name == "tls.handshake.client_cert_vrfy.sig" {
			require.Equal(t, "derived", v.Origin)
			require.Nil(t, v.Range, "a decrypted or reassembled signature has no ciphertext-record range")
		}
	}
}

func FuzzTLSCertificateVerifyBody(f *testing.F) {
	f.Add(uint16(0x304), certificateVerifyTestMessage(0x304, 0x0804, bytes.Repeat([]byte{0xa7}, 32)))
	f.Add(uint16(0x303), certificateVerifyTestMessage(0x303, 0x0403, []byte{0x30, 0x00}))
	f.Add(uint16(0x301), certificateVerifyTestMessage(0x301, 0, nil))
	f.Add(uint16(0x304), []byte{15, 0, 0, 4, 8, 4, 0xff, 0xff})
	f.Add(uint16(0), []byte{15, 0, 0, 0})
	f.Fuzz(func(t *testing.T, version uint16, wire []byte) {
		if len(wire) > 1<<17 {
			return
		}
		view, err := DecodeTLSCertificateVerify(wire, version)
		if err != nil {
			require.Nil(t, view)
			return
		}
		require.True(t, version >= 0x301 && version <= 0x304)
		require.Equal(t, byte(15), wire[0])
		require.Equal(t, len(wire)-4, tlsU24(wire[1:4]))
		start := 6
		if version >= 0x303 {
			start = 8
			require.Equal(t, binary.BigEndian.Uint16(wire[4:6]), view.Fields["Signature Algorithm"])
		} else {
			require.NotContains(t, view.Fields, "Signature Algorithm")
		}
		require.Equal(t, start, view.SignatureOffset)
		signature := view.Fields["Signature"].([]byte)
		require.Equal(t, uint16(len(signature)), view.Fields["Signature Length"])
		require.Equal(t, hex.EncodeToString(wire[start:]), hex.EncodeToString(signature))
		expected := bytes.Clone(signature)
		clear(wire)
		require.Equal(t, expected, signature, "source mutation cannot alter a published signature")
	})
}
