package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type accountingByteStep struct {
	Direction    int
	At           int64  `json:"timestamp_unix"`
	Wire         string `json:"input_hex"`
	Error        string
	Profile      string `json:"expected_profile"`
	Completeness string
	Reply        *int           `json:"response_to_step"`
	Transaction  *int           `json:"transaction_step"`
	Session      map[string]any `json:"expected_legacy_session"`
	Legacy       map[string]any `json:"legacy_fields_hexstrings"`
	Outstanding  *int           `json:"close_outstanding"`
}
type accountingByteControl struct {
	Name   string
	Steps  []accountingByteStep
	Budget struct{ MaxMessageBytes, MaxCollectionElements int }
}

func accountingByteControls(t *testing.T) ([]accountingByteControl, map[string]string, []accountingByteStep) {
	t.Helper()
	var x struct{ Cases []accountingByteControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-accounting", "RADIUS_ACCOUNTING_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 49)
	raw := finsReadBatch(t, "radius-accounting-byte", "RADIUS_ACCOUNTING_BYTE_TEST_ROOT", "carrier49.json")
	var m struct {
		Mapping []struct{ Name, Disposition string }
		Primer  struct {
			Steps []struct {
				Direction    int
				At           int64  `json:"timestamp_unix"`
				Wire         string `json:"input_hex"`
				Profile      string
				Completeness string
				Session      map[string]any `json:"canonical_session_hexbytes"`
				Legacy       map[string]any `json:"legacy_fields_hexstrings"`
			}
		}
	}
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Len(t, m.Mapping, 49)
	require.Len(t, m.Primer.Steps, 2)
	mapping := map[string]string{}
	for _, r := range m.Mapping {
		mapping[r.Name] = r.Disposition
	}
	var prime []accountingByteStep
	zero := 0
	for i, p := range m.Primer.Steps {
		s := accountingByteStep{Direction: p.Direction, At: p.At, Wire: p.Wire, Profile: p.Profile, Completeness: p.Completeness, Session: p.Session, Legacy: p.Legacy, Transaction: &zero}
		if i == 1 {
			s.Reply = &zero
		}
		prime = append(prime, s)
	}
	return x.Cases, mapping, prime
}
func accountingByteTypedLegacy(t *testing.T, in map[string]any) map[string]any {
	// The historical Accounting schema stores User-Name as literal text and
	// unknown Attribute Data as hex. Other compatibility leaves follow the
	// same fixed schema. Normalise only this answer representation.
	m := cloneSession(in)
	if a, ok := m["Attributes"].([]any); ok {
		for _, v := range a {
			a := v.(map[string]any)
			if v, ok := a["User-Name"].(string); ok {
				a["User-Name"] = fmt.Sprintf("%x", []byte(v))
			}
		}
	}
	return accountingByteWant(t, m, true)
}
func accountingByteAssert(t *testing.T, c accountingByteControl, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	extra := 0
	for _, s := range c.Steps {
		if s.Outstanding != nil {
			extra = 1
		}
	}
	require.Len(t, events, len(c.Steps)+extra)
	offsets := [2]uint64{}
	ids := map[uint64]bool{}
	for i, w := range c.Steps {
		e := events[i]
		wire := radiusWire(t, w.Wire)
		require.Equal(t, "radius", e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, "tcp", e.Transport)
		require.Equal(t, CaptureDomain{}, e.Domain)
		require.NotZero(t, e.FlowID)
		require.Equal(t, events[0].FlowID, e.FlowID)
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, offsets[w.Direction], e.Offset)
		offsets[w.Direction] += uint64(len(wire))
		require.True(t, time.Unix(w.At, 0).Equal(e.Timestamp))
		src, dst := "192.0.2.1:40000", "192.0.2.2:40001"
		if w.Direction == 1 {
			src, dst = dst, src
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		require.Equal(t, wire, e.Raw)
		require.Equal(t, len(wire), e.Length)
		require.Empty(t, e.SourceBytes.PacketRefs)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		switch w.Error {
		case "MalformedMessage":
			status = "malformed"
		case "ResourceExceeded":
			status = "limited"
		}
		require.Equal(t, status, e.Status)
		complete := "message"
		if w.Error != "" {
			complete = status
		}
		require.Equal(t, complete, e.Completeness)
		if w.Reply == nil {
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
		}
		if w.Transaction == nil {
			require.Zero(t, e.TransactionID)
		} else {
			require.Equal(t, events[*w.Transaction].ID, e.TransactionID)
		}
		var want map[string]any
		if w.Error == "" {
			want = accountingByteWant(t, w.Session, false)
		}
		require.Equal(t, want, e.Session)
		f, err := e.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error == "" {
			require.Equal(t, accountingByteTypedLegacy(t, w.Legacy), f)
			tree, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, want, tree["session"])
			mutateRTPSQuery(tree)
			mutateRTPSQuery(f)
		} else {
			require.Nil(t, f)
		}
		tlsDTLSPoison(e, "http")
		f, err = e.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error == "" {
			require.Equal(t, accountingByteTypedLegacy(t, w.Legacy), f)
		} else {
			require.Nil(t, f)
			tree, err := e.Decode()
			require.Nil(t, tree)
			rocTypedError(t, w.Error, err)
		}
	}
	if extra == 1 {
		e := events[len(c.Steps)]
		require.Equal(t, "radius", e.Protocol)
		require.Equal(t, "radius-accounting-observed", e.Profile)
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, "incomplete", e.Completeness)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Equal(t, map[string]any{"Outstanding": 1}, e.Session)
		f, err := e.GetFields()
		require.Nil(t, f)
		rocTypedError(t, "NeedMore", err)
	}
}
func TestRADIUSAccountingByteMatrix(t *testing.T) {
	controls, mapping, prime := accountingByteControls(t)
	eligible := 0
	for _, original := range controls {
		kind, ok := mapping[original.Name]
		require.True(t, ok)
		if kind == "separate-carrier-boundary" {
			continue
		}
		eligible++
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{0, 1, 7} {
				t.Run(fmt.Sprintf("%s/d%t/c%d", original.Name, deferred, chunk), func(t *testing.T) {
					c := original
					c.Steps = append([]accountingByteStep(nil), original.Steps...)
					if kind == "primed" {
						for i := range c.Steps {
							if c.Steps[i].Reply != nil {
								n := *c.Steps[i].Reply + 2
								c.Steps[i].Reply = &n
							}
							if c.Steps[i].Transaction != nil {
								n := *c.Steps[i].Transaction + 2
								c.Steps[i].Transaction = &n
							}
						}
						c.Steps = append(append([]accountingByteStep(nil), prime...), c.Steps...)
					}
					b := DefaultParserBudget()
					if c.Budget.MaxCollectionElements > 0 {
						b.MaxCollectionElements = c.Budget.MaxCollectionElements
					}
					s, err := NewProtocolSession(b)
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					total, limited, malformed := 0, 0, 0
					for _, w := range c.Steps {
						wire := radiusWire(t, w.Wire)
						total += len(wire)
						if w.Error == "ResourceExceeded" {
							limited += len(wire)
						}
						if w.Error == "MalformedMessage" {
							malformed++
						}
						z := chunk
						if z == 0 {
							z = len(wire)
						}
						for at := 0; at < len(wire); at += z {
							part := bytes.Clone(wire[at:min(at+z, len(wire))])
							n := len(part)
							o := s.Feed(w.Direction, time.Unix(w.At, 0), part)
							clear(part)
							require.Equal(t, n, o.Consumed)
							if len(o.Events) > 0 {
								if w.Error == "" {
									require.Nil(t, o.Err)
								} else {
									rocTypedError(t, w.Error, o.Err)
								}
							} else if o.Err != nil {
								require.Equal(t, ErrNeedMore, o.Err.Kind)
							}
							events = append(events, o.Events...)
						}
					}
					events = append(events, s.Close("complete")...)
					require.Empty(t, s.Close("again"))
					st := s.Stats()
					require.Zero(t, st.BufferedBytes)
					require.Zero(t, st.Flows)
					require.EqualValues(t, total, st.InputBytes)
					require.EqualValues(t, total, st.MessageBytes)
					require.EqualValues(t, len(c.Steps), st.Messages)
					require.EqualValues(t, limited, st.LimitedBytes)
					require.EqualValues(t, malformed, st.Malformed)
					accountingByteAssert(t, c, events, deferred)
				})
			}
		}
	}
	require.Equal(t, 47, eligible)
}
