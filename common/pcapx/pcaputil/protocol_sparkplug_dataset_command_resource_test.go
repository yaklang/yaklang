package pcaputil

// Unexecuted, repository-external regression. No production API is added or required.
// Semantic cases delegate to the already accepted strict DataSet trace helper.
// Resource cases deliberately use whole public Feed calls in full/deferred mode.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type spCommandResourceDraftStep struct {
	Name                                             string `json:"name"`
	Input                                            string `json:"input_hex"`
	SHA                                              string `json:"input_sha256"`
	Direction                                        int    `json:"direction"`
	Time                                             int64  `json:"timestamp_unix"`
	Offset                                           uint64 `json:"offset"`
	Length                                           int    `json:"length"`
	Source, Destination, Protocol, Transport, Status string
	Session                                          map[string]any
	PublicError                                      string         `json:"public_error"`
	QueryError                                       *ProtocolError `json:"all_query_errors"`
	Feed                                             struct {
		Consumed int
		NeedMore bool
		Events   *int
		Err      *ProtocolError
	} `json:"FeedResult"`
	StatsAfterFeed *struct{ BufferedBytes int64 } `json:"Stats_after_Feed"`
}

type spCommandResourceDraftCase struct {
	Name   string                                                 `json:"name"`
	Steps  json.RawMessage                                        `json:"steps"`
	Budget struct{ MaxRecursionDepth, MaxCollectionElements int } `json:"budget"`
}

func TestSparkplugDataSetCommandInvalidCellsAndDepthExistingAPI(t *testing.T) {
	raw := spDataSetCommandSource(t, "invalid-cell-controls.json")
	require.Equal(t, "a26a2173355d52ab22f69224ae2d8bace63d532cfced760dc6405367345328f3", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct{ Cases []spCommandResourceDraftCase }
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&g))
	require.Len(t, g.Cases, 4)

	// The two depth controls MUST carry identical bytes, not different examples.
	var low, high []spCommandResourceDraftStep
	spCommandResourceDraftDecode(t, g.Cases[2].Steps, &low)
	spCommandResourceDraftDecode(t, g.Cases[3].Steps, &high)
	require.Len(t, low, 6)
	require.Len(t, high, 6)
	for i := range low {
		require.Equal(t, low[i].Input, high[i].Input)
		require.Equal(t, low[i].SHA, high[i].SHA)
		require.Equal(t, low[i].Direction, high[i].Direction)
		require.Equal(t, low[i].Time, high[i].Time)
	}
	require.Equal(t, 4, g.Cases[2].Budget.MaxRecursionDepth)
	require.Equal(t, 5, g.Cases[3].Budget.MaxRecursionDepth)

	for index, c := range g.Cases {
		index, c := index, c
		t.Run(c.Name, func(t *testing.T) {
			if index < 2 {
				// Existing checker already verifies complete semantic Session,
				// type/metadata/input/query/history ownership and empty Close.
				var steps []spDataSetStep
				spCommandResourceDraftDecode(t, c.Steps, &steps)
				spDataSetStateTrace(t, steps)
				return
			}
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("deferred%v", deferred), func(t *testing.T) {
					var steps []spCommandResourceDraftStep
					spCommandResourceDraftDecode(t, c.Steps, &steps)
					spCommandResourceDraftTrace(t, c, steps, deferred)
				})
			}
		})
	}
}

func spCommandResourceDraftDecode(t *testing.T, raw []byte, dst any) {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(dst))
}

type spCommandResourceDraftHistory struct {
	event   *ProtocolEvent
	session map[string]any
	fields  map[string]any
	err     *ProtocolError
}

func spCommandResourceDraftError(t *testing.T, got error, want *ProtocolError) *ProtocolError {
	t.Helper()
	require.NotNil(t, want)
	var pe *ProtocolError
	require.ErrorAs(t, got, &pe)
	require.Equal(t, want.Kind, pe.Kind)
	require.Equal(t, want.Message, pe.Message)
	require.Equal(t, want.Error(), pe.Error())
	return pe
}

