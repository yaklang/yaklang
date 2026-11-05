package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLS13ServerHelloLegacyParameters(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for server := 0; server < 2; server++ {
			for _, observedClient := range []bool{false, true} {
				for _, vector := range []struct {
					name        string
					version     uint16
					compression byte
				}{
					{"version-zero", 0, 0}, {"version-TLS10", 0x301, 0},
					{"version-TLS11", 0x302, 0}, {"version-TLS13", 0x304, 0},
					{"version-unknown", 0xffff, 0}, {"compression-deflate", 0x303, 1},
					{"compression-unknown", 0x303, 0xff},
				} {
					t.Run(fmt.Sprintf("%04x/server%d/client%t/%s", suite, server, observedClient, vector.name), func(t *testing.T) {
						s := newBinTLS()
						sid := []byte("compatibility")
						if observedClient {
							_, err := s.handshakeMessage(1-server, reliabilityHS(1, tlsResumptionHello(true, sid, suite, tlsResumptionExt{43, []byte{2, 3, 4}})), false)
							require.NoError(t, err)
						}
						body := tlsResumptionHello(false, sid, suite, tlsResumptionExt{43, []byte{3, 4}})
						binary.BigEndian.PutUint16(body, vector.version)
						body[37+len(sid)] = vector.compression
						before := *s
						m, err := s.handshakeMessage(server, reliabilityHS(2, body), false)
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, err.Error(), "TLS 1.3 ServerHello legacy parameters")
						require.Nil(t, m)
						require.Equal(t, before, *s, "a malformed complete hello cannot change negotiated state")
					})
				}
			}
		}
	}
}

func TestTLS13ServerHelloLegacyPartialContextControls(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for server := 0; server < 2; server++ {
			for _, sidLength := range []int{0, 16, 32} {
				t.Run(fmt.Sprintf("%04x/server%d/session%d", suite, server, sidLength), func(t *testing.T) {
					s := newBinTLS()
					_, err := s.handshakeMessage(server, reliabilityHS(2, tlsResumptionHello(false, bytes.Repeat([]byte{0x51}, sidLength), suite, append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)...)), false)
					require.NoError(t, err, "missing ClientHello does not require an invented session echo")
					require.Equal(t, uint16(0x304), s.version)
					require.Equal(t, suite, s.suite)
					require.Equal(t, -1, s.client)
					require.True(t, s.finalHello)
				})
			}
		}
	}
	for _, version := range []uint16{0x301, 0x302, 0x303} {
		t.Run(fmt.Sprintf("TLS-before13/%04x", version), func(t *testing.T) {
			s := newBinTLS()
			body := tlsResumptionHello(false, nil, 0x002f)
			binary.BigEndian.PutUint16(body, version)
			_, err := s.handshakeMessage(1, reliabilityHS(2, body), false)
			require.NoError(t, err)
			require.Equal(t, version, s.version)
		})
	}
}

func TestTLS13ServerHelloLegacyRetryWithoutClient(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for server := 0; server < 2; server++ {
			for _, variant := range []string{"valid-unknown-client", "version", "compression"} {
				t.Run(fmt.Sprintf("%04x/server%d/%s", suite, server, variant), func(t *testing.T) {
					s := newBinTLS()
					body := tlsResumptionHello(false, nil, suite, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, []byte{0, 29}})
					copy(body[2:34], tlsRetryRandom[:])
					if variant == "version" {
						binary.BigEndian.PutUint16(body, 0x302)
					}
					if variant == "compression" {
						body[37] = 1
					}
					before := *s
					m, err := s.handshakeMessage(server, reliabilityHS(2, body), false)
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					if variant == "valid-unknown-client" {
						require.Equal(t, ErrContextRequired, typed.Kind, "missing ClientHello still prevents validating the retry branch")
					} else {
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, err.Error(), "TLS 1.3 ServerHello legacy parameters")
					}
					require.Nil(t, m)
					require.Equal(t, before, *s)
				})
			}
		}
	}
}

func FuzzTLS13ServerHelloLegacyParameters(f *testing.F) {
	for _, seed := range []struct {
		version     uint16
		compression byte
	}{{0x303, 0}, {0x302, 0}, {0x303, 1}, {0x304, 0xff}, {0, 0xff}, {0xffff, 0}} {
		f.Add(seed.version, seed.compression, byte(0), byte(0))
	}
	f.Fuzz(func(t *testing.T, version uint16, compression, direction, sidLength byte) {
		s := newBinTLS()
		body := tlsResumptionHello(false, bytes.Repeat([]byte{0x51}, int(sidLength)%33), 0x1302, append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)...)
		binary.BigEndian.PutUint16(body, version)
		body[37+int(sidLength)%33] = compression
		before := *s
		m, err := s.handshakeMessage(int(direction)%2, reliabilityHS(2, body), false)
		if version != 0x303 || compression != 0 {
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.Nil(t, m)
			require.Equal(t, before, *s)
		} else {
			require.NoError(t, err)
			require.NotNil(t, m)
			require.Equal(t, uint16(0x304), s.version)
		}
	})
}
