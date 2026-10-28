package pcaputil

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRADIUSByteIndependentPartialClose(t *testing.T) {
	s, e := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, e)
	defer s.Close("cleanup")
	q := radiusPkt(1, 7, append(radiusAttr(1, []byte("alice")), radiusAttr(4, []byte{192, 0, 2, 9})...))
	r := radiusPkt(2, 7, nil)
	require.Len(t, q, 33)
	require.Len(t, r, 20)
	o := s.Feed(0, time.Unix(100, 0), q)
	require.Nil(t, o.Err)
	require.Len(t, o.Events, 1)
	owner := o.Events[0].ID
	o = s.Feed(1, time.Unix(101, 0), r)
	require.Nil(t, o.Err)
	require.Len(t, o.Events, 1)
	require.Equal(t, owner, o.Events[0].TransactionID)
	require.Equal(t, owner, o.Events[0].ResponseTo)
	partial := []byte{4, 7}
	o = s.Feed(0, time.Unix(102, 0), partial)
	require.NotNil(t, o.Err)
	require.Equal(t, ErrNeedMore, o.Err.Kind)
	require.Empty(t, o.Events)
	require.Equal(t, 2, o.Consumed)
	clear(partial)
	closeEvents := s.Close("complete")
	require.Len(t, closeEvents, 1)
	v := closeEvents[0]
	require.Equal(t, "radius", v.Protocol)
	require.Equal(t, "incomplete", v.Status)
	require.Equal(t, "incomplete", v.Completeness)
	require.Equal(t, []byte{4, 7}, v.Raw)
	require.Equal(t, 2, v.Length)
	require.Equal(t, uint64(33), v.Offset)
	require.Equal(t, 0, v.Direction)
	require.True(t, time.Unix(102, 0).Equal(v.Timestamp))
	require.Zero(t, v.TransactionID)
	require.Zero(t, v.ResponseTo)
	require.Nil(t, v.Session)
	saved := bytes.Clone(v.Raw)
	f, before := v.GetFields()
	require.Nil(t, f)
	v.Protocol = "http"
	v.Status = "decoded"
	v.Fields = map[string]any{"forged": true}
	v.Structured = map[string]any{"fields": map[string]any{"forged": true}}
	v.Session = map[string]any{"forged": true}
	clear(v.Raw)
	f, after := v.GetFields()
	tree, decodeErr := v.Decode()
	stats := s.Stats()
	report := map[string]any{"original_raw": saved, "offset": v.Offset, "length": v.Length, "before_error": nil, "after_error": nil, "decode_error": nil, "forged_fields": f, "forged_decode": tree, "stats": stats}
	if before != nil {
		report["before_error"] = before.Error()
	}
	if after != nil {
		report["after_error"] = after.Error()
	}
	if decodeErr != nil {
		report["decode_error"] = decodeErr.Error()
	}
	bb, e := json.Marshal(report)
	require.NoError(t, e)
	t.Log(string(bb))
	require.Empty(t, s.Close("again"))
	require.Zero(t, stats.BufferedBytes)
	require.EqualValues(t, 2, stats.Messages)
	require.EqualValues(t, 53, stats.MessageBytes)
	require.EqualValues(t, 55, stats.InputBytes)
	require.EqualValues(t, 1, stats.Incomplete)
	require.Zero(t, stats.Malformed)
	rocTypedError(t, "NeedMore", before)
	rocTypedError(t, "NeedMore", after)
	rocTypedError(t, "NeedMore", decodeErr)
	require.Nil(t, f)
	require.Nil(t, tree)
}