func spCommandResourceDraftHistoryCheck(t *testing.T, h spCommandResourceDraftHistory) {
	t.Helper()
	if h.err != nil {
		fields, err := h.event.GetFields()
		require.Nil(t, fields)
		pe := spCommandResourceDraftError(t, err, h.err)
		pe.Kind, pe.Message = ErrMalformedMessage, "poison returned GetFields error"
		tree, err := h.event.Decode()
		require.Nil(t, tree)
		pe = spCommandResourceDraftError(t, err, h.err)
		pe.Kind, pe.Message = ErrMalformedMessage, "poison returned Decode error"
		// Repeat each query after poisoning both returned error copies.
		fields, err = h.event.GetFields()
		require.Nil(t, fields)
		spCommandResourceDraftError(t, err, h.err)
		tree, err = h.event.Decode()
		require.Nil(t, tree)
		spCommandResourceDraftError(t, err, h.err)
		return
	}
	tree, err := h.event.Decode()
	require.NoError(t, err)
	spAssertTree(t, h.session, tree["session"])
	session, ok := tree["session"].(map[string]any)
	require.True(t, ok)
	spDataSetTypes(t, session)
	fields, err := h.event.GetFields()
	require.NoError(t, err)
	require.Equal(t, h.fields, fields)
	if spark, present := h.session["Sparkplug"]; present {
		spAssertTree(t, spark, fields["Sparkplug"])
		spDataSetTypes(t, map[string]any{"Sparkplug": fields["Sparkplug"]})
	} else {
		require.NotContains(t, fields, "Sparkplug")
	}
	spDataSetPoison(tree)
	spDataSetPoison(fields)
}

