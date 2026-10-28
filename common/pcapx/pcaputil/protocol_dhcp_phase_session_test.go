package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDHCPPhaseNativeDatagrams(t *testing.T) {
	for _, c := range dhcpPhaseControls(t) {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", c.name, deferred), func(t *testing.T) {
				sp, dp := uint16(68), uint16(67)
				if c.v6 {
					sp, dp = 546, 547
				}
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(sp, dp))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				var events []*ProtocolEvent
				for i, w := range c.steps {
					dir := 0
					if w.reverse {
						dir = 1
					}
					input := bytes.Clone(w.wire)
					out := s.Feed(dir, time.Unix(w.ts, 0), input)
					clear(input)
					if w.err == "" {
						require.Nil(t, out.Err)
					} else {
						rocTypedError(t, w.err, out.Err)
					}
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					events = append(events, e)
					require.Equal(t, w.wire, e.Raw)
					require.Equal(t, len(w.wire), e.Length)
					require.True(t, time.Unix(w.ts, 0).Equal(e.Timestamp))
					require.Empty(t, e.SourceBytes.PacketRefs)
					require.Equal(t, CaptureDomain{}, e.Domain)
					if w.tx < 0 {
						require.Zero(t, e.TransactionID)
					} else {
						require.Equal(t, events[w.tx].ID, e.TransactionID)
					}
					if w.resp < 0 {
						require.Zero(t, e.ResponseTo)
					} else {
						require.Equal(t, events[w.resp].ID, e.ResponseTo)
					}
					f, err := e.GetFields()
					rocTypedError(t, w.err, err)
					if w.err == "" {
						require.Equal(t, w.fields, f)
						require.Equal(t, w.fields, e.Session)
						tree, err := e.Decode()
						require.NoError(t, err)
						require.Equal(t, w.fields, tree["session"])
						mutateRTPSQuery(tree)
					} else {
						require.Nil(t, f)
					}
					tlsDTLSPoison(e, "http")
					f, err = e.GetFields()
					rocTypedError(t, w.err, err)
					if w.err == "" {
						require.Equal(t, w.fields, f)
					} else {
						require.Nil(t, f)
					}
					_ = i
				}
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
func TestDHCPPhaseCompactStateAndOwnership(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6 := dhcp6OwnedInputs(t)
	for _, six := range []bool{false, true} {
		t.Run(fmt.Sprintf("v6%t", six), func(t *testing.T) {
			q, r := dhcpPhaseAddServer(v4[2], [4]byte{192, 0, 2, 67}), dhcpPhaseAddServer(v4[3], [4]byte{192, 0, 2, 67})
			sp, dp := uint16(68), uint16(67)
			if six {
				q, r = v6[2], v6[3]
				sp, dp = 546, 547
			}
			b := DefaultParserBudget()
			b.MaxMessageBytes = 512
			b.MaxBufferedBytes = 512
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(sp, dp))
			require.NoError(t, err)
			defer s.Close("cleanup")
			out := s.Feed(0, time.Unix(100, 0), q)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			require.Equal(t, e.ID, e.TransactionID)
			id := e.ID
			retained := s.Stats().BufferedBytes
			require.Positive(t, retained)
			require.LessOrEqual(t, retained, int64(512))
			f, err := e.GetFields()
			require.NoError(t, err)
			mutateRTPSQuery(f)
			tlsDTLSPoison(e, "http")
			e.ID = 0
			e.Source = "caller"
			reply := s.Feed(1, time.Unix(101, 0), r)
			require.Nil(t, reply.Err)
			require.Len(t, reply.Events, 1)
			require.Equal(t, id, reply.Events[0].ResponseTo)
			require.Equal(t, id, reply.Events[0].TransactionID)
			require.Equal(t, retained, s.Stats().BufferedBytes)
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(512))
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
