package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func quicTLSLevelMessage(typ byte, body []byte) []byte {
	return append([]byte{typ, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
}

func quicTLSLevelClientHello(t *testing.T) []byte {
	t.Helper()
	q := &binQUIC{native: true, byteLimit: 1 << 20}
	_, err := q.consume(0, rfc9001ClientInitial(), 4096)
	require.NoError(t, err)
	b := q.cryptoData[0][quicSpaceInitial].data
	require.GreaterOrEqual(t, len(b), 4)
	require.Equal(t, byte(1), b[0])
	n := 4 + tlsU24(b[1:4])
	require.LessOrEqual(t, n, len(b))
	return bytes.Clone(b[:n])
}

func TestQUICTLSHandshakeEncryptionLevel(t *testing.T) {
	// The vectors have complete TLS bodies. QUIC must refuse a wrong level
	// before an otherwise successful TLS decoder can publish or change state.
	request := certificateRequestFixture(t, true)
	verify := certificateVerifyTestMessage(0x304, 0x0807, bytes.Repeat([]byte{0x72}, 64))
	ticket := quicTLSLevelMessage(4, []byte{0, 0, 0, 60, 0, 0, 0, 7, 0, 0, 1, 0xa7, 0, 0})
	ee := quicTLSLevelMessage(8, []byte{0, 9, 0, 16, 0, 5, 0, 3, 2, 'h', '3'})
	for _, spec := range []struct {
		name       string
		dir, space int
		wire       []byte
	}{
		{"application-CertificateRequest", 1, quicSpaceApplication, request},
		{"application-CertificateVerify", 0, quicSpaceApplication, verify},
		{"application-Finished", 0, quicSpaceApplication, finishedTestMessage(32)},
		{"application-EncryptedExtensions", 1, quicSpaceApplication, ee},
		{"handshake-NewSessionTicket", 1, quicSpaceHandshake, ticket},
		{"handshake-ClientHello", 0, quicSpaceHandshake, quicTLSLevelClientHello(t)},
		{"application-ClientHello", 0, quicSpaceApplication, quicTLSLevelClientHello(t)},
		{"initial-EndOfEarlyData", 0, quicSpaceInitial, quicTLSLevelMessage(5, nil)},
		{"handshake-EndOfEarlyData", 0, quicSpaceHandshake, quicTLSLevelMessage(5, nil)},
		{"application-EndOfEarlyData", 0, quicSpaceApplication, quicTLSLevelMessage(5, nil)},
		{"application-KeyUpdate", 0, quicSpaceApplication, quicTLSLevelMessage(24, []byte{0})},
		{"handshake-KeyUpdate", 0, quicSpaceHandshake, quicTLSLevelMessage(24, []byte{0})},
		{"initial-unknown-type", 0, quicSpaceInitial, quicTLSLevelMessage(255, nil)},
	} {
		t.Run(spec.name, func(t *testing.T) {
			state := &binTLS{client: 0, version: 0x304, suite: 0x1301, app: [2]bool{true, true}, alpn: "h2", seq: [2]uint64{3, 7}}
			state.secret[0] = bytes.Repeat([]byte{0x39}, 32)
			q := &binQUIC{native: true, byteLimit: 1 << 20, tls: state, alpn: "h2"}
			before := *state
			info := map[string]any{}
			err := q.feedCrypto(spec.dir, spec.space, 0, spec.wire, info, 4096)
			require.Error(t, err, "packet authentication alone cannot authorize a TLS handshake at this QUIC level")
			require.NotContains(t, info, "TLS Handshakes")
			require.Equal(t, 0, q.cryptoRead[spec.dir][spec.space])
			require.Equal(t, before, *q.tls)
			require.Equal(t, "h2", q.alpn)
		})
	}
}

func TestQUICCryptoMessageBatchFailureDoesNotCommitTLS(t *testing.T) {
	clientHello := quicTLSLevelClientHello(t)
	for _, suffix := range [][]byte{quicTLSLevelMessage(2, nil), {2, 0x10, 0, 0}} {
		t.Run(fmt.Sprintf("suffix-%x", suffix), func(t *testing.T) {
			state := &binTLS{client: -1, resumption: tlsResumptionState{context: map[string]any{"Earlier": "kept"}}}
			q := &binQUIC{native: true, byteLimit: 1 << 20, tls: state}
			before := *state
			info := map[string]any{}
			err := q.feedCrypto(0, quicSpaceInitial, 0, append(bytes.Clone(clientHello), suffix...), info, 4096)
			require.Error(t, err)
			require.Equal(t, before, *q.tls, "valid prefix cannot leave a half-committed ClientHello after the same batch fails")
			require.Zero(t, q.cryptoRead[0][quicSpaceInitial])
			require.NotContains(t, info, "TLS Handshakes")
			require.Empty(t, q.alpn)
		})
	}
}

func TestQUICCryptoHandshakeFragmentationStaysWithinLevel(t *testing.T) {
	clientHello := quicTLSLevelClientHello(t)
	for _, split := range []int{1, 3, 4, 17, len(clientHello) - 1} {
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 1 << 20}
			info := map[string]any{}
			require.NoError(t, q.feedCrypto(0, quicSpaceInitial, uint64(split), clientHello[split:], info, 4096))
			require.NotContains(t, info, "TLS Handshakes")
			require.Zero(t, q.cryptoRead[0][quicSpaceInitial])
			info = map[string]any{}
			require.NoError(t, q.feedCrypto(0, quicSpaceInitial, 0, clientHello[:split], info, 4096))
			require.Len(t, info["TLS Handshakes"], 1)
			require.Equal(t, len(clientHello), q.cryptoRead[0][quicSpaceInitial])
			require.Equal(t, 0, q.tls.client)
			require.Len(t, q.tls.random, 32)
			require.NoError(t, q.feedCrypto(0, quicSpaceInitial, 0, clientHello, map[string]any{}, 4096), "exact retransmission is idempotent")
		})
	}
}

