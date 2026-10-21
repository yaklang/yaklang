package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func dlmsTCPIdleControls(t *testing.T) []dlmsListControl {
	return dlmsNamedBlockControls(t, "dlms-idle-flags", "owned-dlms-idle-flags/v1", 9)
}

func testDLMSIdleSession(t *testing.T) {
	controls := dlmsTCPIdleControls(t)
	raw, err := trafficfixture.ReadFile("dlms-idle-flags/controls.json")
	require.NoError(t, err)
	type step struct {
		Direction int
		Raw       string
	}
	type detail struct {
		ID, Answer string
		Steps      []step
		InputBytes uint64 `json:"input_bytes"`
	}
	var doc struct{ Cases []detail }
	require.NoError(t, json.Unmarshal(raw, &doc))
	for ci, c := range controls {
		d := doc.Cases[ci]
		require.Equal(t, c.ID, d.ID)
		answer, err := trafficfixture.ReadFile("dlms-idle-flags/" + d.Answer)
		require.NoError(t, err)
		var sealed detail
		require.NoError(t, json.Unmarshal(answer, &sealed))
		require.Equal(t, d, sealed, "all Feed steps and ledgers bind to the sealed answer hash")
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{1, 2, 7, 64, 2049} {
				t.Run(fmt.Sprintf("%s/deferred%t/chunk%d", c.ID, deferred, chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for si, step := range d.Steps {
						wire := wrapperWire(t, step.Raw)
						for at := 0; at < len(wire); at += chunk {
							owned := bytes.Clone(wire[at:min(len(wire), at+chunk)])
							out := s.Feed(step.Direction, time.Unix(int64(si+1), 0), owned)
							require.Equal(t, len(owned), out.Consumed)
							clear(owned)
							events = append(events, out.Events...)
						}
					}
					events = append(events, s.Close("complete")...)
					dlmsListAssert(t, c, events)
					ei := 0
					for si, step := range d.Steps {
						wire := wrapperWire(t, step.Raw)
						if bytes.Equal(wire, bytes.Repeat([]byte{0x7e}, len(wire))) {
							continue
						}
						require.Equal(t, time.Unix(int64(si+1), 0), events[ei].Timestamp, "idle-only arrival cannot replace the actual opening flag timestamp")
						ei++
					}
					stats := s.Stats()
					require.Equal(t, d.InputBytes, stats.InputBytes)
					require.NotNil(t, c.UnclassifiedBytes)
					require.Equal(t, *c.UnclassifiedBytes, stats.UnclassifiedBytes)
					var framed uint64
					for _, want := range c.Events {
						framed += uint64(len(want.Raw) / 2)
					}
					require.Equal(t, framed, stats.MessageBytes)
					require.Equal(t, stats.InputBytes, stats.MessageBytes+stats.UnclassifiedBytes)
					require.Zero(t, stats.BufferedBytes)
					require.Empty(t, s.Close("again"))
				})
			}
		}
	}
}

func testDLMSIdleBoundaries(t *testing.T) {
	q := wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e")
	r := wrapperWire(t, "7ea013210330d381e6e700c401c10012002a5b3f7e")
	t.Run("bounded-unidentified-prefix", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		defer s.Close("cleanup")
		prefix := bytes.Repeat([]byte{0x7e}, 64)
		require.Equal(t, ProbeReject, s.Probe(append(bytes.Clone(prefix), q...)).Verdict)
		out := s.Feed(0, time.Unix(1, 0), prefix)
		require.Empty(t, out.Events)
		out = s.Feed(0, time.Unix(1, 0), q)
		require.Len(t, out.Events, 1)
		require.Equal(t, "unrecognized", out.Events[0].Status)
		require.Empty(t, s.Feed(0, time.Unix(1, 0), q).Events)
		require.Zero(t, s.Stats().BufferedBytes)
	})
	for _, suffix := range [][]byte{{0x7e}, {0x7e, 0xa0}} {
		t.Run("close-"+hex.EncodeToString(suffix), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			require.Len(t, s.Feed(0, time.Unix(1, 0), q).Events, 1)
			require.Len(t, s.Feed(1, time.Unix(2, 0), r).Events, 1)
			require.Empty(t, s.Feed(0, time.Unix(3, 0), suffix).Events)
			closed := s.Close("complete")
			if len(suffix) == 1 {
				require.Empty(t, closed)
				require.EqualValues(t, 1, s.Stats().UnclassifiedBytes)
			} else {
				require.Len(t, closed, 1)
				require.Equal(t, "incomplete", closed[0].Status)
				require.Equal(t, suffix, closed[0].Raw)
			}
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("idle-cannot-hide-pending-request", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		defer s.Close("cleanup")
		require.Len(t, s.Feed(0, time.Unix(1, 0), q).Events, 1)
		require.Empty(t, s.Feed(0, time.Unix(2, 0), []byte{0x7e, 0x7e}).Events)
		closed := s.Close("incomplete")
		require.Len(t, closed, 1)
		require.Equal(t, "incomplete", closed[0].Status)
		require.Zero(t, closed[0].ResponseTo)
		require.EqualValues(t, 1, closed[0].Session["Outstanding"])
		require.EqualValues(t, 2, s.Stats().UnclassifiedBytes)
		require.Zero(t, s.Stats().BufferedBytes)
		require.Empty(t, s.Close("again"))
	})
	t.Run("no-garbage-resynchronization", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		defer s.Close("cleanup")
		garbage := append([]byte{0x7e, 0}, q...)
		require.Equal(t, ProbeReject, s.Probe(garbage).Verdict)
		require.Len(t, s.Feed(0, time.Unix(1, 0), q).Events, 1)
		out := s.Feed(1, time.Unix(2, 0), append([]byte{0x7e, 0}, r...))
		require.Len(t, out.Events, 1)
		fields, err := out.Events[0].GetFields()
		rocTypedError(t, "MalformedMessage", err)
		require.Nil(t, fields)
		require.Zero(t, out.Events[0].ResponseTo)
		require.Empty(t, s.Feed(1, time.Unix(3, 0), r).Events)
		s.Close("complete")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("udp-remains-one-exact-datagram", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		defer s.Close("cleanup")
		s.(*captureSession).f.a.datagramDecodeAs = map[uint16]string{4059: "dlms"}
		require.Equal(t, ProbeReject, s.Probe(append([]byte{0x7e}, q...)).Verdict)
		out := s.Feed(0, time.Unix(1, 0), q)
		require.Len(t, out.Events, 1)
		out = s.Feed(1, time.Unix(2, 0), append([]byte{0x7e}, r...))
		require.Len(t, out.Events, 1)
		fields, err := out.Events[0].GetFields()
		rocTypedError(t, "MalformedMessage", err)
		require.Nil(t, fields)
		require.Zero(t, out.Events[0].ResponseTo)
		s.Close("complete")
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
