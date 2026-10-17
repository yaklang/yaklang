package pcaputil

// Aggregate wire-field and array-element limits retain strict lifecycle checks.
import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSparkplugArrayAggregateElementBudgetExistingAPI(t *testing.T) {
	raw := spArraySource(t, "aggregate-budget-controls.json")
	require.Equal(t, "fa90f51b98c78ddf3d6bb2c8d10cfb201f69e1ecfd4ae1bd69a8c5daf20e3e16", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct{ Cases []spCommandResourceDraftCase }
	spCommandResourceDraftDecode(t, raw, &gold)
	require.Len(t, gold.Cases, 2)
	require.Equal(t, 33, gold.Cases[0].Budget.MaxCollectionElements)
	require.Equal(t, 34, gold.Cases[1].Budget.MaxCollectionElements)
	var sameLow, sameHigh []spCommandResourceDraftStep
	spCommandResourceDraftDecode(t, gold.Cases[0].Steps, &sameLow)
	spCommandResourceDraftDecode(t, gold.Cases[1].Steps, &sameHigh)
	require.Len(t, sameLow, 6)
	require.Len(t, sameHigh, 6)
	for i := range sameLow {
		require.Equal(t, sameLow[i].Input, sameHigh[i].Input)
		require.Equal(t, sameLow[i].SHA, sameHigh[i].SHA)
		require.Equal(t, sameLow[i].Direction, sameHigh[i].Direction)
		require.Equal(t, sameLow[i].Time, sameHigh[i].Time)
	}
	for _, c := range gold.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			require.Equal(t, 64, c.Budget.MaxRecursionDepth)
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("whole-deferred%v", deferred), func(t *testing.T) {
					var steps []spCommandResourceDraftStep
					spCommandResourceDraftDecode(t, c.Steps, &steps)
					spCommandResourceDraftTrace(t, c, steps, deferred)
				})
			}
			if c.Budget.MaxCollectionElements == 34 {
				// Existing strict helper runs whole/1/3/17 and full/deferred;
				// full fields/session/order/IDs/raw/private ownership stay on.
				b := DefaultParserBudget()
				b.MaxRecursionDepth = 64
				b.MaxCollectionElements = 34
				var steps []spDataSetStep
				spCommandResourceDraftDecode(t, c.Steps, &steps)
				spDataSetStateTraceBudget(t, steps, b)
			} else {
				for _, deferred := range []bool{false, true} {
					for _, chunk := range []int{1, 3, 17} {
						t.Run(fmt.Sprintf("limited-chunk%d-deferred%v", chunk, deferred), func(t *testing.T) {
							spArrayAggregateLimitedChunksDraft(t, c, chunk, deferred)
						})
					}
				}
			}
		})
	}
}

func spArrayAggregateLimitedChunksDraft(t *testing.T, c spCommandResourceDraftCase, chunk int, deferred bool) {
	t.Helper()
	require.Equal(t, 33, c.Budget.MaxCollectionElements)
	spSparkplugAggregateLimitedChunks(t, c, chunk, deferred, 3)
}

