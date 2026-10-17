package pcaputil

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func snMultiOwnedDiagnosis(t *testing.T, e *ProtocolEvent, kind, message string) {
	t.Helper()
	for i := 0; i < 2; i++ {
		fields, err := e.GetFields()
		require.Nil(t, fields)
		var pe *ProtocolError
		require.True(t, errors.As(err, &pe))
		require.Equal(t, kind, string(pe.Kind))
		require.Equal(t, message, pe.Message)
		pe.Kind, pe.Message = ErrContextRequired, "caller fields error"
		decoded, err := e.Decode()
		require.Nil(t, decoded)
		require.True(t, errors.As(err, &pe))
		require.Equal(t, kind, string(pe.Kind))
		require.Equal(t, message, pe.Message)
		pe.Kind, pe.Message = ErrNeedMore, "caller decode error"
	}
}

func TestDLMSWrapperSNMultiResultErrorOwnership(t *testing.T) {
	for _, control := range snMultiResultControls(t) {
		if control.Steps[1]["query_error"] == nil {
			continue
		}
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/d%t", control.Name, tr, deferred), func(t *testing.T) {
					budget := DefaultParserBudget()
					if control.Budget.Collection > 0 {
						budget.MaxCollectionElements = control.Budget.Collection
					}
					if control.Budget.Depth > 0 {
						budget.MaxRecursionDepth = control.Budget.Depth
					}
					session, err := NewProtocolSessionWithOptions(budget, WithSessionTransport(tr), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer session.Close("cleanup")
					session.(*captureSession).f.a.config.Deferred = deferred
					primer := wrapperWire(t, control.Steps[0]["input_hex"].(string))
					p := session.Feed(0, time.Unix(100, 0), primer)
					clear(primer)
					require.Nil(t, p.Err)
					require.False(t, p.NeedMore)
					require.Len(t, p.Events, 1)
					want := snReadStep(t, control.Steps[1], tr)
					kind := want["query_error"].(string)
					message := want["error_message"].(string)
					wire := wrapperWire(t, want["input_hex"].(string))
					in := append([]byte(nil), wire...)
					result := session.Feed(1, time.Unix(101, 0), in)
					clear(in)
					require.Equal(t, len(wire), result.Consumed)
					require.False(t, result.NeedMore)
					require.NotNil(t, result.Err)
					require.Equal(t, kind, string(result.Err.Kind))
					require.Equal(t, message, result.Err.Message)
					require.Len(t, result.Events, 1)
					e := result.Events[0]
					require.NotZero(t, e.ID)
					require.NotEqual(t, p.Events[0].ID, e.ID)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					require.Equal(t, "dlms-wrapper", e.Protocol)
					require.Equal(t, want["profile"], e.Profile)
					require.Equal(t, want["status"], e.Status)
					require.Equal(t, want["completeness"], e.Completeness)
					require.Equal(t, time.Unix(101, 0), e.Timestamp)
					require.Equal(t, CaptureDomain{}, e.Domain)
					require.Equal(t, 1, e.Direction)
					require.Zero(t, e.Offset)
					require.Equal(t, "192.0.2.2:4059", e.Source)
					require.Equal(t, "192.0.2.1:40000", e.Destination)
					require.Equal(t, len(wire), e.Length)
					if tr == "tcp" {
						require.Equal(t, p.Events[0].FlowID, e.FlowID)
						require.NotZero(t, e.FlowID)
					} else {
						require.Zero(t, e.FlowID)
					}
					if tr == "udp" && kind == "ResourceExceeded" {
						require.Nil(t, e.Raw)
					} else {
						require.Equal(t, wire, e.Raw)
					}
					require.Nil(t, e.Fields)
					require.Nil(t, e.Session)
					require.Nil(t, e.Structured)
					require.Equal(t, kind+": "+message, e.Error)
					result.Err.Kind, result.Err.Message = ErrNeedMore, "caller FeedResult error"
					snMultiOwnedDiagnosis(t, e, kind, message)
					require.Equal(t, kind+": "+message, e.Error)
					e.Error = "caller public Error"
					snMultiOwnedDiagnosis(t, e, kind, message)
					require.Empty(t, session.Close("ownership"))
					require.Empty(t, session.Close("again"))
					require.Zero(t, session.Stats().BufferedBytes)
				})
			}
		}
	}
}
