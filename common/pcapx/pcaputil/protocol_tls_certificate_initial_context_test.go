package pcaputil

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSCertificateInitialRequestContextAndRecoveredEpoch(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for _, c := range []struct {
				name, message string
				context       []byte
				application   bool
				recoverEpoch  bool
				refuse        bool
			}{
				{name: "initial-client-empty", message: "certificate"},
				{name: "initial-client-nonempty", message: "certificate", context: []byte{0xa5}, refuse: true},
				{name: "application-client-context", message: "certificate", context: []byte{0xa5}, application: true},
				{name: "recovered-client-context", message: "certificate", context: []byte{0xa5}, application: true, recoverEpoch: true},
				{name: "initial-server-request-empty", message: "request"},
				{name: "initial-server-request-nonempty", message: "request", context: []byte{0xa5}, refuse: true},
				{name: "application-server-request-context", message: "request", context: []byte{0xa5}, application: true},
				{name: "recovered-server-request-context", message: "request", context: []byte{0xa5}, application: true, recoverEpoch: true},
			} {
				t.Run(fmt.Sprintf("suite%04x/client%d/%s", suite, client, c.name), func(t *testing.T) {
					flow, state := boundaryTLSFlow(t, suite)
					state.client = client
					dir := client
					encoded := reliabilityHS(11, certificateExtensionsBody(c.context))
					bodyKey := "Certificate Body"
					if c.message == "request" {
						dir = 1 - client
						encoded = certificateRequest13(c.context, certificateRequestExtension(13, []byte{0, 2, 8, 4}))
						bodyKey = "Certificate Request"
						_, err := DecodeTLSCertificateRequest(encoded, 0x304, 4096)
						require.NoError(t, err, "role-neutral syntax remains inspectable")
					} else {
						_, err := DecodeTLSCertificate(encoded, 0x304, 4096)
						require.NoError(t, err, "role-neutral syntax remains inspectable")
					}
					state.app[dir] = c.application && !c.recoverEpoch
					seal := state
					var recoveredSecret []byte
					if c.recoverEpoch {
						// Model the missing Finished without guessing its bytes.
						// Only authentication with the authorized application secret
						// may establish the new epoch before checking the context.
						trial := *state
						trial.app[dir] = true
						recoveredSecret = bytes.Repeat([]byte{0x7a}, len(state.secret[dir]))
						require.NoError(t, trial.install13(dir, recoveredSecret))
						label := "CLIENT_TRAFFIC_SECRET_0"
						if dir != client {
							label = "SERVER_TRAFFIC_SECRET_0"
						}
						provider, err := ParseTLSKeyLog(label + " " + hex.EncodeToString(state.random) + " " + hex.EncodeToString(recoveredSecret))
						require.NoError(t, err)
						flow.a.tlsSecrets = provider
						seal = &trial
					}
					event := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, seal, dir, 2, encoded, 22, 0)}
					_, err := flow.consumeTLS(dir, event)
					if c.refuse {
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, typed.Error(), "initial")
						require.NotContains(t, event.Session, "Handshake Messages")
					} else {
						require.NoError(t, err)
						messages := event.Session["Handshake Messages"].([]map[string]any)
						require.Len(t, messages, 1)
						body := messages[0][bodyKey].(map[string]any)
						require.Equal(t, byte(len(c.context)), body["Context Length"])
						require.Contains(t, body, "Context", "an empty context differs from an absent field")
						require.Equal(t, string(c.context), string(body["Context"].([]byte)))
					}
					require.Equal(t, client, state.client)
					require.Equal(t, c.application, state.app[dir])
					require.Equal(t, uint64(3), state.seq[dir])
					if c.recoverEpoch {
						require.Equal(t, true, event.Session["Application Epoch Inferred From Authenticated Record"])
						require.Equal(t, true, event.Session["Record Recovery Verified"])
						require.Equal(t, uint64(2), event.Session["Recovered Record Sequence"])
						require.Equal(t, recoveredSecret, state.secret[dir])
					}
				})
			}
		}
	}
}