func spCommandResourceDraftTrace(t *testing.T, c spCommandResourceDraftCase, steps []spCommandResourceDraftStep, deferred bool) {
	t.Helper()
	b := DefaultParserBudget()
	b.MaxRecursionDepth = c.Budget.MaxRecursionDepth
	b.MaxCollectionElements = c.Budget.MaxCollectionElements
	require.Equal(t, 4096, b.MaxCollectionElements)
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(40000, 1883))
	require.NoError(t, err)
	defer s.Close("cleanup")
	// Test-only existing configuration hook, exactly as spDataSetStateTrace.
	// All actual input and lifecycle operations remain public Feed/Close/Stats.
	s.(*captureSession).f.a.config.Deferred = deferred
	var history []spCommandResourceDraftHistory
	recheck := func() {
		for _, h := range history {
			spCommandResourceDraftHistoryCheck(t, h)
		}
	}
	ids := map[uint64]bool{}
	var flow, inputBytes, messageBytes, limitedBytes, successEvents uint64
	for _, w := range steps {
		wire, err := hex.DecodeString(w.Input)
		require.NoError(t, err)
		require.Equal(t, w.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
		input := append([]byte(nil), wire...)
		r := s.Feed(w.Direction, time.Unix(w.Time, 0), input)
		require.Equal(t, w.Feed.Consumed, r.Consumed)
		require.Equal(t, len(input), r.Consumed)
		require.Equal(t, w.Feed.NeedMore, r.NeedMore)
		require.False(t, r.NeedMore)
		if w.Feed.Err == nil {
			require.Nil(t, r.Err) // FeedResult.Err is *ProtocolError, not error.
		} else {
			require.NotNil(t, r.Err)
			require.Equal(t, w.Feed.Err.Kind, r.Err.Kind)
			require.Equal(t, w.Feed.Err.Message, r.Err.Message)
			require.Equal(t, w.Feed.Err.Error(), r.Err.Error())
		}
		wantEvents := 1
		if w.Feed.Events != nil {
			wantEvents = *w.Feed.Events
		}
		require.Len(t, r.Events, wantEvents)
		inputBytes += uint64(len(input))
		for i := range input {
			input[i] ^= 255
		}
		if w.StatsAfterFeed != nil {
			require.Equal(t, w.StatsAfterFeed.BufferedBytes, s.Stats().BufferedBytes)
		}
		if wantEvents == 0 {
			require.Equal(t, ErrFatalSessionError, r.Err.Kind)
			require.Equal(t, "session stopped", r.Err.Message)
			// Public error ownership also applies to a stopped Feed result.
			r.Err.Kind, r.Err.Message = ErrMalformedMessage, "poison stopped Feed error"
			recheck()
			continue
		}
		e := r.Events[0]
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		if flow == 0 {
			flow = e.FlowID
		}
		require.NotZero(t, flow)
		require.Equal(t, flow, e.FlowID)
		require.Equal(t, w.Protocol, e.Protocol)
		require.Equal(t, w.Transport, e.Transport)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, w.Source, e.Source)
		require.Equal(t, w.Destination, e.Destination)
		require.Equal(t, time.Unix(w.Time, 0), e.Timestamp)
		require.Equal(t, w.Offset, e.Offset)
		require.Equal(t, w.Length, e.Length)
		require.Equal(t, wire, e.Raw) // after caller input mutation
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Equal(t, w.PublicError, e.Error)
		require.Equal(t, "mqtt", e.Profile) // existing finalizeEvidence fallback
		require.Equal(t, "reassembled", e.SourceBytes.Kind)
		require.Equal(t, w.SHA, e.SourceBytes.SHA256)
		messageBytes += uint64(len(wire))
		h := spCommandResourceDraftHistory{event: e, session: w.Session}
		if w.QueryError == nil {
			require.Nil(t, w.Feed.Err)
			status := w.Status
			if deferred {
				status = "deferred"
			}
			require.Equal(t, status, e.Status)
			require.Equal(t, "envelope", e.Completeness)
			require.Empty(t, e.ExpertCode)
			require.Equal(t, "MQTT "+w.Session["Packet Name"].(string), e.Summary)
			spAssertTree(t, w.Session, e.Session)
			spDataSetTypes(t, e.Session)
			fields, err := e.GetFields()
			require.NoError(t, err)
			if spark, present := w.Session["Sparkplug"]; present {
				spAssertTree(t, spark, fields["Sparkplug"])
				spDataSetTypes(t, map[string]any{"Sparkplug": fields["Sparkplug"]})
			} else {
				require.NotContains(t, fields, "Sparkplug")
			}
			h.fields = cloneSession(fields)
			spDataSetPoison(fields)
			successEvents++
		} else {
			require.Equal(t, ErrResourceExceeded, w.QueryError.Kind)
			require.Equal(t, "Sparkplug protobuf nesting", w.QueryError.Message)
			require.Equal(t, w.QueryError, w.Feed.Err)
			require.Equal(t, "limited", e.Status)
			require.Equal(t, "limited", e.Completeness)
			require.Equal(t, string(ErrResourceExceeded), e.ExpertCode)
			require.Equal(t, "mqtt", e.Summary) // no successful Session summary
			require.Nil(t, e.Session)
			require.Nil(t, e.Fields)
			copyError := *w.QueryError
			h.err = &copyError
			limitedBytes += uint64(len(wire))
			require.Zero(t, s.Stats().BufferedBytes)
		}
		history = append(history, h)
		recheck() // establish pristine private answer before public poisoning
		if r.Err != nil {
			r.Err.Kind, r.Err.Message = ErrMalformedMessage, "poison Feed error"
		}
		for i := range e.Raw {
			e.Raw[i] ^= 255
		}
		spDataSetPoison(e.Session)
		e.Fields = map[string]any{"poison": "public fields cannot replace private answer"}
		e.Structured = map[string]any{"fields": e.Fields, "session": map[string]any{"poison": true}}
		if h.err != nil {
			e.Status, e.Error = "decoded", ""
			e.Session = map[string]any{"poison": "typed error must remain authoritative"}
		}
		recheck()
	}
	before := s.Stats()
	require.Equal(t, inputBytes, before.InputBytes)
	require.Equal(t, uint64(len(history)), before.Messages)
	require.Equal(t, messageBytes, before.MessageBytes)
	require.Equal(t, limitedBytes, before.LimitedBytes)
	if deferred {
		require.Equal(t, successEvents, before.Deferred)
		require.Zero(t, before.Decoded)
	} else {
		require.Equal(t, successEvents, before.Decoded)
		require.Zero(t, before.Deferred)
	}
	require.Zero(t, before.ContextRequired)
	require.Zero(t, before.Malformed)
	require.Zero(t, before.Incomplete)
	require.Zero(t, before.CallbackPanics)
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Equal(t, before.Incomplete, s.Stats().Incomplete)
	recheck()
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	recheck()
}
