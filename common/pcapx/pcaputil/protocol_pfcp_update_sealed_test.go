package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

func pfcpUpdateControls(t *testing.T) []pfcpSetupControl {
	t.Helper()
	cs := pfcpReadControlsFrom(t, "pfcp-update/", "controls.json", 42)
	b, err := trafficfixture.ReadFile("pfcp-update/controls.json")
	require.NoError(t, err)
	var j struct {
		Cases []struct {
			Target struct {
				Retry []int `json:"retransmission_refs"`
			} `json:"session_target"`
		}
	}
	require.NoError(t, json.Unmarshal(b, &j))
	for i := range cs {
		cs[i].Target.Retry = j.Cases[i].Target.Retry
	}
	return cs
}
func pfcpUpdateCapture(t *testing.T, c pfcpSetupControl) []byte {
	t.Helper()
	b, err := trafficfixture.ReadFile("pfcp-update/" + c.File)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	return b
}
func TestPFCPUpdateSealedByteOracle(t *testing.T) {
	for _, c := range pfcpUpdateControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			n := 0
			for i, st := range c.Steps {
				w := doipDiscoveryWire(t, st.Hex)
				f, err := decodePFCPNode(w, 4096)
				at := pfcpSetupAnswer(c, i+1)
				if at < 0 {
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
func pfcpUpdatePairs(t *testing.T, c pfcpSetupControl, events []*ProtocolEvent) {
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
func TestPFCPUpdateSealedDatagramMatrix(t *testing.T) {
	for _, c := range pfcpUpdateControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := pfcpUpdateCapture(t, c)
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
				pfcpSetupCheckMessages(t, c, events, stats, d, true)
			})
		})
	}
}
func TestPFCPUpdateEndpointStateLifecycle(t *testing.T) {
	for _, c := range pfcpUpdateControls(t) {
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
					rocTypedError(t, c.Errors[i].Code, err)
					require.Nil(t, f)
					require.Zero(t, e.ResponseTo)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					clear(wire)
					clear(e.Fields)
					clear(e.Session)
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
				}
				if len(c.Target.Pending) > i {
					require.Equal(t, c.Target.Pending[i], pfcpPending(a))
				}
			}
			pfcpUpdatePairs(t, c, events)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, pfcpPending(a))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestPFCPUpdatePublicFeedProbeAndOwnership(t *testing.T) {
	for _, c := range pfcpUpdateControls(t) {
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
					rocEqualFields(t, c.Messages[at].Fields, f)
					clear(f)
					clear(r.Events[0].Raw)
					f, e = r.Events[0].GetFields()
					require.NoError(t, e)
					rocEqualFields(t, c.Messages[at].Fields, f)
				}
			}
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
			require.Equal(t, ErrFatalSessionError, s.Feed(0, time.Unix(200, 0), nil).Err.Kind)
		})
	}
}
func TestPFCPUpdateResourceRetirement(t *testing.T) {
	for _, c := range pfcpUpdateControls(t) {
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
					e := &ProtocolEvent{ID: uint64(i + 1), Direction: st.Dir, Profile: pfcpUpdateProfile, Session: map[string]any{}}
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
					rocEqualFields(t, c.Messages[at].Fields, f)
				}
			}
			pfcpUpdatePairs(t, c, events)
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
			if c.Budget.Message != 0 {
				raw := pfcpUpdateCapture(t, c)
				discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
					events, stats := discoveryReplay(t, raw, w, d, o, WithProtocolDecodeAs("udp", 8805, "pfcp"), WithBinParserConfig(BinParserConfig{MaxMessageBytes: c.Budget.Message}))
					require.Len(t, events, len(c.Steps))
					pfcpUpdatePairs(t, c, events)
					require.EqualValues(t, len(c.Steps[1].Hex)/2, stats.LimitedBytes)
					f, err := events[1].GetFields()
					require.Nil(t, f)
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, events[1].Raw)
					for _, i := range []int{0, 2, 3, 4} {
						f, err := events[i].GetFields()
						require.NoError(t, err)
						rocEqualFields(t, c.Messages[pfcpSetupAnswer(c, i+1)].Fields, f)
					}
				})
			}
		})
	}
}
func TestPFCPUpdateProjectionBudgets(t *testing.T) {
	c := pfcpUpdateControls(t)[0]
	wire := doipDiscoveryWire(t, c.Steps[0].Hex)
	need := int(pfcpSetupProjectionBytes) + 256*len(wire) + 4352 + len(wire)
	for _, tc := range []struct {
		name                   string
		elements, depth, bytes int
		accept                 bool
	}{
		{"collection-under", 29, 4, 1 << 20, false}, {"collection-exact", 30, 4, 1 << 20, true}, {"depth-under", 4096, 3, 1 << 20, false}, {"projection-under", 4096, 4, need - 1, false}, {"projection-exact", 4096, 4, need, true},
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
				rocEqualFields(t, c.Messages[0].Fields, f)
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
