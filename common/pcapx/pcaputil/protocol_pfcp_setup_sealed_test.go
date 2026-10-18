package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type pfcpSetupControl struct {
	Name, File, SHA256 string
	Packets            int
	Steps              []struct {
		Dir               int
		Hex, Source, Dest string
		Sport, Dport      uint16
	}
	Messages []struct {
		Raw    string `json:"raw_hex"`
		Refs   []int  `json:"packet_refs"`
		Fields map[string]any
	} `json:"expected_messages"`
	Errors       []*struct{ Code, Detail string } `json:"static_packet_validation"`
	NativeErrors []*struct{ Code, Detail string } `json:"native_packet_validation"`
	Domains      []int                            `json:"capture_domains"`
	Target       *struct {
		Pairs     [][]int `json:"matched_pairs"`
		Unmatched []int   `json:"unmatched_response_refs"`
		Retry     []int   `json:"retransmission_packet_refs"`
		Pending   []int   `json:"pending_after_steps"`
	} `json:"session_target"`
	Budget *struct {
		Frame      int   `json:"max_message_bytes"`
		Message    int   `json:"public_message_bytes"`
		Pending    int   `json:"max_pending_requests"`
		Collection int   `json:"max_collection_elements"`
		Refs       []int `json:"resource_packet_refs"`
	} `json:"budget_target"`
}