// The independent negative chooses the exact message that crosses the shared
// protobuf budget. All accepted history and the terminal event remain checked.
func spSparkplugAggregateLimitedChunks(t *testing.T, c spCommandResourceDraftCase, chunk int, deferred bool, prefix int) {
	t.Helper()
	b := DefaultParserBudget()
	b.MaxRecursionDepth = c.Budget.MaxRecursionDepth
	b.MaxCollectionElements = c.Budget.MaxCollectionElements
	var steps []spCommandResourceDraftStep
	spCommandResourceDraftDecode(t, c.Steps, &steps)
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(40000, 1883))
	require.NoError(t, err)
	defer s.Close("cleanup")
	s.(*captureSession).f.a.config.Deferred = deferred
	var history []spCommandResourceDraftHistory
	var flow uint64
	ids := map[uint64]bool{}
	recheck := func() {
		for _, h := range history {
			spCommandResourceDraftHistoryCheck(t, h)
		}
	}
	var totalInput uint64
	// Retain the accepted prefix; chunk only the independently selected limited message.
	for _, w := range steps[:prefix] {
		wire, err := hex.DecodeString(w.Input)
		require.NoError(t, err)
		require.Equal(t, w.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
		original := append([]byte(nil), wire...)
		r := s.Feed(w.Direction, time.Unix(w.Time, 0), wire)
		totalInput += uint64(len(wire))
		require.Nil(t, r.Err)
		require.False(t, r.NeedMore)
		require.Equal(t, len(wire), r.Consumed)
		require.Len(t, r.Events, 1)
		e := r.Events[0]
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		if flow == 0 {
			flow = e.FlowID
		}
		require.NotZero(t, flow)
		require.Equal(t, flow, e.FlowID)
		require.Equal(t, original, e.Raw)
		require.Equal(t, w.Protocol, e.Protocol)
		require.Equal(t, w.Transport, e.Transport)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, w.Source, e.Source)
		require.Equal(t, w.Destination, e.Destination)
		require.Equal(t, time.Unix(w.Time, 0), e.Timestamp)
		require.Equal(t, w.Offset, e.Offset)
		require.Equal(t, w.Length, e.Length)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Empty(t, e.Error)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		spAssertTree(t, w.Session, e.Session)
		spDataSetTypes(t, e.Session)
		fields, err := e.GetFields()
		require.NoError(t, err)
		history = append(history, spCommandResourceDraftHistory{event: e, session: w.Session, fields: cloneSession(fields)})
		for i := range wire {
			wire[i] ^= 255
		}
		for i := range e.Raw {
			e.Raw[i] ^= 255
		}
		spDataSetPoison(e.Session)
		spDataSetPoison(fields)
		recheck()
	}
	w := steps[prefix]
	require.NotNil(t, w.QueryError)
	require.Equal(t, ErrResourceExceeded, w.QueryError.Kind)
	wire, err := hex.DecodeString(w.Input)
	require.NoError(t, err)
	require.Equal(t, w.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
	var resource *ProtocolEvent
	for start := 0; start < len(wire); start += chunk {
		end := start + chunk
		if end > len(wire) {
			end = len(wire)
		}
		part := append([]byte(nil), wire[start:end]...)
		r := s.Feed(w.Direction, time.Unix(w.Time, 0), part)
		totalInput += uint64(len(part))
		require.Equal(t, len(part), r.Consumed)
		for i := range part {
			part[i] ^= 255
		}
		if end < len(wire) {
			require.Empty(t, r.Events)
			require.True(t, r.NeedMore)
			require.NotNil(t, r.Err)
			require.Equal(t, ErrNeedMore, r.Err.Kind)
		} else {
			require.False(t, r.NeedMore)
			require.Len(t, r.Events, 1)
			require.NotNil(t, r.Err)
			require.Equal(t, ErrResourceExceeded, r.Err.Kind)
			require.Equal(t, w.QueryError.Message, r.Err.Message)
			resource = r.Events[0]
			r.Err.Kind, r.Err.Message = ErrMalformedMessage, "poison returned Feed error"
		}
		recheck()
	}
	require.NotNil(t, resource)
	e := resource
	require.NotZero(t, e.ID)
	require.False(t, ids[e.ID])
	require.Equal(t, flow, e.FlowID)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, w.SHA, e.SourceBytes.SHA256)
	require.Equal(t, "reassembled", e.SourceBytes.Kind)
	require.Equal(t, "mqtt", e.Protocol)
	require.Equal(t, "tcp", e.Transport)
	require.Equal(t, w.Source, e.Source)
	require.Equal(t, w.Destination, e.Destination)
	require.Equal(t, w.Direction, e.Direction)
	require.Equal(t, time.Unix(w.Time, 0), e.Timestamp)
	require.Equal(t, w.Offset, e.Offset)
	require.Equal(t, w.Length, e.Length)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Equal(t, "limited", e.Status)
	require.Equal(t, "limited", e.Completeness)
	require.Equal(t, string(ErrResourceExceeded), e.ExpertCode)
	require.Equal(t, w.QueryError.Error(), e.Error)
	require.Equal(t, "mqtt", e.Summary)
	require.Nil(t, e.Session)
	require.Nil(t, e.Fields)
	history = append(history, spCommandResourceDraftHistory{event: e, err: &ProtocolError{Kind: w.QueryError.Kind, Message: w.QueryError.Message}})
	recheck()
	require.Zero(t, s.Stats().BufferedBytes)
	// Both directions are stopped. No late compatible command or Data may
	// silently re-create the definitions or recover this exact session.
	for _, late := range steps[prefix+1:] {
		wire, err := hex.DecodeString(late.Input)
		require.NoError(t, err)
		r := s.Feed(late.Direction, time.Unix(late.Time, 0), wire)
		totalInput += uint64(len(wire))
		require.Equal(t, len(wire), r.Consumed)
		require.Empty(t, r.Events)
		require.False(t, r.NeedMore)
		require.NotNil(t, r.Err)
		require.Equal(t, ErrFatalSessionError, r.Err.Kind)
		require.Equal(t, "session stopped", r.Err.Message)
		r.Err.Kind, r.Err.Message = ErrMalformedMessage, "poison stopped Feed error"
		require.Zero(t, s.Stats().BufferedBytes)
		recheck()
	}
	stats := s.Stats()
	require.Equal(t, totalInput, stats.InputBytes)
	require.Equal(t, uint64(prefix+1), stats.Messages)
	require.Equal(t, uint64(len(wire)), stats.LimitedBytes)
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	recheck()
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	recheck()
}
