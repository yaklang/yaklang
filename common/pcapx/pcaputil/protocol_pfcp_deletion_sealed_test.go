package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

func pfcpDeletionControls(t *testing.T) []pfcpSetupControl {
	t.Helper()
	cs := pfcpReadControlsFrom(t, "pfcp-deletion/", "controls.json", 39)
	b, err := trafficfixture.ReadFile("pfcp-deletion/controls.json")
	require.NoError(t, err)
	var j struct {
		Cases []struct {
			Messages []struct{ Fields map[string]any } `json:"expected_messages"`
			Target   struct {
				Retry []int `json:"retransmission_refs"`
			} `json:"session_target"`
			Native struct {
				Pairs   [][]int `json:"matched_pairs"`
				Pending []int   `json:"pending_after_steps"`
			} `json:"native_session_target"`
		}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&j))
	for i := range cs {
		cs[i].Target.Retry = j.Cases[i].Target.Retry
		for k, m := range j.Cases[i].Messages {
			cs[i].Messages[k].Fields = m.Fields
		}
		if cs[i].Name == "known-binding-wrong-response-seid" {
			cs[i].Target.Pairs = j.Cases[i].Native.Pairs
			cs[i].Target.Pending = j.Cases[i].Native.Pending
		}
	}
	return cs
}
func pfcpDeletionCapture(t *testing.T, c pfcpSetupControl) []byte {
	t.Helper()
	b, err := trafficfixture.ReadFile("pfcp-deletion/" + c.File)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	return b
}
func TestPFCPDeletionSealedByteOracle(t *testing.T) {
	for _, c := range pfcpDeletionControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			n := 0
			for i, st := range c.Steps {
				w := doipDiscoveryWire(t, st.Hex)
				f, err := decodePFCPNode(w, 4096)
				if c.Name == "udp-split-never-join" {
					f, err = decodePFCPDeletion(w, 4096)
				}
				at := pfcpSetupAnswer(c, i+1)
				if at < 0 {
					rocTypedError(t, c.Errors[i].Code, err)
					require.Nil(t, f)
				} else {
					require.NoError(t, err)
					pfcpDeletionExactFields(t, c.Messages[at].Fields, f)
					n++
				}
			}
			require.Equal(t, len(c.Messages), n)
		})
	}
}
func pfcpDeletionPairs(t *testing.T, c pfcpSetupControl, events []*ProtocolEvent) {
	t.Helper()
	got := map[int]int{}
	for i, e := range events {
		if e.ResponseTo == 0 {
			continue
		}
		for k, q := range events {
			if e.ResponseTo == q.ID {
				got[i+1] = k + 1
				require.Equal(t, e.ResponseTo, e.TransactionID)
			}
		}
	}
	require.Len(t, got, len(c.Target.Pairs))
	for _, p := range c.Target.Pairs {
		require.Equal(t, p[0], got[p[1]])
	}
}
func TestPFCPDeletionSealedDatagramMatrix(t *testing.T) {
	for _, c := range pfcpDeletionControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := pfcpDeletionCapture(t, c)
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				events, stats := discoveryReplay(t, raw, w, d, o, WithProtocolDecodeAs("udp", 8805, "pfcp"))
				for i, e := range events {
					require.Equal(t, fmt.Sprintf("%s:%d", c.Steps[i].Source, c.Steps[i].Sport), e.Source)
					require.Equal(t, fmt.Sprintf("%s:%d", c.Steps[i].Dest, c.Steps[i].Dport), e.Destination)
					if i > 0 {
						require.Greater(t, e.ID, events[i-1].ID)
					}
				}
				if c.Domains != nil {
					for i, e := range events {
						require.Equal(t, c.Domains[i], e.Domain.Interface)
					}
					require.NotEqual(t, events[0].Domain, events[1].Domain)
				}
				pfcpDeletionCheckMessages(t, c, events, stats, d, true)
				if c.Budget != nil {
					if c.Name == "heartbeat-public64-byte-budget-late-reply" {
						require.Zero(t, events[2].ResponseTo)
					} else {
						require.Equal(t, events[0].ID, events[2].ResponseTo)
					}
				}
			})
		})
	}
}
func TestPFCPDeletionEndpointStateLifecycle(t *testing.T) {
	for _, c := range pfcpDeletionControls(t) {
		if c.Budget != nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			s := discoverySession(t, DefaultParserBudget(), 8805)
			defer s.Close("cleanup")
			a := s.f.a
			var events []*ProtocolEvent
			for i, st := range c.Steps {
				e := &ProtocolEvent{Source: fmt.Sprintf("%s:%d", st.Source, st.Sport), Destination: fmt.Sprintf("%s:%d", st.Dest, st.Dport), Timestamp: time.Unix(int64(100+i), 0)}
				if c.Domains != nil {
					e.Domain.Interface = c.Domains[i]
				}
				wire := doipDiscoveryWire(t, st.Hex)
				require.True(t, a.decodeDiscoveryDatagram(e, wire, st.Sport, st.Dport, "pfcp"))
				events = append(events, e)
				f, err := e.GetFields()
				at := pfcpSetupAnswer(c, i+1)
				if at < 0 {
					expected := c.Errors[i]
					if c.NativeErrors != nil {
						expected = c.NativeErrors[i]
					}
					rocTypedError(t, expected.Code, err)
					require.Nil(t, f)
					require.Zero(t, e.ResponseTo)
				} else {
					require.NoError(t, err)
					pfcpDeletionExactFields(t, c.Messages[at].Fields, f)
					clear(wire)
					clear(e.Fields)
					clear(e.Session)
					f, err = e.GetFields()
					require.NoError(t, err)
					pfcpDeletionExactFields(t, c.Messages[at].Fields, f)
				}
				if len(c.Target.Pending) > i {
					require.Equal(t, c.Target.Pending[i], pfcpPending(a))
				}
			}
			pfcpDeletionPairs(t, c, events)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, pfcpPending(a))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestPFCPDeletionPublicFeedProbeAndOwnership(t *testing.T) {
	for _, c := range pfcpDeletionControls(t) {
		if c.Budget != nil || c.Domains != nil || c.Name == "version2" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(8805, 8805))
			require.NoError(t, err)
			defer s.Close("cleanup")
			for i, st := range c.Steps {
				wire := doipDiscoveryWire(t, st.Hex)
				admittedHeader := len(wire) >= 2 && wire[0]>>5 == 1
				at := pfcpSetupAnswer(c, i+1)
				probe := s.Probe(wire)
				if at >= 0 {
					require.Equal(t, ProbeAccept, probe.Verdict)
				} else {
					require.NotEqual(t, ProbeAccept, probe.Verdict)
				}
				r := s.Feed(st.Dir, time.Unix(int64(100+i), 0), wire)
				clear(wire)
				require.Len(t, r.Events, 1)
				f, e := r.Events[0].GetFields()
				if at < 0 {
					require.NotNil(t, r.Err)
					if admittedHeader {
						rocTypedError(t, c.Errors[i].Code, e)
					} else {
						// Feed supplies a port hint, not explicit DecodeAs. A fragment
						// without the version header cannot establish PFCP identity.
						require.NotEqual(t, "pfcp", r.Events[0].Protocol)
						require.Error(t, e)
						require.Zero(t, r.Events[0].ResponseTo)
						require.Zero(t, r.Events[0].TransactionID)
					}
					require.Nil(t, f)
				} else {
					require.Nil(t, r.Err)
					require.NoError(t, e)
					pfcpDeletionExactFields(t, c.Messages[at].Fields, f)
					clear(f)
					clear(r.Events[0].Raw)
					f, e = r.Events[0].GetFields()
					require.NoError(t, e)
					pfcpDeletionExactFields(t, c.Messages[at].Fields, f)
				}
			}
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
			require.Equal(t, ErrFatalSessionError, s.Feed(0, time.Unix(200, 0), nil).Err.Kind)
		})
	}
}
func TestPFCPDeletionResourceRetirement(t *testing.T) {
	for _, c := range pfcpDeletionControls(t) {
		if c.Budget == nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			if c.Budget.Pending != 0 {
				s := discoverySession(t, DefaultParserBudget(), 8805)
				defer s.Close("end")
				state := &binPFCP{}
				s.f.pfcp = state
				for i, st := range c.Steps {
					e := &ProtocolEvent{ID: uint64(i + 1), Direction: st.Dir, Profile: pfcpDeletionProfile, Session: map[string]any{}}
					err := state.associate(s.f, e, doipDiscoveryWire(t, st.Hex), 1)
					if i == 1 {
						rocTypedError(t, "ResourceExceeded", err)
						s.f.blockPFCPSetup()
						state = s.f.pfcp
						require.Zero(t, state.pendingCount())
						require.EqualValues(t, 512, s.f.sessionBytes)
					} else {
						require.NoError(t, err)
						if i == 2 {
							require.Zero(t, e.ResponseTo)
						}
					}
				}
				return
			}
			b := DefaultParserBudget()
			if c.Budget.Frame != 0 {
				b.MaxFrameBytes = c.Budget.Frame
				b.MaxMessageBytes = 64
			}
			if c.Budget.Message != 0 {
				b.MaxMessageBytes = c.Budget.Message
			}
			s := discoverySession(t, b, 8805)
			defer s.Close("cleanup")
			a := s.f.a
			var events []*ProtocolEvent
			for i, st := range c.Steps {
				e := &ProtocolEvent{Source: fmt.Sprintf("%s:%d", st.Source, st.Sport), Destination: fmt.Sprintf("%s:%d", st.Dest, st.Dport), Timestamp: time.Unix(int64(100+i), 0)}
				wire := doipDiscoveryWire(t, st.Hex)
				require.True(t, a.decodeDiscoveryDatagram(e, wire, st.Sport, st.Dport, "pfcp"))
				events = append(events, e)
				denied := false
				for _, r := range c.Budget.Refs {
					if r == i+1 {
						denied = true
					}
				}
				f, err := e.GetFields()
				if denied {
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
					require.Equal(t, "limited", e.Status)
					require.Nil(t, e.Raw)
					require.Zero(t, pfcpPending(a))
				} else {
					require.NoError(t, err)
					at := pfcpSetupAnswer(c, i+1)
					pfcpDeletionExactFields(t, c.Messages[at].Fields, f)
				}
			}
			pfcpDeletionPairs(t, c, events)
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
			if c.Budget.Message != 0 {
				raw := pfcpDeletionCapture(t, c)
				discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
					events, stats := discoveryReplay(t, raw, w, d, o, WithProtocolDecodeAs("udp", 8805, "pfcp"), WithBinParserConfig(BinParserConfig{MaxMessageBytes: c.Budget.Message}))
					require.Len(t, events, len(c.Steps))
					pfcpDeletionPairs(t, c, events)
					require.EqualValues(t, len(c.Steps[1].Hex)/2, stats.LimitedBytes)
					f, err := events[1].GetFields()
					require.Nil(t, f)
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, events[1].Raw)
					for i := range c.Steps {
						if i == 1 {
							continue
						}
						f, err := events[i].GetFields()
						require.NoError(t, err)
						pfcpDeletionExactFields(t, c.Messages[pfcpSetupAnswer(c, i+1)].Fields, f)
					}
				})
			}
		})
	}
}
func TestPFCPDeletionProjectionBudgets(t *testing.T) {
	c := pfcpDeletionControls(t)[0]
	wire := doipDiscoveryWire(t, c.Steps[0].Hex)
	need := int(pfcpSetupProjectionBytes) + 256*len(wire) + 4352 + len(wire)
	for _, tc := range []struct {
		name                   string
		elements, depth, bytes int
		accept                 bool
	}{
		{"collection-under", 33, 4, 1 << 20, false}, {"collection-exact", 34, 4, 1 << 20, true}, {"depth-under", 4096, 3, 1 << 20, false}, {"projection-under", 4096, 4, need - 1, false}, {"projection-exact", 4096, 4, need, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes = 4096
			b.MaxFrameBytes = 4096
			b.MaxBufferedBytes = tc.bytes
			b.MaxCollectionElements = tc.elements
			b.MaxRecursionDepth = tc.depth
			s := discoverySession(t, b, 8805)
			defer s.Close("cleanup")
			r := s.Feed(0, time.Unix(100, 0), wire)
			require.Len(t, r.Events, 1)
			f, err := r.Events[0].GetFields()
			if tc.accept {
				require.Nil(t, r.Err)
				require.NoError(t, err)
				pfcpDeletionExactFields(t, c.Messages[0].Fields, f)
				require.Equal(t, 1, pfcpPending(s.f.a))
			} else {
				require.NotNil(t, r.Err)
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Zero(t, pfcpPending(s.f.a))
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(tc.bytes))
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func pfcpDeletionExactFields(t *testing.T, want, got map[string]any) {
	t.Helper()
	normalize := func(v map[string]any) map[string]any {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		var n map[string]any
		require.NoError(t, d.Decode(&n))
		return n
	}
	require.Equal(t, normalize(want), normalize(got))
}

func TestPFCPDeletionIndependentReceiverBinding(t *testing.T) {
	var control pfcpSetupControl
	for _, c := range pfcpDeletionControls(t) {
		if c.Name == "known-binding-wrong-response-seid" {
			control = c
		}
	}
	require.NotEmpty(t, control.Name)
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprint(bound), func(t *testing.T) {
			opts := []ProtocolSessionOption{WithSessionTransport("udp")}
			if bound {
				opts = append(opts, WithSessionPFCPSessionIdentity(0x99aabbccddeeff00, 0x1122334455667788))
			}
			// Binding must use the final ports, independent of option order.
			opts = append(opts, WithSessionPorts(8805, 8805))
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
			require.NoError(t, err)
			defer s.Close("cleanup")
			var events []*ProtocolEvent
			for i, st := range control.Steps {
				res := s.Feed(st.Dir, time.Unix(int64(100+i), 0), doipDiscoveryWire(t, st.Hex))
				require.Nil(t, res.Err)
				require.Len(t, res.Events, 1)
				e := res.Events[0]
				events = append(events, e)
				fields, err := e.GetFields()
				require.NoError(t, err)
				pfcpDeletionExactFields(t, control.Messages[i].Fields, fields)
				require.Equal(t, false, fields["session_identity_verified"])
				if bound {
					require.Equal(t, i != 1, e.Session["Independent Receiver SEID Matches"])
					require.Equal(t, []int{1, 1, 0}[i], pfcpPending(s.(*captureSession).f.a))
				} else {
					require.NotContains(t, e.Session, "Independent Receiver SEID Matches")
				}
			}
			if bound {
				require.Zero(t, events[1].ResponseTo)
				require.Equal(t, events[0].ID, events[2].ResponseTo)
			} else {
				require.Equal(t, events[0].ID, events[1].ResponseTo)
				require.Zero(t, events[2].ResponseTo)
			}
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, opts := range [][]ProtocolSessionOption{
		{WithSessionPFCPSessionIdentity(0, 1)}, {WithSessionPFCPSessionIdentity(1, 0)},
		{WithSessionTransport("tcp"), WithSessionPFCPSessionIdentity(1, 2)},
		{WithSessionPFCPSessionIdentity(1, 2)},
	} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
		require.Nil(t, s)
	}
}
func pfcpDeletionCheckMessages(t *testing.T, c pfcpSetupControl, events []*ProtocolEvent, stats ProtocolStats, deferred, captured bool) {
	t.Helper()
	require.Len(t, events, len(c.Steps))
	require.EqualValues(t, len(events), stats.Messages)
	n := 0
	for i, e := range events {
		require.Equal(t, "pfcp", e.Protocol)
		wire := doipDiscoveryWire(t, c.Steps[i].Hex)
		profile := discoveryProfile("pfcp", wire)
		require.Equal(t, profile, e.Profile)
		require.Equal(t, doipDiscoveryWire(t, c.Steps[i].Hex), e.Raw)
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
		} else {
			require.Empty(t, e.SourceBytes.PacketRefs)
		}
		f, err := e.GetFields()
		at := pfcpSetupAnswer(c, i+1)
		if at < 0 {
			expected := c.Errors[i]
			if c.NativeErrors != nil {
				expected = c.NativeErrors[i]
			}
			rocTypedError(t, expected.Code, err)
			require.Nil(t, f)
			require.Empty(t, e.Session)
			require.Zero(t, e.ResponseTo)
			continue
		}
		require.NoError(t, err)
		pfcpDeletionExactFields(t, c.Messages[at].Fields, f)
		require.Empty(t, e.Error)
		require.Equal(t, "message", e.Completeness)
		n++
		f["kind"] = "caller change"
		if rows, ok := f["ies"].([]map[string]any); ok && len(rows) > 0 {
			rows[0]["value_hex"] = "changed"
		}
		e.Session["kind"] = "changed"
		if e.Fields != nil {
			delete(e.Fields, "node_id")
		}
		clear(e.Raw)
		f, err = e.GetFields()
		require.NoError(t, err)
		pfcpDeletionExactFields(t, c.Messages[at].Fields, f)
	}
	require.Equal(t, len(c.Messages), n)
	if deferred {
		require.EqualValues(t, n, stats.Deferred)
	} else {
		require.EqualValues(t, n, stats.Decoded)
	}
	if c.Target != nil && c.Budget == nil {
		got := map[int]int{}
		for i, e := range events {
			if e.ResponseTo != 0 {
				for q, r := range events {
					if r.ID == e.ResponseTo {
						got[i+1] = q + 1
						break
					}
				}
				require.Equal(t, e.ResponseTo, e.TransactionID)
			}
		}
		require.Len(t, got, len(c.Target.Pairs))
		for _, p := range c.Target.Pairs {
			require.Len(t, p, 2)
			require.Equal(t, p[0], got[p[1]])
		}
		for _, r := range c.Target.Unmatched {
			require.Zero(t, events[r-1].ResponseTo)
		}
		for _, r := range c.Target.Retry {
			require.Equal(t, events[0].ID, events[r-1].TransactionID)
		}
	}
}