func pfcpSetupControls(t *testing.T) []pfcpSetupControl {
	return pfcpReadControls(t, "controls.json", 85)
}
func pfcpSetupIsolationControls(t *testing.T) []pfcpSetupControl {
	return pfcpReadControls(t, "isolation-controls.json", 15)
}
func pfcpReadControls(t *testing.T, name string, count int) []pfcpSetupControl {
	return pfcpReadControlsFrom(t, "pfcp-setup/", name, count)
}
func pfcpReadControlsFrom(t *testing.T, prefix, name string, count int) []pfcpSetupControl {
	t.Helper()
	b, err := trafficfixture.ReadFile(prefix + name)
	require.NoError(t, err)
	var doc struct{ Cases []pfcpSetupControl }
	var raw struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(b, &doc))
	require.NoError(t, json.Unmarshal(b, &raw))
	require.Len(t, doc.Cases, count)
	// Source endpoints use explicit JSON names; keep observations distinct from role.
	var stepDoc struct {
		Cases []struct{ Steps []struct{ Src, Dst string } }
	}
	require.NoError(t, json.Unmarshal(b, &stepDoc))
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range doc.Cases {
		for n, s := range stepDoc.Cases[i].Steps {
			doc.Cases[i].Steps[n].Source = s.Src
			doc.Cases[i].Steps[n].Dest = s.Dst
		}
		path := "answers/" + c.Name + ".json"
		a, err := trafficfixture.ReadFile(prefix + path)
		require.NoError(t, err)
		require.JSONEq(t, string(raw.Cases[i]), string(a))
		bound := 0
		for _, inv := range inventories {
			for _, v := range inv.Cases {
				if v.Input.OriginalPath != "common/pcapx/pcaputil/"+prefix+c.File {
					continue
				}
				require.Equal(t, c.SHA256, v.Input.SHA256)
				for _, e := range v.Expectations {
					var bind struct {
						Answer  string `json:"answer_file"`
						SHA     string `json:"answer_sha256"`
						Packets int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &bind))
					if bind.Answer != path {
						continue
					}
					bound++
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(a)), bind.SHA)
					require.Equal(t, c.Packets, bind.Packets)
				}
			}
		}
		require.Equal(t, 1, bound, c.Name)
	}
	return doc.Cases
}
func pfcpSetupAnswer(c pfcpSetupControl, packet int) int {
	for i, m := range c.Messages {
		if len(m.Refs) == 1 && m.Refs[0] == packet {
			return i
		}
	}
	return -1
}
func pfcpSetupCapture(t *testing.T, c pfcpSetupControl) []byte {
	t.Helper()
	b, err := trafficfixture.ReadFile("pfcp-setup/" + c.File)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	return b
}
func TestPFCPSetupSealedByteOracle(t *testing.T) {
	for _, c := range pfcpSetupControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			n := 0
			require.Len(t, c.Errors, len(c.Steps))
			for i, st := range c.Steps {
				w := doipDiscoveryWire(t, st.Hex)
				f, err := decodePFCPSetup(w, 4096)
				at := pfcpSetupAnswer(c, i+1)
				if at < 0 {
					require.NotNil(t, c.Errors[i])
					rocTypedError(t, c.Errors[i].Code, err)
					require.Nil(t, f)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					n++
				}
			}
			require.Equal(t, len(c.Messages), n)
		})
	}
}
func pfcpSetupCheckMessages(t *testing.T, c pfcpSetupControl, events []*ProtocolEvent, stats ProtocolStats, deferred, captured bool) {
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
			require.Zero(t, e.TransactionID)
			continue
		}
		require.NoError(t, err)
		rocEqualFields(t, c.Messages[at].Fields, f)
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
		rocEqualFields(t, c.Messages[at].Fields, f)
	}
	require.Equal(t, len(c.Messages), n)
	if deferred {
		require.EqualValues(t, n, stats.Deferred)
	} else {
		require.EqualValues(t, n, stats.Decoded)
	}
	if c.Target != nil && c.Budget == nil {
		pfcpDeletionPairs(t, c, events)
		for _, r := range c.Target.Unmatched {
			require.Zero(t, events[r-1].ResponseTo)
		}
		for _, r := range c.Target.Retry {
			require.Equal(t, events[0].ID, events[r-1].TransactionID)
		}
	}
}
func TestPFCPSetupSealedDatagramMatrix(t *testing.T) {
	for _, c := range pfcpSetupControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := pfcpSetupCapture(t, c)
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				events, stats := discoveryReplay(t, raw, w, d, o, WithProtocolDecodeAs("udp", 8805, "pfcp"), WithProtocolDecodeAs("udp", 8806, "pfcp"))
				pfcpSetupCheckMessages(t, c, events, stats, d, true)
			})
		})
	}
}
func pfcpPending(a *binParser) int {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	n := 0
	if a.udpSessions != nil {
		for _, el := range a.udpSessions.entries {
			f := el.Value.(*binUDPEntry).flow
			if f.pfcp != nil {
				n += f.pfcp.pendingCount()
			}
		}
	}
	return n
}
func TestPFCPSetupPublicDatagramAndLifecycle(t *testing.T) {
	for _, c := range pfcpSetupControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(8805, 8805))
			require.NoError(t, err)
			n := 0
			for i, st := range c.Steps {
				w := doipDiscoveryWire(t, st.Hex)
				at := pfcpSetupAnswer(c, i+1)
				p := s.Probe(w)
				if at >= 0 {
					require.Equal(t, ProbeAccept, p.Verdict)
					require.Equal(t, "pfcp", p.Protocol)
				} else {
					require.NotEqual(t, ProbeAccept, p.Verdict)
				}
				r := s.Feed(st.Dir, time.Unix(int64(100+i), 0), w)
				clear(w)
				if len(st.Hex) >= 2 && st.Hex[:2] != "20" && st.Hex[:2] != "38" && st.Hex[:2] != "21" && st.Hex[:2] != "22" && st.Hex[:2] != "24" {
					for _, e := range r.Events {
						require.NotEqual(t, pfcpSetupProfile, e.Profile)
					}
					continue
				}
				require.Len(t, r.Events, 1)
				f, err := r.Events[0].GetFields()
				if at >= 0 {
					require.Nil(t, r.Err)
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					n++
				} else {
					require.NotNil(t, r.Err)
					expected := c.Errors[i]
					if c.NativeErrors != nil {
						expected = c.NativeErrors[i]
					}
					require.Equal(t, ProtocolErrorKind(expected.Code), r.Err.Kind)
					require.Nil(t, f)
				}
				// Endpoint/source-port variants require actual PCAP ingress, not Feed's
				// fixed endpoints. Their transaction targets are verified by the matrix.
				if c.Target != nil && c.Budget == nil && len(c.Target.Pending) > i && c.Name != "response-wrong-endpoint" && c.Name != "response-wrong-source-port" {
					require.Equal(t, c.Target.Pending[i], pfcpPending(s.(*captureSession).f.a))
				}
			}
			require.Equal(t, len(c.Messages), n)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Equal(t, ErrFatalSessionError, s.Feed(0, time.Unix(200, 0), nil).Err.Kind)
		})
	}
}
func TestPFCPSetupResourceTargets(t *testing.T) {
	for _, c := range append(pfcpSetupControls(t), pfcpSetupIsolationControls(t)...) {
		if c.Budget == nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			if c.Budget.Pending != 0 { // The archived target has no public configuration; exercise bounded state table directly below.
				session := discoverySession(t, DefaultParserBudget(), 8805)
				f := session.f
				defer session.Close("private bounded table")
				state := &binPFCP{}
				f.pfcp = state
				w := doipDiscoveryWire(t, c.Steps[0].Hex)
				e := &ProtocolEvent{ID: 1, Profile: pfcpSetupProfile, Session: map[string]any{}}
				require.NoError(t, state.associate(f, e, w, 1))
				w = doipDiscoveryWire(t, c.Steps[1].Hex)
				e = &ProtocolEvent{ID: 2, Profile: pfcpSetupProfile, Session: map[string]any{}}
				err := state.associate(f, e, w, 1)
				rocTypedError(t, "ResourceExceeded", err)
				require.Zero(t, state.pendingCount())
				require.True(t, state.blocked)
				return
			}
			b := DefaultParserBudget()
			if c.Budget.Frame != 0 {
				b.MaxMessageBytes = 64
				b.MaxFrameBytes = c.Budget.Frame
			}
			if c.Budget.Message != 0 {
				b.MaxMessageBytes = c.Budget.Message
			}
			if c.Budget.Collection != 0 {
				b.MaxCollectionElements = c.Budget.Collection
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(8805, 8805))
			require.NoError(t, err)
			for i, st := range c.Steps {
				r := s.Feed(st.Dir, time.Unix(int64(100+i), 0), doipDiscoveryWire(t, st.Hex))
				require.Len(t, r.Events, 1)
				denied := false
				for _, ref := range c.Budget.Refs {
					if ref == i+1 {
						denied = true
					}
				}
				if denied {
					require.NotNil(t, r.Err)
					require.Equal(t, ErrResourceExceeded, r.Err.Kind)
					require.Equal(t, "limited", r.Events[0].Status)
					if len(st.Hex)/2 > b.MaxFrameBytes || len(st.Hex)/2 > b.MaxMessageBytes {
						require.Nil(t, r.Events[0].Raw)
					}
					f, err := r.Events[0].GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
					require.Zero(t, pfcpPending(s.(*captureSession).f.a))
				} else {
					require.Nil(t, r.Err)
					f, err := r.Events[0].GetFields()
					require.NoError(t, err)
					at := pfcpSetupAnswer(c, i+1)
					rocEqualFields(t, c.Messages[at].Fields, f)
					if st.Dir == 1 {
						require.Zero(t, r.Events[0].ResponseTo)
						require.Zero(t, r.Events[0].TransactionID)
					}
				}
			}
			s.Close("budget")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestPFCPSetupSealedIsolationControls(t *testing.T) {
	for _, c := range pfcpSetupIsolationControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := pfcpSetupCapture(t, c)
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				events, stats := discoveryReplay(t, raw, w, d, o, WithProtocolDecodeAs("udp", 8805, "pfcp"))
				pfcpSetupCheckMessages(t, c, events, stats, d, true)
				if c.Domains != nil {
					require.Len(t, events, len(c.Domains))
					for i, e := range events {
						require.Equal(t, c.Domains[i], e.Domain.Interface)
					}
					require.Equal(t, events[0].Domain, events[2].Domain)
					require.NotEqual(t, events[0].Domain, events[1].Domain)
				}
			})
		})
	}
}
func TestPFCPNodeHeaderPublicIsolation(t *testing.T) {
	for _, c := range pfcpSetupIsolationControls(t) {
		if c.Budget != nil || c.Domains != nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			s := discoverySession(t, DefaultParserBudget(), 8805)
			defer s.Close("deferred cleanup")
			var events []*ProtocolEvent
			for i, st := range c.Steps {
				result := s.Feed(st.Dir, time.Unix(int64(100+i), 0), doipDiscoveryWire(t, st.Hex))
				require.Len(t, result.Events, 1)
				if c.Errors[i] != nil {
					require.NotNil(t, result.Err)
					require.Equal(t, ProtocolErrorKind(c.Errors[i].Code), result.Err.Kind)
				} else {
					require.Nil(t, result.Err)
				}
				events = append(events, result.Events...)
				if len(c.Target.Pending) > i {
					require.Equal(t, c.Target.Pending[i], pfcpPending(s.f.a))
				}
			}
			pfcpSetupCheckMessages(t, c, events, s.Stats(), false, false)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, pfcpPending(s.f.a))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestPFCPSetupProjectionBudgetBoundaries(t *testing.T) {
	controls := pfcpSetupControls(t)
	for _, spec := range []struct {
		name                   string
		elements, depth, bytes int
		denied                 bool
	}{
		{"cp-request-minimal", 24, 4, 1 << 20, true}, {"cp-request-minimal", 25, 4, 1 << 20, false},
		{"up-feature-known-r16", 41, 4, 1 << 20, true}, {"up-feature-known-r16", 42, 4, 1 << 20, false},
		{"cp-request-minimal", 4096, 3, 1 << 20, true}, {"cp-request-minimal", 4096, 4, 1 << 20, false},
		{"cp-request-minimal", 4096, 4, 32768, true},
	} {
		t.Run(fmt.Sprintf("%s/e%d/d%d/b%d", spec.name, spec.elements, spec.depth, spec.bytes), func(t *testing.T) {
			var c pfcpSetupControl
			for _, row := range controls {
				if row.Name == spec.name {
					c = row
					break
				}
			}
			require.NotEmpty(t, c.Name)
			budget := DefaultParserBudget()
			budget.MaxCollectionElements = spec.elements
			budget.MaxRecursionDepth = spec.depth
			budget.MaxBufferedBytes = spec.bytes
			budget.MaxMessageBytes = min(16384, spec.bytes)
			s := discoverySession(t, budget, 8805)
			defer s.Close("cleanup")
			result := s.Feed(0, time.Unix(100, 0), doipDiscoveryWire(t, c.Steps[0].Hex))
			require.Len(t, result.Events, 1)
			if spec.denied {
				require.NotNil(t, result.Err)
				require.Equal(t, ErrResourceExceeded, result.Err.Kind)
				f, err := result.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Zero(t, pfcpPending(s.f.a))
			} else {
				require.Nil(t, result.Err)
				f, err := result.Events[0].GetFields()
				require.NoError(t, err)
				rocEqualFields(t, c.Messages[0].Fields, f)
				require.Equal(t, 1, pfcpPending(s.f.a))
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(spec.bytes))
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