func TestQUICTLSVersionIsNotBorrowedFromLegacyRecord(t *testing.T) {
	client := func(versions []byte) []byte {
		var extensions []tlsResumptionExt
		if versions != nil {
			extensions = append(extensions, tlsResumptionExt{43, versions})
		}
		return quicTLSLevelMessage(1, tlsResumptionHello(true, nil, 0x1301, extensions...))
	}
	server := func(versions []byte) []byte {
		var extensions []tlsResumptionExt
		if versions != nil {
			extensions = append(extensions, tlsResumptionExt{43, versions})
		}
		return quicTLSLevelMessage(2, tlsResumptionHello(false, nil, 0x1301, extensions...))
	}
	for _, spec := range []struct {
		name string
		dir  int
		wire []byte
		kind ProtocolErrorKind
	}{
		{"client-legacy-only", 0, client(nil), ErrMalformedMessage},
		{"client-offers-TLS12", 0, client([]byte{2, 3, 3}), ErrMalformedMessage},
		{"client-offers-TLS13-and-TLS12", 0, client([]byte{4, 3, 4, 3, 3}), ErrMalformedMessage},
		{"server-legacy-only", 1, server(nil), ErrMalformedMessage},
		{"server-selects-TLS12", 1, server([]byte{3, 3}), ErrMalformedMessage},
		{"client-only-unselected-future-version", 0, client([]byte{2, 3, 5}), ErrUnsupportedVersion},
		{"server-unselected-future-version", 1, server([]byte{3, 5}), ErrUnsupportedVersion},
	} {
		t.Run(spec.name, func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 1 << 20, tls: newBinTLS()}
			before := *q.tls
			info := map[string]any{}
			err := q.feedCrypto(spec.dir, quicSpaceInitial, 0, spec.wire, info, 4096)
			require.Error(t, err)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, spec.kind, typed.Kind)
			require.Equal(t, before, *q.tls)
			require.Zero(t, q.cryptoRead[spec.dir][quicSpaceInitial])
			require.NotContains(t, info, "TLS Handshakes")
		})
	}
	// Newer versions can be offered beside the supported version. This does not
	// certify all transport parameters, TLS negotiation or future body grammars.
	q := &binQUIC{native: true, byteLimit: 1 << 20}
	info := map[string]any{}
	require.NoError(t, q.feedCrypto(0, quicSpaceInitial, 0, client([]byte{6, 3, 4, 0x0a, 0x0a, 3, 5}), info, 4096))
	require.Len(t, info["TLS Handshakes"], 1)
}

func TestQUICNewSessionTicketEarlyDataBound(t *testing.T) {
	for _, size := range []uint32{0, 1, 16384, 0xfffffffe, 0xffffffff} {
		t.Run(fmt.Sprintf("%08x", size), func(t *testing.T) {
			data := binary.BigEndian.AppendUint32(nil, size)
			wire := quicTLSLevelMessage(4, tlsResumptionNST13(60, nil, []byte{0xa7}, tlsResumptionExt{42, data}))
			state := &binTLS{client: 0, version: 0x304, suite: 0x1301, app: [2]bool{true, true}}
			q := &binQUIC{native: true, byteLimit: 1 << 20, tls: state}
			before := *state
			info := map[string]any{}
			err := q.feedCrypto(1, quicSpaceApplication, 0, wire, info, 128)
			if size == 0xffffffff {
				require.NoError(t, err)
				require.Len(t, info["TLS Handshakes"], 1)
			} else {
				require.Error(t, err, "RFC9001 requires the QUIC early_data maximum to be all ones")
				require.Equal(t, before, *q.tls)
				require.Zero(t, q.cryptoRead[1][quicSpaceApplication])
				require.NotContains(t, info, "TLS Handshakes")
			}
			// These same non-all-ones values remain legal TLS over TCP body
			// observations. The restriction belongs to QUIC, not shared TLS.
			_, err = (&binTLS{client: 0, version: 0x304, app: [2]bool{true, true}}).handshakeMessage(1, wire, true, 128)
			require.NoError(t, err)
		})
	}
}

