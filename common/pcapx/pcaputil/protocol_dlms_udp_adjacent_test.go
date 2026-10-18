package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func testDLMSUDPIdleFreshIdentityExistingAPI(t *testing.T) {
	for name, wires := range map[string][]string{"native": {"7ea0190321107fdae6e600c001c1000f0000280000ff020091537e", "7ea012210330689de6e700c401c100112a43c37e", "7ea0190321326fd8e6e600c001c2000f0000280000ff020022ad7e", "7ea0122103527cdde6e700c401c200112a8ee67e"},
		"wrapper": {"000100100001000dc001c1000f0000280000ff0200", "0001000100100006c401c100112a", "000100100001000dc001c2000f0000280000ff0200", "0001000100100006c401c200112a"}} {
		t.Run(name, func(t *testing.T) {
			for _, deferred := range []bool{false, true} {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				feed := func(dir int, at int64, w string) *ProtocolEvent {
					input := wrapperWire(t, w)
					b := s.Feed(dir, time.Unix(at, 0), input)
					for i := range input {
						input[i] ^= 255
					}
					require.Len(t, b.Events, 1)
					require.Equal(t, wrapperWire(t, w), b.Events[0].Raw)
					return b.Events[0]
				}
				a := feed(0, 100, wires[0])
				ar := feed(1, 100, wires[1])
				_, err = ar.GetFields()
				require.NoError(t, err)
				require.Equal(t, a.ID, ar.ResponseTo)
				b := feed(0, 131, wires[2])
				_, err = b.GetFields()
				require.NoError(t, err, "complete old exchange must not block a fresh distinct identity")
				late := feed(1, 131, wires[1])
				require.Zero(t, late.ResponseTo)
				require.Zero(t, late.TransactionID)
				br := feed(1, 131, wires[3])
				_, err = br.GetFields()
				require.NoError(t, err)
				require.Equal(t, b.ID, br.ResponseTo)
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			}
		})
	}
}

// Completed identity history is retained, bounded and charged. An unused
// identity is permitted only if a new history slot can be reserved; exhaustion
// must not evict the previous token and accidentally trust its delayed reply.
func testDLMSUDPIdleCompletedHistoryBudget(t *testing.T) {
	controls := dlmsIdleControlsFrom(t, "dlms-udp-adjacent", "owned-dlms-udp-adjacent/v1", 18)
	for _, c := range controls {
		if c.ID != "native-new-invoke-31" && c.ID != "wrapper-new-invoke-31" {
			continue
		}
		t.Run(c.Protocol, func(t *testing.T) {
			for _, limit := range []int{1, 2} {
				budget := DefaultParserBudget()
				budget.MaxCollectionElements = limit
				s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				feed := func(dir int, at int64, wire string) *ProtocolEvent {
					b := s.Feed(dir, time.Unix(at, 0), wrapperWire(t, wire))
					require.Len(t, b.Events, 1)
					return b.Events[0]
				}
				a := feed(0, 100, c.Events[0].Raw)
				r := feed(1, 100, c.Events[1].Raw)
				_, err = r.GetFields()
				require.NoError(t, err)
				require.Equal(t, a.ID, r.ResponseTo)
				// Trigger expiry without creating a second identity.
				late := feed(1, 130, c.Events[1].Raw)
				require.Zero(t, late.ResponseTo)
				require.Zero(t, late.TransactionID)
				retained := int64(512)
				if c.Protocol == "dlms-wrapper" {
					retained += 256
				}
				auditor := s.(*captureSession).f.a
				for _, el := range auditor.udpSessions.entries {
					require.GreaterOrEqual(t, el.Value.(*binUDPEntry).flow.sessionBytes, retained)
				}
				b := feed(0, 131, c.Events[2].Raw)
				_, err = b.GetFields()
				if limit == 1 {
					rocTypedError(t, "ResourceExceeded", err)
					r = feed(1, 131, c.Events[3].Raw)
					require.Zero(t, r.ResponseTo)
					require.Zero(t, r.TransactionID)
				} else {
					require.NoError(t, err)
					r = feed(1, 131, c.Events[3].Raw)
					_, err = r.GetFields()
					require.NoError(t, err)
					require.Equal(t, b.ID, r.ResponseTo)
				}
				late = feed(1, 131, c.Events[1].Raw)
				require.Zero(t, late.ResponseTo)
				require.Zero(t, late.TransactionID)
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			}
		})
	}
}
