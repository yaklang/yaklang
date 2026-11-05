package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSCertificateKnownServerRoleAdmission(t *testing.T) {
	// RFC 8446 section 4.4.2 requires an empty server request context and
	// a nonempty server certificate list. An empty client list is valid.
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, c := range []struct {
				name        string
				client      bool
				unknown     bool
				application bool
				context     []byte
				entry       bool
				refuse      bool
			}{
				{name: "server-context", context: []byte{0xa5}, entry: true, refuse: true},
				{name: "server-empty-list", refuse: true},
				{name: "server-certificate", entry: true},
				{name: "server-application-certificate", application: true, entry: true, refuse: true},
				{name: "client-empty-list", client: true, application: true},
				{name: "client-post-handshake-empty-list", client: true, application: true, context: []byte{0xa5}},
				{name: "unknown-empty-list", unknown: true, application: true},
				{name: "unknown-context", unknown: true, application: true, context: []byte{0xa5}, entry: true},
			} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, c.name), func(t *testing.T) {
					flow, state := boundaryTLSFlow(t, suite)
					state.client = 1 - server
					if c.unknown {
						state.client = -1
					}
					dir := server
					if c.client {
						dir = 1 - server
					}
					state.app[dir] = c.application
					epochs := state.app
					var entries [][]byte
					if c.entry {
						entries = [][]byte{nil}
					}
					encoded := reliabilityHS(11, certificateExtensionsBody(c.context, entries...))
					// The public grammar API has no endpoint role and must keep
					// both structurally valid vectors available for inspection.
					view, err := DecodeTLSCertificate(encoded, 0x304, 4096)
					require.NoError(t, err)
					secret, iv, key := bytes.Clone(state.secret[dir]), bytes.Clone(state.iv[dir]), state.keys[dir]
					wire := boundaryTLSRecord(t, state, dir, 2, encoded, 22, 0)
					event := &ProtocolEvent{Protocol: "tls", Raw: wire}
					_, err = flow.consumeTLS(dir, event)
					if c.unknown {
						require.NoError(t, err)
						require.Equal(t, "missing-context", event.Session["Secret Lookup Status"])
						require.NotContains(t, event.Session, "Handshake Messages")
						require.Equal(t, uint64(2), state.seq[dir], "missing role cannot admit authenticated plaintext")
					} else if c.refuse {
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, typed.Error(), "server TLS Certificate")
						require.NotContains(t, event.Session, "Handshake Messages")
						require.NotContains(t, event.DisplayFieldValues().FieldMap(), "tls.handshake.certificate")
					} else {
						require.NoError(t, err)
						messages := event.Session["Handshake Messages"].([]map[string]any)
						require.Len(t, messages, 1)
						require.Equal(t, view.Fields, messages[0]["Certificate Body"])
						require.Equal(t, encoded, messages[0]["Encoded Certificate"])
					}
					require.Equal(t, secret, state.secret[dir])
					require.Equal(t, iv, state.iv[dir])
					require.Same(t, key, state.keys[dir])
					if !c.unknown {
						require.Equal(t, uint64(3), state.seq[dir], "the authenticated record consumed its own sequence")
					}
					require.Equal(t, epochs, state.app, "a body role check cannot change either key epoch")
				})
			}
		}
	}
}
