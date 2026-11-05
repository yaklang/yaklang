package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func eeScopeBody(extensions ...tlsResumptionExt) []byte {
	var b []byte
	for _, e := range extensions {
		b = append(b, certificateRequestExtension(e.kind, e.data)...)
	}
	return append(binary.BigEndian.AppendUint16(nil, uint16(len(b))), b...)
}

func eeScopeALPN(names ...string) []byte {
	var b []byte
	for _, name := range names {
		b = append(b, byte(len(name)))
		b = append(b, name...)
	}
	return append(binary.BigEndian.AppendUint16(nil, uint16(len(b))), b...)
}

func TestTLSEncryptedExtensionsScopeBeforeStateChanges(t *testing.T) {
	for _, profile := range []string{"version-selection", "key-share", "PSK-selection", "cookie", "PSK-modes", "signature-algorithms", "certificate-signature-algorithms", "padding", "client-direction", "TLS12", "application-epoch", "version-unavailable"} {
		t.Run(profile, func(t *testing.T) {
			state := newBinTLS()
			state.client, state.version, state.suite, state.alpn = 0, 0x304, 0x1301, "prior"
			dir := 1
			var ext []tlsResumptionExt
			switch profile {
			case "version-selection":
				ext = []tlsResumptionExt{{43, []byte{3, 3}}}
			case "key-share":
				ext = []tlsResumptionExt{{51, []byte{0, 29, 0, 1, 1}}}
			case "PSK-selection":
				ext = []tlsResumptionExt{{41, []byte{0, 0}}}
			case "cookie":
				ext = []tlsResumptionExt{{44, []byte{0, 1, 1}}}
			case "PSK-modes":
				ext = []tlsResumptionExt{{45, []byte{1, 1}}}
			case "signature-algorithms":
				ext = []tlsResumptionExt{{13, []byte{0, 2, 8, 4}}}
			case "certificate-signature-algorithms":
				ext = []tlsResumptionExt{{50, []byte{0, 2, 8, 4}}}
			case "padding":
				ext = []tlsResumptionExt{{21, nil}}
			case "client-direction":
				dir = 0
			case "TLS12":
				state.version = 0x303
			case "application-epoch":
				state.app[dir] = true
			case "version-unavailable":
				state.version = 0
			}
			before := *state
			_, err := state.handshakeMessage(dir, reliabilityHS(8, eeScopeBody(ext...)), true, 4096)
			require.Error(t, err, "a protected message cannot bypass handshake/extension placement")
			require.Equal(t, before, *state, "refusal must not change version, ALPN or any retained state")
		})
	}
}

func TestTLSEncryptedExtensionsRetainsObservedALPNOffer(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, profile := range []string{"offered", "unoffered", "absent-offer", "case-sensitive", "partial-context"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, profile), func(t *testing.T) {
					state := newBinTLS()
					state.client, state.version, state.suite = 1-server, 0x304, suite
					if profile != "partial-context" {
						state.client, state.version = -1, 0
						ext := append([]tlsResumptionExt{{43, []byte{2, 3, 4}}}, tlsResumptionDHExtensions(true)...)
						if profile != "absent-offer" {
							name := "h2"
							if profile == "case-sensitive" {
								name = "H2"
							}
							ext = append(ext, tlsResumptionExt{16, eeScopeALPN(name, "http/1.1")})
						}
						_, err := state.handshakeMessage(1-server, reliabilityHS(1, tlsResumptionHello(true, nil, suite, ext...)), false)
						require.NoError(t, err)
						_, err = state.handshakeMessage(server, reliabilityHS(2, tlsResumptionHello(false, nil, suite, append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)...)), false)
						require.NoError(t, err)
						require.Nil(t, state.firstHello, "offer knowledge must survive final ServerHello's body release")
					}
					selected := "h2"
					if profile == "unoffered" {
						selected = "h3"
					}
					before := *state
					_, err := state.handshakeMessage(server, reliabilityHS(8, eeScopeBody(tlsResumptionExt{16, eeScopeALPN(selected)})), true)
					if profile == "offered" || profile == "partial-context" {
						require.NoError(t, err)
						require.Equal(t, selected, state.alpn)
					} else {
						require.ErrorContains(t, err, "ALPN was not offered")
						require.Equal(t, before, *state)
					}
				})
			}
		}
	}
}

func TestTLSEncryptedExtensionsCollectionBudget(t *testing.T) {
	state := newBinTLS()
	state.client, state.version, state.suite = 0, 0x304, 0x1301
	before := *state
	_, err := state.handshakeMessage(1, reliabilityHS(8, eeScopeBody(tlsResumptionExt{0xaaa9, nil}, tlsResumptionExt{0xbbbb, nil})), true, 1)
	require.ErrorContains(t, err, "ResourceExceeded")
	require.Equal(t, before, *state)
}