func TestQUICKnownCompressedCertificateEnvelopeUsesHandshakeLevel(t *testing.T) {
	// No decompression claim: the known type must not be rejected just because
	// its compressed body profile is not implemented. It still has one level.
	wire := quicTLSLevelMessage(25, []byte{0, 1, 0, 0, 3, 0, 0, 1, 0})
	for _, space := range []int{quicSpaceInitial, quicSpaceHandshake, quicSpaceApplication} {
		t.Run(fmt.Sprint(space), func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 1024, tls: &binTLS{client: 0, version: 0x304}}
			info := map[string]any{}
			err := q.feedCrypto(1, space, 0, wire, info, 128)
			if space == quicSpaceHandshake {
				require.NoError(t, err)
				require.Len(t, info["TLS Handshakes"], 1)
			} else {
				require.Error(t, err)
				require.NotContains(t, info, "TLS Handshakes")
			}
		})
	}
}

func TestQUICAuthenticatedWrongLevelCannotBecomeCleanDuplicate(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, typ := range []byte{13, 15, 20, 24} {
				t.Run(fmt.Sprintf("v%d/%s/type%d", version, algorithm, typ), func(t *testing.T) {
					q, key := quicConformanceContext(t, version, algorithm)
					q.byteLimit = 1 << 20
					suite, hashSize := uint16(0x1301), 32
					if algorithm == quicAEADAES256GCM {
						suite, hashSize = 0x1302, 48
					}
					if algorithm == quicAEADChaCha20Poly1305 {
						suite = 0x1303
					}
					q.tls = &binTLS{client: 0, version: 0x304, suite: suite, app: [2]bool{true, true}}
					q.tls.secret[0] = bytes.Repeat([]byte{0x29}, hashSize)
					wire := quicTLSLevelMessage(24, []byte{0})
					dir := 0
					switch typ {
					case 13:
						wire = certificateRequestFixture(t, true)
						dir = 1
					case 15:
						wire = certificateVerifyTestMessage(0x304, 0x0807, bytes.Repeat([]byte{0x72}, 64))
					case 20:
						wire = finishedTestMessage(hashSize)
					}
					before := *q.tls
					plain := append([]byte{6, 0}, quicPutVarint(uint64(len(wire)))...)
					plain = append(plain, wire...)
					packet := quicConformancePacket(t, key, "1-RTT", 100, 0, plain)
					info, err := q.consume(dir, packet, 4096)
					require.Error(t, err)
					require.Equal(t, true, info["Authentication Verified"])
					require.NotContains(t, info, "TLS Handshakes")
					require.Equal(t, before, *q.tls)
					require.Equal(t, plain, info["Rejected Packet Payload"])
					require.Contains(t, info["Payload Delivery"], "parsing stopped")
					info["Rejected Packet Payload"].([]byte)[0] ^= 0xff
					first := err.(*ProtocolError)
					kind, message := first.Kind, first.Message
					first.Message = "caller changed returned error"
					info, err = q.consume(dir, packet, 4096)
					require.Error(t, err)
					require.Equal(t, true, info["Duplicate"])
					require.Equal(t, kind, err.(*ProtocolError).Kind)
					require.Equal(t, message, err.(*ProtocolError).Message)
					require.Equal(t, plain, info["Rejected Packet Payload"])
					require.NotContains(t, info, "TLS Handshakes")
					require.Equal(t, before, *q.tls)
				})
			}
		}
	}
}

func TestQUICCryptoInvalidNamespaceDoesNotPanic(t *testing.T) {
	for _, index := range [][2]int{{-1, 0}, {2, 0}, {0, -1}, {0, 3}} {
		q := &binQUIC{native: true, byteLimit: 1024}
		require.Error(t, q.feedCrypto(index[0], index[1], 0, nil, map[string]any{}, 128))
		require.Nil(t, q.tls)
	}
}

