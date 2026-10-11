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

// Query output immutability is a separate contract from the independent wire
// oracle exercised by the callers. Preserve every original field/metadata value.
func testLegacyRuleQueryOwnership(t *testing.T, e *ProtocolEvent) {
	t.Helper()
	first, err := e.Decode()
	require.NoError(t, err)
	want := cloneSession(first)
	id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
	for _, where := range []string{"returned-envelope", "public-fields", "public-envelope", "returned-fields"} {
		t.Run(where, func(t *testing.T) {
			result, err := e.Decode()
			require.NoError(t, err)
			switch where {
			case "returned-envelope":
				mutateRTPSQuery(protocolFields(result))
				mutateRTPSQuery(result["metadata"])
			case "public-fields":
				mutateRTPSQuery(e.Fields)
				mutateRTPSQuery(e.Metadata)
			case "public-envelope":
				e.Structured = map[string]any{"fields": map[string]any{"caller-forged": true}, "metadata": map[string]any{"caller-forged": true}}
			case "returned-fields":
				fields, err := e.GetFields()
				require.NoError(t, err)
				mutateRTPSQuery(fields)
			}
			again, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, want, again, "queries cannot rewrite any accepted rule field, metadata or session")
			fields, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, protocolFields(want), fields)
			require.Equal(t, id, e.ID)
			require.Equal(t, flow, e.FlowID)
			require.Equal(t, response, e.ResponseTo)
			require.Equal(t, transaction, e.TransactionID)
		})
	}
	for _, limit := range []int{e.RetainedBytes() - 1, e.RetainedBytes()} {
		history, err := NewBinParserInspector(1, limit)
		require.NoError(t, err)
		history.OnEvent(e)
		if limit < e.RetainedBytes() {
			require.Empty(t, history.Rows("", 0))
			require.EqualValues(t, 1, history.Evicted())
			continue
		}
		rows := history.Rows("", 0)
		require.Len(t, rows, 1)
		require.Nil(t, rows[0].semanticMetadata)
		require.False(t, rows[0].ownedRuleEnvelope)
		for i := 0; i < 2; i++ {
			detail, err := history.Details(e.ID)
			require.NoError(t, err)
			result, err := detail.Decode()
			require.NoError(t, err)
			require.Equal(t, want, result)
			mutateRTPSQuery(protocolFields(result))
			mutateRTPSQuery(result["metadata"])
			mutateRTPSQuery(detail.Fields)
			mutateRTPSQuery(detail.Metadata)
		}
		require.Zero(t, history.Evicted())
	}
}

// The new retained envelope primitive also owns nonnil rule metadata. This is
// an ownership/accounting unit control, not a claim about a captured wire schema.
func testLegacyRuleMetadataOwnership(t *testing.T) {
	t.Helper()
	fields := map[string]any{"n": uint16(1)}
	metadata := map[string]any{"tag": []byte{1, 2, 3}}
	e := &ProtocolEvent{ID: 41, Protocol: "goose", Fields: fields, Metadata: metadata, Structured: map[string]any{"fields": fields, "metadata": metadata}}
	e.retainRuleEnvelope(e.Structured)
	// Fields64+1+32+16=113; metadata64+3+32+24+3=126.
	require.Equal(t, 239, e.RetainedBytes())
	want := map[string]any{"fields": map[string]any{"n": uint16(1)}, "metadata": map[string]any{"tag": []byte{1, 2, 3}}}
	mutateRTPSQuery(fields)
	mutateRTPSQuery(metadata)
	for _, limit := range []int{238, 239} {
		history, err := NewBinParserInspector(1, limit)
		require.NoError(t, err)
		history.OnEvent(e)
		if limit == 238 {
			require.Empty(t, history.Rows("", 0))
			require.EqualValues(t, 1, history.Evicted())
			continue
		}
		for i := 0; i < 2; i++ {
			detail, err := history.Details(e.ID)
			require.NoError(t, err)
			result, err := detail.Decode()
			require.NoError(t, err)
			require.Equal(t, want, result)
			mutateRTPSQuery(detail.Metadata)
			mutateRTPSQuery(result["metadata"])
		}
		rows, _, _ := history.RowsAfter(0, 1, "", 0)
		require.Len(t, rows, 1)
		require.Nil(t, rows[0].semanticMetadata)
		require.False(t, rows[0].ownedRuleEnvelope)
	}
	for _, protocol := range []string{"goose", "rtp"} {
		manual := &ProtocolEvent{Protocol: protocol, Fields: map[string]any{"manual": true}, Structured: want}
		actual, err := manual.GetFields()
		require.NoError(t, err)
		require.Equal(t, manual.Fields, actual, "manual field caches preserve compatibility")
		result, err := manual.Decode()
		require.NoError(t, err)
		require.Equal(t, want, result)
	}
}
