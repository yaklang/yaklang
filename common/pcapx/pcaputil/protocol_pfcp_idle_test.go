package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// A passive capture cannot identify a new PFCP transaction generation from
// idle time alone. TS 29.244 16.12.1 section 6.4 matches the endpoint and sequence;
// a late response has the same identifying bytes as the reused request's reply.
func TestPFCPIdleHistoryExistingAPI(t *testing.T) {
	q := discoveryPayload(t, "pfcp-heartbeat-request")
	r := discoveryPayload(t, "pfcp-heartbeat-response")
	for _, state := range []string{"pending", "completed", "refused"} {
		t.Run(state, func(t *testing.T) {
			b := DefaultParserBudget()
			if state == "refused" {
				b.MaxFrameBytes = len(q)
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 8805))
			require.NoError(t, err)
			defer s.Close("cleanup")
			a := s.Feed(0, time.Unix(100, 0), q)
			require.Nil(t, a.Err)
			require.Len(t, a.Events, 1)
			if state == "completed" {
				good := s.Feed(1, time.Unix(101, 0), r)
				require.Nil(t, good.Err)
				require.Equal(t, a.Events[0].ID, good.Events[0].ResponseTo)
			}
			if state == "refused" {
				denied := s.Feed(0, time.Unix(101, 0), discoveryPayload(t, "pfcp-recovery-extension"))
				rocTypedError(t, "ResourceExceeded", denied.Err)
			}
			reused := s.Feed(0, time.Unix(132, 0), bytes.Clone(q))
			require.Nil(t, reused.Err)
			require.Len(t, reused.Events, 1)
			late := s.Feed(1, time.Unix(132, 0), r)
			require.Nil(t, late.Err)
			require.Len(t, late.Events, 1)
			require.Zero(t, late.Events[0].ResponseTo, "idle must not pair request %d with an old response", reused.Events[0].ID)
			require.Zero(t, late.Events[0].TransactionID)
			require.Empty(t, s.Close("idle"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("adjacent-live-retry", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 8805))
		require.NoError(t, err)
		defer s.Close("cleanup")
		a := s.Feed(0, time.Unix(100, 0), q)
		retry := s.Feed(0, time.Unix(129, 0), q)
		good := s.Feed(1, time.Unix(130, 0), r)
		require.Nil(t, a.Err)
		require.Nil(t, retry.Err)
		require.Nil(t, good.Err)
		require.Equal(t, a.Events[0].ID, retry.Events[0].TransactionID)
		require.Equal(t, "retransmitted-request", retry.Events[0].Session["Association"])
		require.Equal(t, a.Events[0].ID, good.Events[0].ResponseTo)
		require.Empty(t, s.Close("normal"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func testPFCPIdleHistoryMatrix(t *testing.T) {
	const folder = "pfcp-idle-history/"
	b, err := trafficfixture.ReadFile(folder + "controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			InputAlias                  string `json:"input_alias"`
			Packets                     int
			Limit                       int `json:"message_limit"`
			Events                      []struct {
				Raw, Profile, Error string
				Direction           int
				Timestamp           int64  `json:"timestamp_sec"`
				Reply               uint64 `json:"response_to"`
				Transaction         uint64 `json:"transaction_id"`
				Fields, Session     map[string]any
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber() // SEIDs and selected counters must retain every uint64 bit.
	require.NoError(t, decoder.Decode(&doc))
	require.Equal(t, "owned-pfcp-idle-history/v1", doc.Schema)
	require.Len(t, doc.Cases, 16)
	templates, err := trafficfixture.ReadFile(folder + "templates.json")
	require.NoError(t, err)
	var sources []struct {
		Alias string `json:"historical_alias"`
		SHA   string `json:"historical_sha256"`
	}
	require.NoError(t, json.Unmarshal(templates, &sources))
	require.Len(t, sources, 4)
	for _, source := range sources {
		original, err := trafficfixture.ReadFile("../../../" + source.Alias)
		require.NoError(t, err)
		require.Equal(t, source.SHA, fmt.Sprintf("%x", sha256.Sum256(original)))
	}
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		t.Run(c.ID, func(t *testing.T) {
			answer, err := trafficfixture.ReadFile(folder + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var parsed struct{ Events any }
			require.NoError(t, json.Unmarshal(answer, &parsed))
			// Re-marshal the original rows, preserving JSON tag-independent maps.
			var rows struct {
				Cases []struct {
					ID     string
					Events any
				}
			}
			require.NoError(t, json.Unmarshal(b, &rows))
			for _, row := range rows.Cases {
				if row.ID == c.ID {
					require.Equal(t, row.Events, parsed.Events)
				}
			}
			bound := 0
			for _, inv := range inventories {
				for _, row := range inv.Cases {
					if row.ID != "pfcp-idle-history/"+c.ID {
						continue
					}
					bound++
					require.Equal(t, c.SHA256, row.Input.SHA256)
					require.Equal(t, c.InputAlias, row.Input.OriginalPath)
					require.Equal(t, c.Packets, row.Facts.PacketCount)
					for _, e := range row.Expectations {
						var link struct {
							Answer string `json:"answer_file"`
							SHA    string `json:"answer_sha256"`
						}
						require.NoError(t, json.Unmarshal(e.PayloadConstraints, &link))
						require.Equal(t, c.Answer, link.Answer)
						require.Equal(t, c.AnswerSHA, link.SHA)
					}
				}
			}
			require.Equal(t, 1, bound)
			raw, err := trafficfixture.ReadFile("../../../" + c.InputAlias)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var opts []CaptureOption
				if c.Limit != 0 {
					opts = append(opts, WithProtocolBudget(c.Limit, 1<<20))
				}
				es, stats := discoveryReplay(t, raw, workers, deferred, observe, opts...)
				require.Len(t, es, len(c.Events))
				require.EqualValues(t, len(es), stats.Messages)
				flows := make(map[string]uint64)
				for i, e := range es {
					w := c.Events[i]
					require.Equal(t, uint64(i+1), e.ID)
					require.Equal(t, "pfcp", e.Protocol)
					require.Equal(t, w.Profile, e.Profile)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, time.Unix(w.Timestamp, 0), e.Timestamp)
					src, dst := "192.0.2.1:40000", "192.0.2.2:8805"
					if w.Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					require.Equal(t, w.Reply, e.ResponseTo)
					require.Equal(t, w.Transaction, e.TransactionID)
					require.Equal(t, es[0].Domain, e.Domain)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.Equal(t, uint64(i+1), e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					f, err := e.GetFields()
					if w.Error != "" {
						rocTypedError(t, w.Error, err)
						require.Nil(t, f)
						require.Nil(t, e.Session)
						require.Empty(t, e.Raw)
						require.Equal(t, "limited", e.Status)
						continue
					}
					require.NoError(t, err)
					require.Empty(t, e.Error)
					require.Equal(t, "message", e.Completeness)
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, len(w.Raw)/2, e.Length)
					require.NotZero(t, e.FlowID)
					key := w.Profile
					if key != "pfcp-v1-heartbeat" {
						key = "node"
					}
					if flows[key] == 0 {
						flows[key] = e.FlowID
					}
					require.Equal(t, flows[key], e.FlowID)
					pfcpDeletionExactFields(t, w.Fields, f)
					pfcpDeletionExactFields(t, w.Session, e.Session)
					mutatePFCPUsageTree(f)
					mutatePFCPUsageTree(e.Session)
					clear(e.Raw)
					clear(e.Fields)
					f, err = e.GetFields()
					require.NoError(t, err)
					pfcpDeletionExactFields(t, w.Fields, f)
				}
				require.Zero(t, stats.BufferedBytes)
				require.LessOrEqual(t, stats.PeakBufferedBytes, int64(1<<20))
			})
		})
	}
}

func TestPFCPUntrackedRefusalExistingAPI(t *testing.T) {
	q := discoveryPayload(t, "pfcp-heartbeat-request")
	r := discoveryPayload(t, "pfcp-heartbeat-response")
	b := DefaultParserBudget()
	b.MaxFrameBytes = len(q)
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 8805))
	require.NoError(t, err)
	defer s.Close("cleanup")
	denied := s.Feed(0, time.Unix(100, 0), discoveryPayload(t, "pfcp-recovery-extension"))
	rocTypedError(t, "ResourceExceeded", denied.Err)
	require.Len(t, denied.Events, 1)
	require.Zero(t, denied.Events[0].FlowID)
	request := s.Feed(0, time.Unix(101, 0), q)
	require.Nil(t, request.Err)
	late := s.Feed(1, time.Unix(102, 0), r)
	require.Nil(t, late.Err)
	require.Zero(t, late.Events[0].ResponseTo, "an unretained rejected identity cannot become a verified future transaction")
	require.Zero(t, late.Events[0].TransactionID)
	require.Empty(t, s.Close("refused"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestPFCPTransientRefusalBytesExistingAPI(t *testing.T) {
	q := discoveryPayload(t, "pfcp-heartbeat-request")
	b := DefaultParserBudget()
	b.MaxMessageBytes, b.MaxFrameBytes, b.ProbeBytes = 64, 64, 16
	b.MaxBufferedBytes = 2559 // 512 +128*16 =2560 transient bytes, one byte short.
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 8805))
	require.NoError(t, err)
	defer s.Close("cleanup")
	out := s.Feed(0, time.Unix(100, 0), q)
	rocTypedError(t, "ResourceExceeded", out.Err)
	require.Len(t, out.Events, 1)
	require.Nil(t, out.Events[0].Session)
	require.Empty(t, out.Events[0].Raw)
	require.EqualValues(t, len(q), s.Stats().LimitedBytes, "discarding unreserved wire must not hide refused byte accounting")
	require.Zero(t, s.Stats().BufferedBytes)
	require.Empty(t, s.Close("limited"))
	require.Zero(t, s.Stats().BufferedBytes)
	b.MaxBufferedBytes = 6928 // 2560 transient +4368 retained request, exact.
	s, err = NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 8805))
	require.NoError(t, err)
	defer s.Close("cleanup-exact")
	good := s.Feed(0, time.Unix(100, 0), q)
	require.Nil(t, good.Err)
	require.Len(t, good.Events, 1)
	require.Equal(t, q, good.Events[0].Raw)
	require.Equal(t, "request", good.Events[0].Session["Association"])
	require.Equal(t, good.Events[0].ID, good.Events[0].TransactionID)
	f, err := good.Events[0].GetFields()
	require.NoError(t, err)
	for _, c := range discoveryControls(t, "controls.json") {
		if c.Name == "pfcp-heartbeat-request" {
			rocEqualFields(t, c.ExpectedFields, f)
		}
	}
	require.Zero(t, s.Stats().LimitedBytes)
	require.EqualValues(t, 6928, s.Stats().PeakBufferedBytes)
	require.Empty(t, s.Close("exact"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestPFCPRefusalProfileSwitchExistingAPI(t *testing.T) {
	q := discoveryPayload(t, "pfcp-heartbeat-request")
	deniedWire := append(bytes.Clone(q), make([]byte, 24)...)
	deniedWire[3], deniedWire[11] = 36, 28
	var pair []string
	for _, c := range pfcpSetupControls(t) {
		if c.Name == "cp-up-pair" {
			for _, st := range c.Steps {
				pair = append(pair, st.Hex)
			}
		}
	}
	require.Len(t, pair, 2)
	b := DefaultParserBudget()
	b.MaxFrameBytes = 35
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 8805))
	require.NoError(t, err)
	defer s.Close("cleanup")
	require.Nil(t, s.Feed(0, time.Unix(100, 0), q).Err)
	denied := s.Feed(0, time.Unix(101, 0), deniedWire)
	rocTypedError(t, "ResourceExceeded", denied.Err)
	request := s.Feed(0, time.Unix(132, 0), doipDiscoveryWire(t, pair[0]))
	require.Nil(t, request.Err)
	late := s.Feed(1, time.Unix(133, 0), doipDiscoveryWire(t, pair[1]))
	require.Nil(t, late.Err)
	require.Zero(t, late.Events[0].ResponseTo, "switching node profiles cannot erase an endpoint refusal")
	require.Zero(t, late.Events[0].TransactionID)
	require.Empty(t, s.Close("profile-switch"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func testPFCPIdleStorageAndScope(t *testing.T) {
	q, r := discoveryPayload(t, "pfcp-heartbeat-request"), discoveryPayload(t, "pfcp-heartbeat-response")
	d0 := CaptureDomain{Section: 1, Interface: 0, Encapsulation: "vpn-A"}
	domains := []CaptureDomain{d0, {Section: 2, Interface: 0, Encapsulation: "vpn-A"}, {Section: 1, Interface: 1, Encapsulation: "vpn-A"}, {Section: 1, Interface: 0, Encapsulation: "vpn-B"}}
	s := discoverySession(t, DefaultParserBudget(), 8805)
	defer s.Close("cleanup")
	a := s.f.a
	feed := func(src, dst string, d CaptureDomain, at int64, wire []byte) *ProtocolEvent {
		e := &ProtocolEvent{Source: src, Destination: dst, Domain: d, Timestamp: time.Unix(at, 0), Transport: "udp"}
		borrowed := bytes.Clone(wire)
		require.True(t, a.decodeDiscoveryDatagram(e, borrowed, 8805, 8805, "pfcp"))
		clear(borrowed)
		require.Nil(t, e.sessionError)
		return e
	}
	first := feed("a:40000", "b:8805", d0, 100, q)
	late := feed("b:8805", "a:40000", d0, 130, r)
	require.Equal(t, "ambiguous-sequence", late.Session["Association"])
	require.Zero(t, late.ResponseTo)
	require.EqualValues(t, 4352, s.Stats().BufferedBytes)
	for _, el := range a.udpSessions.entries {
		f := el.Value.(*binUDPEntry).flow
		require.EqualValues(t, 4352, f.sessionBytes)
		require.Len(t, f.pfcp.seen, 1)
		for _, p := range f.pfcp.seen {
			require.Nil(t, p.wire)
			require.False(t, p.pending)
			require.True(t, p.ambiguous)
		}
	}
	// Section, interface and encapsulation are independent capture scopes. A
	// refusal on one peer never clears or blocks a retained unrelated peer.
	for _, d := range domains[1:] {
		req := feed("a:40000", "b:8805", d, 131, q)
		got := feed("b:8805", "a:40000", d, 132, r)
		require.NotEqual(t, first.FlowID, req.FlowID)
		require.Equal(t, req.FlowID, got.FlowID)
		require.Equal(t, req.ID, got.ResponseTo)
		require.Equal(t, req.ID, got.TransactionID)
	}
	peer := feed("c:40000", "d:8805", d0, 133, q)
	// No identity slot is allocated for the rejected peer. Existing peers remain
	// usable, while every newly admitted key is explicitly unverified.
	a.config.MaxMessageBytes = len(q)
	denied := &ProtocolEvent{Source: "x:40000", Destination: "y:8805", Domain: d0, Timestamp: time.Unix(134, 0)}
	require.True(t, a.decodeDiscoveryDatagram(denied, discoveryPayload(t, "pfcp-recovery-extension"), 40000, 8805, "pfcp"))
	rocTypedError(t, "ResourceExceeded", denied.sessionError)
	require.True(t, a.pfcpUntrackedRefusal)
	good := feed("d:8805", "c:40000", d0, 135, r)
	require.Equal(t, peer.ID, good.ResponseTo)
	fresh := feed("x:40000", "y:8805", d0, 136, q)
	require.Equal(t, "unverified-after-refusal", fresh.Session["Association"])
	require.Zero(t, fresh.TransactionID)
	got := feed("y:8805", "x:40000", d0, 137, r)
	require.Zero(t, got.ResponseTo)
	require.Equal(t, "unverified-after-refusal", got.Session["Association"])
	colliding := feed("a:40000", "e:8805", d0, 138, q)
	require.Equal(t, "unverified-after-refusal", colliding.Session["Association"])
	for _, el := range a.udpSessions.entries {
		f := el.Value.(*binUDPEntry).flow
		if f.id == colliding.FlowID || f.id == fresh.FlowID {
			require.Empty(t, f.pfcp.seen, "terminal markers never reacquire request bodies on peer collisions")
			require.EqualValues(t, 512, f.sessionBytes)
		}
	}
	require.Empty(t, s.Close("scope"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, a.udpSessions)
	// Retired sequence markers count against the collection limit. Expiry cannot
	// evade the bound by repeatedly emptying and recreating the same history.
	s = discoverySession(t, DefaultParserBudget(), 8805)
	defer s.Close("cleanup-bounded")
	a = s.f.a
	a.budget.MaxCollectionElements = 2
	feed("a:40000", "b:8805", d0, 200, q)
	q2 := bytes.Clone(q)
	q2[6]++
	feed("a:40000", "b:8805", d0, 230, q2)
	a.expirePFCP(time.Unix(260, 0))
	require.EqualValues(t, 4608, s.Stats().BufferedBytes)
	q3 := bytes.Clone(q)
	q3[6] += 2
	e := &ProtocolEvent{Source: "a:40000", Destination: "b:8805", Domain: d0, Timestamp: time.Unix(260, 0)}
	require.True(t, a.decodeDiscoveryDatagram(e, q3, 40000, 8805, "pfcp"))
	rocTypedError(t, "ResourceExceeded", e.sessionError)
	require.Nil(t, e.Session)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.EqualValues(t, 512, s.Stats().BufferedBytes)
	require.Empty(t, s.Close("bounded"))
	require.Zero(t, s.Stats().BufferedBytes)
}
