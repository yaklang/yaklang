package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func accountingByteCase(t *testing.T, name string) accountingByteControl {
	t.Helper()
	cs, _, _ := accountingByteControls(t)
	for _, c := range cs {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing fixed historical control %s", name)
	return accountingByteControl{}
}
func TestRADIUSAccountingByteCoalesced(t *testing.T) {
	for _, d := range []bool{false, true} {
		t.Run(fmt.Sprint(d), func(t *testing.T) {
			c := accountingByteCase(t, "delay-changed-new-id")
			require.Len(t, c.Steps, 4)
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = d
			var events []*ProtocolEvent
			total := 0
			for _, start := range []int{0, 2} {
				var wire []byte
				for i := start; i < start+2; i++ {
					c.Steps[i].At = c.Steps[start].At
					wire = append(wire, radiusWire(t, c.Steps[i].Wire)...)
				}
				total += len(wire)
				o := s.Feed(c.Steps[start].Direction, time.Unix(c.Steps[start].At, 0), wire)
				clear(wire)
				require.Nil(t, o.Err)
				require.Len(t, o.Events, 2)
				events = append(events, o.Events...)
			}
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			st := s.Stats()
			require.Zero(t, st.BufferedBytes)
			require.Zero(t, st.Flows)
			require.EqualValues(t, total, st.InputBytes)
			require.EqualValues(t, total, st.MessageBytes)
			require.EqualValues(t, 4, st.Messages)
			accountingByteAssert(t, c, events, d)
		})
	}
}
func TestRADIUSAccountingBytePaddingBoundary(t *testing.T) {
	c := accountingByteCase(t, "padding")
	for _, d := range []bool{false, true} {
		for _, chunk := range []int{0, 1, 7} {
			t.Run(fmt.Sprintf("d%t/c%d", d, chunk), func(t *testing.T) {
				s, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = d
				want := c
				want.Steps = append([]accountingByteStep(nil), c.Steps...)
				var events []*ProtocolEvent
				var tails [][]byte
				total, complete := 0, 0
				for i, w := range c.Steps {
					wire := radiusWire(t, w.Wire)
					n := int(binary.BigEndian.Uint16(wire[2:4]))
					tails = append(tails, bytes.Clone(wire[n:]))
					want.Steps[i].Wire = fmt.Sprintf("%x", wire[:n])
					want.Steps[i].Session = cloneSession(w.Session)
					want.Steps[i].Session["Padding Bytes"] = float64(0)
					total += len(wire)
					complete += n
					z := chunk
					if z == 0 {
						z = len(wire)
					}
					for at := 0; at < len(wire); at += z {
						part := bytes.Clone(wire[at:min(at+z, len(wire))])
						o := s.Feed(w.Direction, time.Unix(w.At, 0), part)
						clear(part)
						if o.Err != nil {
							require.Equal(t, ErrNeedMore, o.Err.Kind)
						}
						events = append(events, o.Events...)
					}
				}
				require.Len(t, events, 2)
				accountingByteAssert(t, want, events, d)
				closeEvents := s.Close("complete")
				require.Len(t, closeEvents, 2)
				for i, e := range closeEvents {
					w := c.Steps[i]
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, tails[i], e.Raw)
					require.Equal(t, len(tails[i]), e.Length)
					require.EqualValues(t, len(radiusWire(t, w.Wire))-len(tails[i]), e.Offset)
					require.True(t, time.Unix(w.At, 0).Equal(e.Timestamp))
					require.Equal(t, "radius", e.Protocol)
					require.Equal(t, "incomplete", e.Status)
					require.Equal(t, "incomplete", e.Completeness)
					require.Nil(t, e.Session)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					f, err := e.GetFields()
					require.Nil(t, f)
					rocTypedError(t, "NeedMore", err)
					tlsDTLSPoison(e, "http")
					tree, err := e.Decode()
					require.Nil(t, tree)
					rocTypedError(t, "NeedMore", err)
				}
				require.Empty(t, s.Close("again"))
				st := s.Stats()
				require.Zero(t, st.BufferedBytes)
				require.Zero(t, st.Flows)
				require.EqualValues(t, total, st.InputBytes)
				require.EqualValues(t, complete, st.MessageBytes)
				require.EqualValues(t, 2, st.Messages)
				require.EqualValues(t, 2, st.Incomplete)
				require.Zero(t, st.Malformed)
			})
		}
	}
}
func TestRADIUSAccountingByteMessageBudgetBoundary(t *testing.T) {
	c := accountingByteCase(t, "byte-budget")
	_, _, prime := accountingByteControls(t)
	for _, d := range []bool{false, true} {
		t.Run(fmt.Sprint(d), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes = 64
			s, err := NewProtocolSession(b)
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = d
			var events []*ProtocolEvent
			total, complete := 0, 0
			for _, w := range prime {
				wire := radiusWire(t, w.Wire)
				total += len(wire)
				complete += len(wire)
				o := s.Feed(w.Direction, time.Unix(w.At, 0), wire)
				require.Nil(t, o.Err)
				events = append(events, o.Events...)
			}
			accountingByteAssert(t, accountingByteControl{Steps: prime}, events, d)
			require.Len(t, c.Steps, 2)
			for i, w := range c.Steps {
				wire := radiusWire(t, w.Wire)
				total += len(wire)
				o := s.Feed(w.Direction, time.Unix(w.At, 0), wire)
				clear(wire)
				if i == 0 {
					rocTypedError(t, "ResourceExceeded", o.Err)
					require.Len(t, o.Events, 1)
					e := o.Events[0]
					require.Equal(t, "radius", e.Protocol)
					require.Equal(t, "limited", e.Status)
					require.Equal(t, "limited", e.Completeness)
					require.Equal(t, 109, e.Length)
					require.Equal(t, radiusWire(t, w.Wire)[:64], e.Raw)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					require.Nil(t, e.Session)
					f, err := e.GetFields()
					require.Nil(t, f)
					rocTypedError(t, "ResourceExceeded", err)
					tlsDTLSPoison(e, "http")
					tree, err := e.Decode()
					require.Nil(t, tree)
					rocTypedError(t, "ResourceExceeded", err)
				} else {
					rocTypedError(t, "FatalSessionError", o.Err)
					require.Empty(t, o.Events)
				}
			}
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			st := s.Stats()
			require.Zero(t, st.BufferedBytes)
			require.Zero(t, st.Flows)
			require.EqualValues(t, total, st.InputBytes)
			require.EqualValues(t, complete, st.MessageBytes)
			require.EqualValues(t, 2, st.Messages)
			require.EqualValues(t, 109, st.LimitedBytes)
			require.EqualValues(t, 38, st.UnclassifiedBytes)
			require.Zero(t, st.Malformed)
			require.Zero(t, st.Incomplete)
		})
	}
}
