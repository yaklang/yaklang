package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDLMSACSESegmentedExistingAPI(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	defer s.Close("baseline")
	var qid uint64
	x0 := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea81d032110b3f2e6e600601da109060760857405080101be10040e47017e"))
	require.Nil(t, x0.Err, "selected fixed-reference segmented ACSE must be observed")
	require.Len(t, x0.Events, 1)
	x1 := s.Feed(0, time.Unix(2, 0), wrapperWire(t, "7ea0170321122f5701000000065f1f0400000010040007d17e"))
	require.Nil(t, x1.Err, "selected fixed-reference segmented ACSE must be observed")
	require.Len(t, x1.Events, 1)
	qid = x1.Events[0].ID
	x2 := s.Feed(1, time.Unix(3, 0), wrapperWire(t, "7ea81d210350b716e6e7006129a109060760857405080101a2030201a9d27e"))
	require.Nil(t, x2.Err, "selected fixed-reference segmented ACSE must be observed")
	require.Len(t, x2.Events, 1)
	x3 := s.Feed(1, time.Unix(4, 0), wrapperWire(t, "7ea81d210352a53500a305a103020100be10040e0800065f1f040000d3e17e"))
	require.Nil(t, x3.Err, "selected fixed-reference segmented ACSE must be observed")
	require.Len(t, x3.Events, 1)
	x4 := s.Feed(1, time.Unix(5, 0), wrapperWire(t, "7ea00f21035464f000100400000760317e"))
	require.Nil(t, x4.Err, "selected fixed-reference segmented ACSE must be observed")
	require.Len(t, x4.Events, 1)
	require.Equal(t, qid, x4.Events[0].ResponseTo)
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSACSESegmentedAdjacentComplete(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	defer s.Close("adjacent")
	q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea02b032110fbafe6e600601da109060760857405080101be10040e01000000065f1f0400000010040081e17e"))
	require.Nil(t, q.Err)
	require.Len(t, q.Events, 1)
	a := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea0372103306c7ce6e7006129a109060760857405080101a203020100a305a103020100be10040e0800065f1f04000000100400000786a17e"))
	require.Nil(t, a.Err)
	require.Equal(t, q.Events[0].ID, a.Events[0].ResponseTo)
}

func acseSegmentControls(t *testing.T) []dlmsListControl {
	t.Helper()
	if root := os.Getenv("DLMS_ACSE_SEG_TEST_ROOT"); root != "" {
		b, e := os.ReadFile(filepath.Join(root, "controls.json"))
		require.NoError(t, e)
		var m struct {
			Schema string
			Cases  []dlmsListControl
		}
		require.NoError(t, json.Unmarshal(b, &m))
		require.Equal(t, "owned-dlms-acse-seg/v1", m.Schema)
		require.Len(t, m.Cases, 36)
		return m.Cases
	}
	return dlmsNamedBlockControls(t, "dlms-acse-segmented", "owned-dlms-acse-seg/v1", 36)
}
func TestDLMSACSESegmentedSealedMatrix(t *testing.T) { dlmsSealedMatrix(t, acseSegmentControls(t)) }
func TestDLMSACSESegmentedSealedOwnership(t *testing.T) {
	dlmsOwnershipAndChunks(t, acseSegmentControls(t))
}

func acseSegmentAccepted(t *testing.T, transport string) dlmsListControl {
	t.Helper()
	for _, c := range acseSegmentControls(t) {
		if c.ID == "accepted-"+transport {
			return c
		}
	}
	t.Fatal("missing independent accepted control")
	return dlmsListControl{}
}
func TestDLMSACSESegmentedLiveOwnership(t *testing.T) {
	for _, tr := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", tr, deferred), func(t *testing.T) {
				c := acseSegmentAccepted(t, tr)
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("test")
				s.(*captureSession).f.a.config.Deferred = deferred
				var requestID uint64
				for i, w := range c.Events {
					input := wrapperWire(t, w.Raw)
					out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), input)
					clear(input)
					require.Nil(t, out.Err)
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					fields, err := e.GetFields()
					require.NoError(t, err)
					want := cloneSession(w.Fields)
					if w.Reply != nil {
						want["ACSE"].(map[string]any)["association_request_event"] = requestID
						require.Equal(t, requestID, e.ResponseTo)
					} else {
						require.Zero(t, e.ResponseTo)
					}
					rocEqualFields(t, want, fields)
					rocEqualFields(t, want, e.Session)
					require.Zero(t, e.TransactionID)
					if i == 1 {
						requestID = e.ID
					}
					fields["ACSE Segmentation"].(map[string]any)["assembled_information_hex"] = "mutated"
					e.Session["ACSE Segmentation"].(map[string]any)["information_hex"] = "mutated"
					if a, ok := e.Session["ACSE"].(map[string]any); ok {
						a["conformance_bits"] = 0
						a["max_pdu_size"] = 1
					}
					e.ID = 999999
					clear(e.Raw)
				}
				require.Less(t, s.Stats().BufferedBytes, int64(1<<20), "completed ACSE releases the large assembly reservation")
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
func TestDLMSACSESegmentedCloseBudgetIdle(t *testing.T) {
	for _, tr := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, n := range []int{1, 3} {
				t.Run(fmt.Sprintf("close/%s/%t/%d", tr, deferred, n), func(t *testing.T) {
					c := acseSegmentAccepted(t, tr)
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					var first *ProtocolEvent
					for i, w := range c.Events[:n] {
						o := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
						require.Nil(t, o.Err)
						require.Len(t, o.Events, 1)
						if i == 0 {
							first = o.Events[0]
						}
					}
					out := s.Close("incomplete")
					require.Len(t, out, 1)
					e := out[0]
					require.Equal(t, "dlms-hdlc-acse-segmented", e.Profile)
					require.Equal(t, first.Source, e.Source)
					require.Equal(t, first.Destination, e.Destination)
					require.Equal(t, first.FlowID, e.FlowID)
					require.Equal(t, tr, e.Transport)
					require.Equal(t, "incomplete", e.Status)
					require.Equal(t, "incomplete", e.Completeness)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					_, err = e.GetFields()
					rocTypedError(t, "NeedMore", err)
					rocEqualFields(t, map[string]any{"Outstanding": 1, "ObservedFrames": 1, "InformationBytes": 20}, e.Session)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
	c := acseSegmentAccepted(t, "udp")
	for _, collection := range []bool{false, true} {
		t.Run(fmt.Sprintf("refusal/collection-%t", collection), func(t *testing.T) {
			b := DefaultParserBudget()
			if collection {
				b.MaxCollectionElements = 19
			} else {
				b.MaxFrameBytes, b.MaxMessageBytes, b.MaxBufferedBytes = 2049, 2049, 8192
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("test")
			x := s.Feed(0, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
			require.Len(t, x.Events, 1)
			f, err := x.Events[0].GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, f)
			require.Zero(t, x.Events[0].ResponseTo)
			require.Zero(t, x.Events[0].TransactionID)
			a := s.(*captureSession).f.a
			a.budget.MaxBufferedBytes = 8 << 20
			a.budget.MaxCollectionElements = 4096
			for _, w := range c.Events {
				out := s.Feed(w.Direction, time.Unix(2, 0), wrapperWire(t, w.Raw))
				require.Len(t, out.Events, 1)
				require.Zero(t, out.Events[0].ResponseTo)
				require.Zero(t, out.Events[0].TransactionID)
			}
			require.Empty(t, s.Close("refused"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("idle", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		defer s.Close("test")
		first := s.Feed(0, time.Unix(1, 0), bytes.Clone(wrapperWire(t, c.Events[0].Raw)))
		require.Len(t, first.Events, 1)
		for _, w := range c.Events[1:] {
			out := s.Feed(w.Direction, time.Unix(32, 0), wrapperWire(t, w.Raw))
			require.Len(t, out.Events, 1)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Zero(t, out.Events[0].TransactionID)
		}
		require.Empty(t, s.Close("idle"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
