package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSResumptionQUICApplicationTransitionPreservesPrivateProjection(t *testing.T) {
	// These literal observations describe a completed ServerHello selecting
	// identity zero. Subsequent application packets retain the observation but
	// must not pretend to contain a new wire handshake.
	selection := map[string]any{
		"Selected Identity": 0, "Offer Match": "unavailable-client-hello",
		"Binder Verified": false, "Ticket Association": "unconfirmed; selected PSK may be external",
		"Evidence Scope": "current ServerHello",
	}
	context := map[string]any{
		"Evidence Scope": "current connection hello observations", "Certificate Trust": "not-evaluated",
		"PSK Selected": true, "PSK Selection": selection,
	}
	for _, protocol := range []string{"quic", "http3", "doq"} {
		for _, handshake := range []bool{false, true} {
			t.Run(protocol+"/handshake="+map[bool]string{false: "false", true: "true"}[handshake], func(t *testing.T) {
				private := map[string]any{"Session Establishment": cloneSession(context)}
				if handshake {
					private["TLS Handshakes"] = []map[string]any{{
						"Type": 2, "PSK Selection": cloneSession(selection),
						"Session ID": map[string]any{"Length": 0, "SHA256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
					}}
				}
				e := &ProtocolEvent{Protocol: protocol, nativeQUICObservation: true, semanticSession: private,
					Session: map[string]any{"Session Establishment": map[string]any{"PSK Selected": false}}}
				want := e.DisplayFields()
				require.Equal(t, []any{true}, want["tls.session.psk.selected"])
				require.Equal(t, []any{false}, want["tls.session.psk.binder_verified"])
				require.Equal(t, []any{uint64(0)}, want["tls.session.psk.selected_identity"])
				if handshake {
					require.Equal(t, []any{uint64(0)}, want["tls.handshake.extensions.psk.identity.selected"])
					require.Equal(t, []any{uint64(0)}, want["tls.handshake.session_id_length"])
				} else {
					require.NotContains(t, want, "tls.handshake.extensions.psk.identity.selected")
					require.NotContains(t, want, "tls.handshake.session_id_length")
				}
				for _, occurrence := range e.DisplayFieldValues().Values {
					if occurrence.Name == "tls.session.psk.selected_identity" {
						require.Equal(t, "context", occurrence.Origin)
						require.Nil(t, occurrence.Range)
					}
				}
				e.Session = map[string]any{"Session Establishment": map[string]any{"PSK Selected": "forged"}}
				require.Equal(t, want, e.DisplayFields(), "public session edits cannot replace admitted QUIC observations")
			})
		}
	}
	for _, protocol := range []string{"http3", "doq", "http"} {
		e := &ProtocolEvent{Protocol: protocol, Session: map[string]any{"Session Establishment": context,
			"TLS Handshakes": []map[string]any{{"Type": 2, "PSK Selection": selection}}}}
		require.NotContains(t, e.DisplayFields(), "tls.session.psk.selected", "application name alone cannot create a QUIC/TLS observation")
		require.NotContains(t, e.DisplayFields(), "tls.handshake.extensions.psk.identity.selected")
	}
}
