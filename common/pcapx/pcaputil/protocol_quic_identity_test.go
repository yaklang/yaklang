package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQUICApplicationIdentityRequiresPrivateWireAuthenticationAndALPN(t *testing.T) {
	proof := map[string]any{"Authentication Verified": true, "Native Carrier": true, "HTTP3": true, "ALPN": "h3"}
	e := &ProtocolEvent{Protocol: "http3", Status: "malformed", Session: cloneSession(proof)}
	require.Empty(t, e.AuthenticatedQUICApplicationProtocol(), "public flags do not grant native identity")
	e.nativeQUICObservation, e.semanticSession = true, cloneSession(proof)
	require.Equal(t, "http3", e.AuthenticatedQUICApplicationProtocol())
	e.Protocol, e.Profile, e.Session = "ssh", "other", map[string]any{"Authentication Verified": false}
	require.Equal(t, "http3", e.AuthenticatedQUICApplicationProtocol(), "public edits cannot rewrite observed identity")
	for _, key := range []string{"Authentication Verified", "Native Carrier", "HTTP3", "ALPN"} {
		e.semanticSession = cloneSession(proof)
		delete(e.semanticSession, key)
		require.Empty(t, e.AuthenticatedQUICApplicationProtocol(), key)
	}
	e.semanticSession = cloneSession(proof)
	e.semanticSession["Authentication Verified"] = false
	require.Empty(t, e.AuthenticatedQUICApplicationProtocol())
	var missing *ProtocolEvent
	require.Empty(t, missing.AuthenticatedQUICApplicationProtocol())
}
