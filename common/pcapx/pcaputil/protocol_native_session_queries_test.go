package pcaputil

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
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

// RFC5246 7.2: Alert is two one-byte fields. These controls observe one
// isolated cleartext record; no negotiated or authenticated connection is inferred.
func tlsAlertOwnedQueries(t *testing.T) {
	answerRaw, err := trafficfixture.ReadFile("testdata/tls-owned-queries/answers.json")
	require.NoError(t, err)
	var answers struct {
		Cases map[string]struct {
			Fields map[string]any `json:"fields"`
			Error  string         `json:"error"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(answerRaw, &answers))
	require.Len(t, answers.Cases, 2)
	manual := &ProtocolEvent{Protocol: "tls", Fields: map[string]any{"manual": true}}
	fields, err := manual.GetFields()
	require.NoError(t, err)
	require.Equal(t, manual.Fields, fields)
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("deferred-%v", deferred), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("done")
			s.(*captureSession).f.a.config.Deferred = deferred
			wire, err := trafficfixture.ReadFile("testdata/tls-owned-queries/records/alert.bin")
			require.NoError(t, err)
			r := s.Feed(0, time.Unix(1, 0), wire)
			clear(wire)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, []byte{21, 3, 3, 0, 2, 1, 0}, e.Raw)
			id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
			want := map[string]any{"Record Type": byte(21), "Record Version": uint16(0x303), "Record Length": 2, "Authentication Verified": false, "Certificate Trust": "not-evaluated", "Content Visibility": "cleartext", "Alert Level": byte(1), "Alert Description": byte(0), "Negotiated Version": uint16(0), "Cipher Suite": uint16(0), "SNI": "", "ALPN": ""}
			rocEqualFields(t, answers.Cases["alert"].Fields, want)
			require.Equal(t, want, e.Session)
			query := func() {
				result, err := e.Decode()
				require.NoError(t, err)
				require.Equal(t, want, protocolFields(result))
				require.Equal(t, want, result["session"])
				mutateRTPSQuery(protocolFields(result))
				mutateRTPSQuery(result["session"])
				fields, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, want, fields)
				require.Equal(t, id, e.ID)
				require.Equal(t, flow, e.FlowID)
				require.Equal(t, response, e.ResponseTo)
				require.Equal(t, transaction, e.TransactionID)
			}
			query()
			history, err := NewBinParserInspector(1, e.RetainedBytes())
			require.NoError(t, err)
			history.OnEvent(e)
			mutateRTPSQuery(e.Session)
			query()
			e.Session = map[string]any{"Authentication Verified": true}
			query()
			e.Session = nil
			query()
			for i := 0; i < 2; i++ {
				detail, err := history.Details(id)
				require.NoError(t, err)
				result, err := detail.Decode()
				require.NoError(t, err)
				require.Equal(t, want, protocolFields(result))
				require.Equal(t, want, result["session"])
				mutateRTPSQuery(detail.Session)
				mutateRTPSQuery(detail.Fields)
			}

			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("malformed-alert-query-deferred-%v", deferred), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("done")
			s.(*captureSession).f.a.config.Deferred = deferred
			wire, err := trafficfixture.ReadFile("testdata/tls-owned-queries/records/malformed.bin")
			require.NoError(t, err)
			r := s.Feed(0, time.Unix(1, 0), wire)
			require.NotNil(t, r.Err)
			require.Equal(t, ErrMalformedMessage, r.Err.Kind)
			require.Equal(t, string(ErrMalformedMessage), answers.Cases["malformed"].Error)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			result, err := e.Decode()
			require.Nil(t, result)
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
			message := pe.Message
			id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
			e.Status, e.Error = "decoded", ""
			e.Session = map[string]any{"Authentication Verified": true}
			e.Fields, e.Structured = map[string]any{"caller-forged": true}, map[string]any{"fields": map[string]any{"caller-forged": true}}
			for i := 0; i < 2; i++ {
				pe.Kind, pe.Message = ErrEncrypted, "caller-forged"
				fields, err := e.GetFields()
				require.Nil(t, fields)
				require.ErrorAs(t, err, &pe)
				require.Equal(t, ErrMalformedMessage, pe.Kind)
				require.Equal(t, message, pe.Message)
				require.Equal(t, id, e.ID)
				require.Equal(t, flow, e.FlowID)
				require.Equal(t, response, e.ResponseTo)
				require.Equal(t, transaction, e.TransactionID)
			}
			s.Close("done")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestTLSAlertOwnedQueries(t *testing.T) { tlsAlertOwnedQueries(t) }
