package pcaputil

import (
	"bytes"
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
			// Preserve the immutable legacy bytes and answer. RFC5246 6.2.1
			// permits this three-octet fragment (one Alert plus one pending
			// octet); the old filename is not a normative malformed oracle.
			require.Equal(t, []byte{21, 3, 3, 0, 3, 1, 0, 0}, wire)
			neighbor, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			neighbor.(*captureSession).f.a.config.Deferred = deferred
			n := neighbor.Feed(0, time.Unix(1, 0), bytes.Clone(wire))
			require.Nil(t, n.Err)
			require.Len(t, n.Events, 1)
			want := map[string]any{"Record Type": byte(21), "Record Version": uint16(0x303), "Record Length": 3, "Authentication Verified": false, "Certificate Trust": "not-evaluated", "Content Visibility": "cleartext", "Negotiated Version": uint16(0), "Cipher Suite": uint16(0), "SNI": "", "ALPN": "", "Alert Messages": []map[string]any{{"Level": byte(1), "Description": byte(0), "Raw": []byte{1, 0}}}, "Alert Buffered Bytes": 1}
			require.Equal(t, want, n.Events[0].Session)
			tlsAlertCheckQuery(t, n.Events[0], want, "")
			pending := neighbor.Close("partial")
			require.Len(t, pending, 1)
			require.Equal(t, map[string]any{"Alert Buffered Bytes": 1}, pending[0].Session)
			tlsDTLSQuery(t, pending[0], nil, "NeedMore")
			require.Empty(t, neighbor.Close("again"))
			require.Zero(t, neighbor.Stats().BufferedBytes)
			// RFC5246 6.2.1 forbids zero-length Alert fragments. Keep every
			// original private error/ownership assertion on an actual refusal.
			wire = bytes.Clone(wire[:5])
			wire[4] = 0
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

// RESP2 array/bulk and simple-string bytes have fixed wire meanings. Role,
// association and latency below are the parser's passive-observation contract,
// not proof that a server executed a command. No endpoint is contacted here.
func redisOwnedQueries(t *testing.T) {
	answerRaw, err := trafficfixture.ReadFile("testdata/native-owned-queries/answers.json")
	require.NoError(t, err)
	var answers struct {
		Cases map[string]struct {
			Fields    map[string]any `json:"fields"`
			Error     string         `json:"second_error"`
			LateError string         `json:"late_error"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(answerRaw, &answers))
	require.Len(t, answers.Cases, 3)
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis-query-deferred-%v", deferred), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("done")
			s.(*captureSession).f.a.config.Deferred = deferred
			ts := time.Unix(1, 0)
			wire, err := trafficfixture.ReadFile("testdata/native-owned-queries/records/ping.bin")
			require.NoError(t, err)
			r := s.Feed(0, ts, wire)
			clear(wire)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			q := r.Events[0]
			wantRequest := map[string]any{"RESP Type": "Array", "Version": "RESP2", "Version Negotiated": false, "Context Level": "observed", "Direction Value Index": uint64(1), "Value": map[string]any{"Type": "Array", "Items": []any{map[string]any{"Type": "Bulk", "Value": []byte("PING")}}}, "Role Evidence": "command-shape", "Command": "PING", "Outstanding": true}
			rocEqualFields(t, answers.Cases["request"].Fields, wantRequest)
			require.Equal(t, wantRequest, q.Session)
			require.Equal(t, q.ID, q.TransactionID)
			require.Zero(t, q.ResponseTo)
			check := func(e *ProtocolEvent, want map[string]any) {
				t.Helper()
				id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
				query := func() {
					tree, err := e.Decode()
					require.NoError(t, err)
					require.Equal(t, want, protocolFields(tree))
					require.Equal(t, want, tree["session"])
					mutateRTPSQuery(protocolFields(tree))
					mutateRTPSQuery(tree["session"])
					fields, err := e.GetFields()
					require.NoError(t, err)
					require.Equal(t, want, fields)
					mutateRTPSQuery(fields)
					require.Equal(t, id, e.ID)
					require.Equal(t, flow, e.FlowID)
					require.Equal(t, response, e.ResponseTo)
					require.Equal(t, transaction, e.TransactionID)
				}
				history, err := NewBinParserInspector(1, e.RetainedBytes())
				require.NoError(t, err)
				history.OnEvent(e)
				query()
				mutateRTPSQuery(e.Session)
				mutateRTPSQuery(e.Fields)
				mutateRTPSQuery(e.Structured)
				query()
				e.Session = map[string]any{"Matched Request": true, "Command": "caller-forged"}
				e.Fields = map[string]any{"caller-forged": true}
				e.Structured = map[string]any{"fields": e.Fields}
				query()
				e.Session = nil
				query()
				for i := 0; i < 2; i++ {
					detail, err := history.Details(id)
					require.NoError(t, err)
					tree, err := detail.Decode()
					require.NoError(t, err)
					require.Equal(t, want, protocolFields(tree))
					require.Equal(t, want, tree["session"])
					mutateRTPSQuery(detail.Session)
					mutateRTPSQuery(detail.Fields)
				}
			}
			check(q, wantRequest)
			wire, err = trafficfixture.ReadFile("testdata/native-owned-queries/records/pong.bin")
			require.NoError(t, err)
			r = s.Feed(1, ts.Add(time.Millisecond), wire)
			clear(wire)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			wantResponse := map[string]any{"RESP Type": "Simple", "Version": "RESP2", "Version Negotiated": false, "Context Level": "observed", "Direction Value Index": uint64(1), "Value": map[string]any{"Type": "Simple", "Value": "PONG"}, "Matched Request": true, "Command": "PING", "Latency NS": int64(time.Millisecond)}
			rocEqualFields(t, answers.Cases["response"].Fields, wantResponse)
			require.Equal(t, wantResponse, e.Session)
			require.Equal(t, q.ID, e.ResponseTo)
			require.Equal(t, q.ID, e.TransactionID)
			require.Equal(t, q.FlowID, e.FlowID)
			check(e, wantResponse)
			s.Close("done")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
		t.Run(fmt.Sprintf("redis-resource-query-deferred-%v", deferred), func(t *testing.T) {
			budget := DefaultParserBudget()
			budget.MaxCollectionElements = 1
			s, err := NewProtocolSession(budget)
			require.NoError(t, err)
			defer s.Close("done")
			s.(*captureSession).f.a.config.Deferred = deferred
			wire := []byte("*1\r\n$4\r\nPING\r\n")
			r := s.Feed(0, time.Unix(1, 0), wire)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			q := r.Events[0]
			require.NotZero(t, q.ID)
			r = s.Feed(0, time.Unix(2, 0), wire)
			require.NotNil(t, r.Err)
			require.Equal(t, ErrResourceExceeded, r.Err.Kind)
			require.Equal(t, string(r.Err.Kind), answers.Cases["pending-refusal"].Error)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, q.FlowID, e.FlowID)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
			id, flow := e.ID, e.FlowID
			e.Status, e.Error = "decoded", ""
			e.Session = map[string]any{"Matched Request": true, "Command": "PING"}
			e.Fields, e.Structured = e.Session, map[string]any{"fields": e.Session}
			for i := 0; i < 2; i++ {
				tree, err := e.Decode()
				require.Nil(t, tree)
				var pe *ProtocolError
				require.ErrorAs(t, err, &pe)
				require.Equal(t, ErrResourceExceeded, pe.Kind)
				require.Equal(t, "Redis pending requests", pe.Message)
				pe.Kind, pe.Message = ErrMalformedMessage, "caller-forged"
				fields, err := e.GetFields()
				require.Nil(t, fields)
				require.ErrorAs(t, err, &pe)
				require.Equal(t, ErrResourceExceeded, pe.Kind)
				require.Equal(t, "Redis pending requests", pe.Message)
				require.Equal(t, id, e.ID)
				require.Equal(t, flow, e.FlowID)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
			}
			// Resource refusal stops this standalone session. Late input must
			// not emit a reply or recover either request's association state.
			r = s.Feed(1, time.Unix(3, 0), []byte("+PONG\r\n"))
			require.NotNil(t, r.Err)
			require.Equal(t, ErrFatalSessionError, r.Err.Kind)
			require.Equal(t, string(r.Err.Kind), answers.Cases["pending-refusal"].LateError)
			require.Empty(t, r.Events)
			s.Close("done")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	manual := &ProtocolEvent{Protocol: "redis", Fields: map[string]any{"manual": true}}
	fields, err := manual.GetFields()
	require.NoError(t, err)
	require.Equal(t, manual.Fields, fields)
}

func TestRedisOwnedQueries(t *testing.T) { redisOwnedQueries(t) }

// Complete snapshot stability is an ownership oracle, not an independent
// grammar oracle. The caller verifies native-client result answers first and
// preserves the original capture's full handshake and statement context.
func testPreparedQueryOwnership(t *testing.T, e *ProtocolEvent) {
	t.Helper()
	require.Equal(t, "MySQLPreparedFields", e.Entry)
	want := cloneSession(e.Session)
	savedFields, savedStructured := cloneSession(e.Fields), cloneSession(e.Structured)
	defer func() { e.Session, e.Fields, e.Structured = want, savedFields, savedStructured }()
	id, flow, response, transaction := e.ID, e.FlowID, e.ResponseTo, e.TransactionID
	query := func() {
		tree, err := e.Decode()
		require.NoError(t, err)
		require.Equal(t, want, protocolFields(tree))
		require.Equal(t, want, tree["session"])
		mutateRTPSQuery(protocolFields(tree))
		mutateRTPSQuery(tree["session"])
		fields, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, want, fields)
		mutateRTPSQuery(fields)
		require.Equal(t, id, e.ID)
		require.Equal(t, flow, e.FlowID)
		require.Equal(t, response, e.ResponseTo)
		require.Equal(t, transaction, e.TransactionID)
	}
	query()
	mutateRTPSQuery(e.Session)
	mutateRTPSQuery(e.Fields)
	mutateRTPSQuery(e.Structured)
	query()
	e.Session = map[string]any{"Statement ID": uint32(999999), "Session State Validated": true}
	e.Fields, e.Structured = e.Session, map[string]any{"fields": e.Session}
	query()
	e.Session = nil
	query()
}