func TestQUICHandshakeCRYPTOCannotExtendObservedFinished(t *testing.T) {
	finished := finishedTestMessage(32)
	for _, dir := range []int{0, 1} {
		for _, mode := range []string{"same-delivery-prefix", "later-prefix", "later-message", "pending-beyond-Finished"} {
			t.Run(fmt.Sprintf("dir%d/%s", dir, mode), func(t *testing.T) {
				q := &binQUIC{native: true, byteLimit: 8192, tls: &binTLS{client: 0, version: 0x304, suite: 0x1301}, alpn: "h2"}
				if mode == "pending-beyond-Finished" {
					require.NoError(t, q.feedCrypto(dir, quicSpaceHandshake, uint64(len(finished)+10), []byte{1}, map[string]any{}, 128))
				}
				data, offset := append(bytes.Clone(finished), 15), uint64(0)
				if mode == "pending-beyond-Finished" {
					data = finished
				}
				if mode == "later-prefix" || mode == "later-message" {
					require.NoError(t, q.feedCrypto(dir, quicSpaceHandshake, 0, finished, map[string]any{}, 128))
					require.True(t, q.tls.app[dir])
					offset = uint64(len(finished))
					data = []byte{15}
					if mode == "later-message" {
						if dir == 0 {
							data = certificateVerifyTestMessage(0x304, 0x0807, bytes.Repeat([]byte{0x72}, 64))
						} else {
							data = quicTLSLevelMessage(8, []byte{0, 9, 0, 16, 0, 5, 0, 3, 2, 'h', '3'})
						}
					}
				}
				before, readBefore := *q.tls, q.cryptoRead[dir][quicSpaceHandshake]
				info := map[string]any{}
				err := q.feedCrypto(dir, quicSpaceHandshake, offset, data, info, 128)
				require.Error(t, err, "new bytes cannot extend the QUIC handshake CRYPTO stream after its Finished")
				require.Equal(t, before, *q.tls)
				require.Equal(t, readBefore, q.cryptoRead[dir][quicSpaceHandshake])
				require.Equal(t, "h2", q.alpn)
				require.NotContains(t, info, "TLS Handshakes")
			})
		}
	}
}

func FuzzQUICCryptoTLSQualification(f *testing.F) {
	f.Add(quicTLSLevelMessage(255, nil), uint8(0), uint8(0), true)
	f.Add(quicTLSLevelMessage(24, []byte{0}), uint8(2), uint8(0), false)
	f.Add(certificateVerifyTestMessage(0x304, 0x0807, bytes.Repeat([]byte{0x72}, 64)), uint8(2), uint8(0), false)
	f.Add(quicTLSLevelMessage(4, tlsResumptionNST13(60, nil, []byte{0xa7}, tlsResumptionExt{42, []byte{0xff, 0xff, 0xff, 0xff}})), uint8(2), uint8(1), false)
	f.Add(quicTLSLevelMessage(25, []byte{0, 1, 0, 0, 3, 0, 0, 1, 0}), uint8(1), uint8(1), false)
	f.Fuzz(func(t *testing.T, payload []byte, level, direction uint8, fresh bool) {
		if len(payload) > 4096 {
			return
		}
		space, dir := int(level%3), int(direction%2)
		state := &binTLS{client: 0, version: 0x304, suite: 0x1301, app: [2]bool{true, true}, alpn: "h2", resumption: tlsResumptionState{context: map[string]any{"Earlier": "kept"}}}
		state.secret[0], state.secret[1] = bytes.Repeat([]byte{0x29}, 32), bytes.Repeat([]byte{0x39}, 32)
		if fresh {
			state.client, state.version, state.suite = -1, 0, 0
			state.app = [2]bool{}
		}
		q := &binQUIC{native: true, byteLimit: 8192, tls: state, alpn: "h2"}
		before := *state
		before.resumption.context = cloneSession(state.resumption.context)
		original := bytes.Clone(payload)
		info := map[string]any{}
		err := q.feedCrypto(dir, space, 0, payload, info, 128)
		require.Equal(t, original, payload, "parsing cannot change caller-owned bytes")
		if err != nil {
			require.Equal(t, before, *q.tls)
			require.Zero(t, q.cryptoRead[dir][space])
			require.NotContains(t, info, "TLS Handshakes")
			require.Equal(t, "h2", q.alpn)
		} else {
			messages, _ := info["TLS Handshakes"].([]map[string]any)
			for _, m := range messages {
				typ := m["Type"].(byte)
				switch space {
				case 0:
					require.True(t, typ == 1 || typ == 2)
				case 1:
					require.Contains(t, []byte{8, 11, 13, 15, 20, 25}, typ)
				case 2:
					require.Equal(t, byte(4), typ)
				}
			}
		}
		retained := bytes.Clone(q.cryptoData[dir][space].data)
		for i := range payload {
			payload[i] ^= 0xff
		}
		require.Equal(t, retained, q.cryptoData[dir][space].data, "CRYPTO reassembly owns its stored input")
	})
}
