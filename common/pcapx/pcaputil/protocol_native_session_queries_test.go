package pcaputil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The existing sealed NMEA/Semtech/RTPS matrices supply complete independent
// answers. Session here is a stateless observation of that exact message;
// replacing its public projection must not rewrite subsequent queries.
func testNativeObservationSessionQueries(t testing.TB, e *ProtocolEvent, want map[string]any) {
	t.Helper()
	if e.Transport != "udp" || e.Profile != e.Protocol+"-native" {
		return // Stateful downlink context has a different, separate oracle.
	}
	saved := cloneSession(e.Session)
	defer func() { e.Session = saved }()
	id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
	wantJSON, err := json.Marshal(want)
	require.NoError(t, err)
	equal := func(value any) {
		got, err := json.Marshal(value)
		require.NoError(t, err)
		require.JSONEq(t, string(wantJSON), string(got), "complete independently answered fields and session")
	}
	query := func() {
		tree, err := e.Decode()
		require.NoError(t, err)
		equal(protocolFields(tree))
		session, ok := tree["session"].(map[string]any)
		require.True(t, ok, "complete native observation session must be retained")
		equal(session)
		mutateRTPSQuery(session)
		mutateRTPSQuery(protocolFields(tree))
		fields, err := e.GetFields()
		require.NoError(t, err)
		equal(fields)
		require.Equal(t, id, e.ID)
		require.Equal(t, flow, e.FlowID)
		require.Equal(t, response, e.ResponseTo)
		require.Equal(t, transaction, e.TransactionID)
	}
	query()
	mutateRTPSQuery(e.Session)
	query()
	e.Session = map[string]any{"caller": "replacement"}
	query()
	e.Session = nil
	query()
}

// The callers supply independently sealed whole-message answers. Their native
// session contains that same accepted observation, including association fields.
func testExactObservationSessionQueries(t *testing.T, e *ProtocolEvent, want map[string]any) {
	t.Helper()
	defer func() { e.Session = cloneSession(want) }()
	id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
	query := func() {
		tree, err := e.Decode()
		require.NoError(t, err)
		rocEqualFields(t, want, protocolFields(tree))
		session, ok := tree["session"].(map[string]any)
		require.True(t, ok, "accepted private session must remain available")
		rocEqualFields(t, want, session)
		mutateRTPSQuery(session)
		mutateRTPSQuery(protocolFields(tree))
		fields, err := e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want, fields)
		require.Equal(t, id, e.ID)
		require.Equal(t, flow, e.FlowID)
		require.Equal(t, response, e.ResponseTo)
		require.Equal(t, transaction, e.TransactionID)
	}
	query()
	mutateRTPSQuery(e.Session)
	query()
	e.Session = map[string]any{"caller": "replacement"}
	query()
	e.Session = nil
	query()
}

// Rule fields and observed session context need not have the same schema.
// The caller supplies the independently answered complete session projection.
func testOwnedSessionQueries(t *testing.T, e *ProtocolEvent, want map[string]any) {
	t.Helper()
	saved := e.Session
	defer func() { e.Session = saved }()
	id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
	for _, public := range []map[string]any{e.Session, {"Packet Name": "forged", "Association Status": "verified"}, nil} {
		e.Session = public
		for repeat := 0; repeat < 2; repeat++ {
			result, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, want, result["session"])
			result["session"].(map[string]any)["Association Status"] = "caller-forged"
			require.Equal(t, id, e.ID)
			require.Equal(t, flow, e.FlowID)
			require.Equal(t, response, e.ResponseTo)
			require.Equal(t, transaction, e.TransactionID)
		}
	}
}
