package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type pfcpUsageControl struct {
	pfcpSetupControl
	Native   []string `json:"native_errors"`
	Children []struct {
		Value     string `json:"group_value_hex"`
		Qualified struct {
			Fields map[string]any
			Error  *struct{ Code, Detail string }
		} `json:"qualified_selected_child_answer"`
	} `json:"selected_child_answers"`
	Limits struct {
		Children int `json:"child_limit"`
		Bytes    int `json:"group_limit"`
	} `json:"helper_limits"`
}

func pfcpUsageControls(t *testing.T) []pfcpUsageControl {
	t.Helper()
	// Reuse the immutable input/answer/inventory SHA and packet-count binding.
	base := pfcpReadControlsFrom(t, "pfcp-usage/", "controls.json", 41)
	raw, err := trafficfixture.ReadFile("pfcp-usage/controls.json")
	require.NoError(t, err)
	var d struct{ Cases []pfcpUsageControl }
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&d))
	for i := range d.Cases {
		for j := range d.Cases[i].Steps {
			d.Cases[i].Steps[j].Source = base[i].Steps[j].Source
			d.Cases[i].Steps[j].Dest = base[i].Steps[j].Dest
		}
	}
	return d.Cases
}
func pfcpUsageCheck(t *testing.T, c pfcpUsageControl, events []*ProtocolEvent, captured bool) {
	t.Helper()
	require.Len(t, events, len(c.Steps))
	for i, e := range events {
		require.Equal(t, "pfcp", e.Protocol)
		require.Equal(t, discoveryProfile("pfcp", doipDiscoveryWire(t, c.Steps[i].Hex)), e.Profile)
		require.Equal(t, doipDiscoveryWire(t, c.Steps[i].Hex), e.Raw)
		if captured {
			require.Equal(t, fmt.Sprintf("%s:%d", c.Steps[i].Source, c.Steps[i].Sport), e.Source)
			require.Equal(t, fmt.Sprintf("%s:%d", c.Steps[i].Dest, c.Steps[i].Dport), e.Destination)
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
		}
		if i > 0 {
			require.Greater(t, e.ID, events[i-1].ID)
		}
		f, err := e.GetFields()
		if c.Native[i] != "" {
			rocTypedError(t, c.Native[i], err)
			require.Nil(t, f)
			require.Zero(t, e.ResponseTo)
			require.Empty(t, e.Session)
			require.Equal(t, "limited", e.Status)
			continue
		}
		require.NoError(t, err)
		// Exact session snapshots supplement static wire-field answers. These
		// states are independent observed-pairing expectations, not device truth.
		association, pending := "unmatched-response", 0
		if len(c.Steps) > 1 {
			states := map[string][]string{
				"usage-interleaved-sequences":   {"request", "request", "matched-response", "matched-response"},
				"usage-exact-request-retry":     {"request", "retransmitted-request", "matched-response"},
				"usage-id-reuse-quarantined":    {"request", "ambiguous-sequence", "ambiguous-sequence"},
				"usage-wrong-direction":         {"request", "unmatched-response", "matched-response"},
				"usage-refusal-retires-pending": {"request", "", "ambiguous-conversation"},
			}
			if expected, ok := states[c.Name]; ok {
				association = expected[i]
				pending = c.Target.Pending[i]
			} else {
				require.Len(t, c.Steps, 2)
				require.Equal(t, [][]int{{1, 2}}, c.Target.Pairs)
				if i == 0 {
					association, pending = "request", 1
				} else {
					association = "matched-response"
				}
			}
			require.NotZero(t, e.FlowID)
			require.Equal(t, events[0].FlowID, e.FlowID)
			require.Equal(t, c.Steps[i].Dir, e.Direction)
		}
		expectedSession := make(map[string]any, len(c.Messages[i].Fields)+2)
		for k, v := range c.Messages[i].Fields {
			expectedSession[k] = v
		}
		expectedSession["Association"] = association
		expectedSession["Outstanding"] = pending
		pfcpDeletionExactFields(t, expectedSession, e.Session)
		pfcpDeletionExactFields(t, c.Messages[i].Fields, f)
		require.Empty(t, e.Error)
		require.Equal(t, "message", e.Completeness)
		// Exercise ownership at every nested map/slice, not just the root.
		mutatePFCPUsageTree(f)
		mutatePFCPUsageTree(e.Session)
		clear(e.Fields)
		clear(e.Raw)
		f, err = e.GetFields()
		require.NoError(t, err)
		pfcpDeletionExactFields(t, c.Messages[i].Fields, f)
	}
	pfcpDeletionPairs(t, c.pfcpSetupControl, events)
	for _, r := range c.Target.Retry {
		require.Equal(t, events[0].ID, events[r-1].TransactionID)
	}
}
func mutatePFCPUsageTree(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			mutatePFCPUsageTree(e)
			x[k] = "caller mutation"
		}
	case []map[string]any:
		for _, e := range x {
			mutatePFCPUsageTree(e)
		}
	case []any:
		for i, e := range x {
			mutatePFCPUsageTree(e)
			x[i] = nil
		}
	}
}
func TestPFCPUsageSealedByteOracle(t *testing.T) {
	for _, c := range pfcpUsageControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			for i, st := range c.Steps {
				f, err := decodePFCPNode(doipDiscoveryWire(t, st.Hex), 4096)
				if c.Native[i] != "" {
					rocTypedError(t, c.Native[i], err)
					require.Nil(t, f)
				} else {
					require.NoError(t, err)
					pfcpDeletionExactFields(t, c.Messages[i].Fields, f)
				}
			}
			for _, child := range c.Children {
				cl, bl := 4096, 4096
				if c.Limits.Children > 0 {
					cl = c.Limits.Children
				}
				if c.Limits.Bytes > 0 {
					bl = c.Limits.Bytes
				}
				f, err := decodePFCPUsage(doipDiscoveryWire(t, child.Value), cl, bl)
				if child.Qualified.Error != nil {
					rocTypedError(t, child.Qualified.Error.Code, err)
					require.Nil(t, f)
					require.Equal(t, child.Qualified.Error.Detail, pfcpUsageDetail(err))
				} else {
					require.NoError(t, err)
					pfcpDeletionExactFields(t, child.Qualified.Fields, f)
				}
			}
		})
	}
}
func TestPFCPUsageSealedDatagramMatrix(t *testing.T) {
	for _, c := range pfcpUsageControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("pfcp-usage/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				events, stats := discoveryReplay(t, raw, w, d, o, WithProtocolDecodeAs("udp", 8805, "pfcp"))
				for i, e := range events {
					want := "decoded"
					if d {
						want = "deferred"
					}
					if c.Native[i] != "" {
						want = "limited"
					}
					require.Equal(t, want, e.Status)
				}
				pfcpUsageCheck(t, c, events, true)
				require.EqualValues(t, len(c.Steps), stats.Messages)
				n := 0
				for _, e := range c.Native {
					if e == "" {
						n++
					}
				}
				if d {
					require.EqualValues(t, n, stats.Deferred)
				} else {
					require.EqualValues(t, n, stats.Decoded)
				}
			})
		})
	}
}
func TestPFCPUsageSessionStateOwnership(t *testing.T) {
	for _, c := range pfcpUsageControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			s := discoverySession(t, DefaultParserBudget(), 8805)
			defer s.Close("cleanup")
			var events []*ProtocolEvent
			for i, st := range c.Steps {
				wire := doipDiscoveryWire(t, st.Hex)
				e := &ProtocolEvent{Source: fmt.Sprintf("%s:%d", st.Source, st.Sport), Destination: fmt.Sprintf("%s:%d", st.Dest, st.Dport), Timestamp: time.Unix(int64(100+i), 0)}
				require.True(t, s.f.a.decodeDiscoveryDatagram(e, wire, st.Sport, st.Dport, "pfcp"))
				clear(wire)
				events = append(events, e)
				if len(c.Target.Pending) > i {
					require.Equal(t, c.Target.Pending[i], pfcpPending(s.f.a))
				}
			}
			pfcpUsageCheck(t, c, events, false)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, pfcpPending(s.f.a))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	// The public UDP API must expose the same exact fields and correlation.
	for _, c := range pfcpUsageControls(t) {
		t.Run("public/"+c.Name, func(t *testing.T) {
			s := discoverySession(t, DefaultParserBudget(), 8805)
			defer s.Close("cleanup")
			var events []*ProtocolEvent
			for i, st := range c.Steps {
				wire := doipDiscoveryWire(t, st.Hex)
				r := s.Feed(st.Dir, time.Unix(int64(100+i), 0), wire)
				require.Len(t, r.Events, 1)
				if c.Native[i] == "" {
					require.Nil(t, r.Err)
				} else {
					require.NotNil(t, r.Err)
					rocTypedError(t, c.Native[i], r.Err)
				}
				clear(wire)
				events = append(events, r.Events...)
				if len(c.Target.Pending) > i {
					require.Equal(t, c.Target.Pending[i], pfcpPending(s.f.a))
				}
			}
			pfcpUsageCheck(t, c, events, false)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, pfcpPending(s.f.a))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	// Caller limits apply before projection and remain distinct from wire syntax.
	wire := doipDiscoveryWire(t, "2137002b99aabbccddeeff00123456000013000101004f001600510004010203040068000400000012003f00021000")
	for _, tc := range []struct {
		name            string
		elements, depth int
		want            string
	}{{"map-under", 34, 6, string(ErrResourceExceeded)}, {"map-exact", 35, 6, ""}, {"depth-under", 35, 5, string(ErrResourceExceeded)}, {"depth-exact", 35, 6, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxCollectionElements = tc.elements
			b.MaxRecursionDepth = tc.depth
			s := discoverySession(t, b, 8805)
			defer s.Close("cleanup")
			e := &ProtocolEvent{Source: "192.0.2.2:8805", Destination: "192.0.2.1:8805"}
			require.True(t, s.f.a.decodeDiscoveryDatagram(e, wire, 8805, 8805, "pfcp"))
			f, err := e.GetFields()
			if tc.want != "" {
				rocTypedError(t, tc.want, err)
				require.Nil(t, f)
			} else {
				require.NoError(t, err)
				require.NotNil(t, f)
			}
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	// An unsolicited response allocates no pending request/conversation.
	need := int(discoveryProjectionBytes("pfcp", wire))
	for _, cap := range []int{need - 1, need} {
		t.Run(fmt.Sprintf("projection/%d", cap), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes = 4096
			b.MaxFrameBytes = 4096
			b.MaxBufferedBytes = cap
			s := discoverySession(t, b, 8805)
			r := s.Feed(1, time.Unix(100, 0), wire)
			require.Len(t, r.Events, 1)
			f, err := r.Events[0].GetFields()
			if cap < need {
				require.NotNil(t, r.Err)
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
			} else {
				require.Nil(t, r.Err)
				require.NoError(t, err)
				require.NotNil(t, f)
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(cap))
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}

}
