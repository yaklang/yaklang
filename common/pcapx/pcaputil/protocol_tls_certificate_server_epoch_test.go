package pcaputil

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSServerCertificateRequiresHandshakeEpoch(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, mode := range []string{"initial", "application", "recovered-application", "unauthenticated-recovery"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, mode), func(t *testing.T) {
					flow, state := boundaryTLSFlow(t, suite)
					state.client = 1 - server
					state.app[server] = mode == "application"
					encoded := reliabilityHS(11, certificateExtensionsBody(nil, certificateRequestExtension(0xff01, []byte{1, 2})))
					_, err := DecodeTLSCertificate(encoded, 0x304, 4096)
					require.NoError(t, err, "role-neutral body syntax remains inspectable")
					secret, iv, key := bytes.Clone(state.secret[server]), bytes.Clone(state.iv[server]), state.keys[server]
					seal, sequence := state, uint64(2)
					var applicationSecret []byte
					recovering := mode == "recovered-application" || mode == "unauthenticated-recovery"
					if recovering {
						trial := *state
						trial.app[server] = true
						applicationSecret = bytes.Repeat([]byte{0x7b}, len(secret))
						require.NoError(t, trial.install13(server, applicationSecret))
						provided := bytes.Clone(applicationSecret)
						if mode == "unauthenticated-recovery" {
							provided[0] ^= 1
						}
						provider, err := ParseTLSKeyLog("SERVER_TRAFFIC_SECRET_0 " + hex.EncodeToString(state.random) + " " + hex.EncodeToString(provided))
						require.NoError(t, err)
						flow.a.tlsSecrets = provider
						seal, sequence = &trial, 0
					}
					event := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, seal, server, sequence, encoded, 22, 0)}
					_, err = flow.consumeTLS(server, event)
					if mode == "initial" {
						require.NoError(t, err)
						messages := event.Session["Handshake Messages"].([]map[string]any)
						require.Len(t, messages, 1)
						require.Equal(t, encoded, messages[0]["Encoded Certificate"])
					} else {
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						kind := ErrMalformedMessage
						if mode == "unauthenticated-recovery" {
							kind = ErrAuthenticationFailed
						} else {
							require.Contains(t, typed.Error(), "server TLS Certificate")
							require.Contains(t, typed.Error(), "application")
						}
						require.Equal(t, kind, typed.Kind)
						require.NotContains(t, event.Session, "Handshake Messages")
						require.NotContains(t, event.DisplayFieldValues().FieldMap(), "tls.handshake.certificate")
					}
					require.Equal(t, 1-server, state.client)
					if mode == "unauthenticated-recovery" {
						require.False(t, state.app[server])
						require.Equal(t, uint64(3), state.seq[server], "the framed failure is isolated, not authenticated")
						require.True(t, state.recordGap[server])
						require.Equal(t, uint8(1), state.recoveryFailures[server])
						require.Equal(t, true, event.Session["Record Failure Isolated"])
						require.NotContains(t, event.Session, "Application Epoch Inferred From Authenticated Record")
						require.Equal(t, false, event.Session["Authentication Verified"])
						require.Equal(t, secret, state.secret[server])
						require.Equal(t, iv, state.iv[server])
						require.Same(t, key, state.keys[server])
					} else {
						require.Equal(t, mode != "initial", state.app[server])
						require.Equal(t, sequence+1, state.seq[server])
						require.Equal(t, true, event.Session["Authentication Verified"])
						if recovering {
							require.Equal(t, true, event.Session["Application Epoch Inferred From Authenticated Record"])
							require.Equal(t, applicationSecret, state.secret[server])
						}
					}
				})
			}
		}
	}
}

func TestTLSServerCertificateFragmentedEpochAdmission(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, mode := range []string{"initial", "application", "recovered-application"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, mode), func(t *testing.T) {
					flow, state := boundaryTLSFlow(t, suite)
					state.client = 1 - server
					state.app[server] = mode == "application"
					encoded := reliabilityHS(11, certificateExtensionsBody(nil, nil))
					const split = 7 // The certificate-list length crosses records.
					first := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, state, server, 2, encoded[:split], 22, 0)}
					_, err := flow.consumeTLS(server, first)
					require.NoError(t, err)
					require.Equal(t, true, first.Session["Authentication Verified"])
					require.Equal(t, true, first.Session["Handshake Incomplete"])
					require.Empty(t, first.Session["Handshake Messages"])
					require.Equal(t, encoded[:split], state.handshake[server])

					seal, sequence := state, uint64(3)
					if mode == "recovered-application" {
						trial := *state
						trial.app[server] = true
						secret := bytes.Repeat([]byte{0x7c}, len(state.secret[server]))
						require.NoError(t, trial.install13(server, secret))
						provider, err := ParseTLSKeyLog("SERVER_TRAFFIC_SECRET_0 " + hex.EncodeToString(state.random) + " " + hex.EncodeToString(secret))
						require.NoError(t, err)
						flow.a.tlsSecrets = provider
						seal, sequence = &trial, 0
					}
					last := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, seal, server, sequence, encoded[split:], 22, 0)}
					_, err = flow.consumeTLS(server, last)
					if mode == "initial" {
						require.NoError(t, err)
						messages := last.Session["Handshake Messages"].([]map[string]any)
						require.Len(t, messages, 1)
						require.Equal(t, encoded, messages[0]["Encoded Certificate"])
						require.Empty(t, state.handshake[server])
					} else if mode == "recovered-application" {
						// Recovery retires the old-epoch fragment only after the
						// new record authenticates. Its suffix cannot establish
						// a handshake boundary, even if a header fits by chance.
						require.NoError(t, err)
						require.Equal(t, split, last.Session["Discarded Handshake Fragment Bytes"])
						require.Empty(t, state.handshake[server])
						require.True(t, state.handshakeGap[server])
						require.Equal(t, "unavailable after authenticated key-epoch recovery", last.Session["Handshake Boundary"])
						require.Equal(t, "unavailable after missing protected handshake bytes", last.Session["Handshake Parsing"])
						require.NotContains(t, last.Session, "Handshake Messages")
						require.NotContains(t, last.DisplayFieldValues().FieldMap(), "tls.handshake.certificate")
					} else {
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, typed.Error(), "server TLS Certificate in application epoch")
						require.NotContains(t, last.Session, "Handshake Messages")
						require.NotContains(t, last.DisplayFieldValues().FieldMap(), "tls.handshake.certificate")
					}
					require.Equal(t, 1-server, state.client)
					require.Equal(t, sequence+1, state.seq[server])
					require.Equal(t, true, last.Session["Authentication Verified"])
					require.Equal(t, mode != "initial", state.app[server])
					if mode == "recovered-application" {
						require.Equal(t, true, last.Session["Application Epoch Inferred From Authenticated Record"])
					}
				})
			}
		}
	}
}
