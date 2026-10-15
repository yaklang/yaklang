package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Poison every public result before feeding the next datagram, while the
// incomplete method body and outer request owners are still retained.
func TestRADIUSTLSChainActiveOwnership(t *testing.T) {
	for _, c := range radiusTLSChainControls(t) {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", c.Name, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(radiusEAPBudget(c), WithSessionTransport("udp"), WithSessionPorts(c.Ports[0], c.Ports[1]))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				var events []*ProtocolEvent
				var originals []*ProtocolEvent
				for i, w := range c.Steps {
					raw := radiusWire(t, w.Wire)
					out := s.Feed(w.Direction, time.Unix(w.At, w.Nanos), raw)
					clear(raw)
					require.Equal(t, len(raw), out.Consumed)
					if w.Error == "" {
						require.Nil(t, out.Err)
					} else {
						rocTypedError(t, w.Error, out.Err)
					}
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					want := radiusEAPWant(t, c, i)
					require.Equal(t, want, e.Session)
					f, err := e.GetFields()
					rocTypedError(t, w.Error, err)
					if w.Error == "" {
						require.Equal(t, want, f)
						mutateRTPSQuery(f)
						radiusEAPMutateStringLists(f)
						decoded, err := e.Decode()
						require.NoError(t, err)
						require.Equal(t, want, decoded["session"])
						mutateRTPSQuery(decoded)
						radiusEAPMutateStringLists(decoded)
					} else {
						require.Nil(t, f)
					}
					snapshot := *e
					snapshot.Raw = bytes.Clone(e.Raw)
					snapshot.Session = cloneSession(e.Session)
					snapshot.Fields = cloneSession(e.Fields)
					snapshot.Structured = cloneSession(e.Structured)
					snapshot.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
					events = append(events, &snapshot)
					originals = append(originals, e)
					mutateRTPSQuery(e.Session)
					radiusEAPMutateStringLists(e.Session)
					tlsDTLSPoison(e, "http")
					e.ID, e.FlowID, e.TransactionID, e.ResponseTo = 999999, 999999, 999999, 999999
					f, err = e.GetFields()
					rocTypedError(t, w.Error, err)
					if w.Error == "" {
						require.Equal(t, want, f)
					} else {
						require.Nil(t, f)
					}
				}
				events = append(events, s.Close("complete")...)
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				radiusEAPCheck(t, c, events, false, deferred)
				for i, e := range originals {
					f, err := e.GetFields()
					rocTypedError(t, c.Steps[i].Error, err)
					if c.Steps[i].Error == "" {
						require.Equal(t, radiusEAPWant(t, c, i), f)
					} else {
						require.Nil(t, f)
					}
				}
			})
		}
	}
}

// A known aggregate decoder refusal must release the admitted prefix, not keep
// it behind a completed/tombstone flag. Outer IDs and the independent neighbor
// are asserted by the same full-answer checker as the ordinary matrix.
func TestRADIUSTLSChainResourceRelease(t *testing.T) {
	found := false
	for _, c := range radiusTLSChainControls(t) {
		if c.Budget.MaxCollectionElements == 0 {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(radiusEAPBudget(c), WithSessionTransport("udp"), WithSessionPorts(c.Ports[0], c.Ports[1]))
			require.NoError(t, err)
			defer s.Close("cleanup")
			var events []*ProtocolEvent
			pendingBytes := int64(0)
			for i, w := range c.Steps {
				before := s.Stats().BufferedBytes
				raw := radiusWire(t, w.Wire)
				out := s.Feed(w.Direction, time.Unix(w.At, w.Nanos), raw)
				clear(raw)
				require.Equal(t, len(raw), out.Consumed)
				if w.Error == "" {
					require.Nil(t, out.Err)
				} else {
					rocTypedError(t, w.Error, out.Err)
				}
				require.Len(t, out.Events, 1)
				events = append(events, out.Events...)
				want := radiusEAPWant(t, c, i)
				require.Equal(t, want, out.Events[0].Session)
				if pendingBytes > 0 && w.Error == "ResourceExceeded" {
					found = true
					require.LessOrEqual(t, s.Stats().BufferedBytes, before-pendingBytes, "refused aggregate must release at least its owned prefix without discarding the neighbor's outer ID")
					pendingBytes = 0
				}
				if inner, ok := want["EAP"].(map[string]any); ok {
					if tls, ok := inner["TLS"].(map[string]any); ok && tls["Fragment Status"] == "pending" {
						pendingBytes = int64(tls["Collected Bytes"].(int))
					}
				}
			}
			events = append(events, s.Close("complete")...)
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			radiusEAPCheck(t, c, events, false, false)
		})
	}
	require.True(t, found, "finite independent aggregate-refusal fixture must actually execute")
}
