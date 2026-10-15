package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type finsAnswer struct {
	Raw, Error string
	Direction  int
	Fields     map[string]any
	Reply      *int
	PacketRefs []int
}
type finsControl struct {
	ID, Transport, Capture, Answer, SHA256, AnswerSHA string
	Packets                                           int
	Events                                            []finsAnswer
	Close                                             []struct{ Direction, Count int }
}

func finsRead(t *testing.T, n string) []byte {
	return finsReadBatch(t, "fins-selected", "FINS_TEST_ROOT", n)
}
func finsReadBatch(t *testing.T, folder, env, n string) []byte {
	t.Helper()
	var raw []byte
	var err error
	if root := os.Getenv(env); root != "" {
		raw, err = os.ReadFile(filepath.Join(root, folder, n))
	} else {
		raw, err = trafficfixture.ReadFile(folder + "/" + n)
	}
	require.NoError(t, err)
	return raw
}
func finsControls(t *testing.T) []finsControl {
	return finsControlBatch(t, "fins-selected", "FINS_TEST_ROOT", "owned-fins-selected/v1", 81)
}
func finsControlBatch(t *testing.T, folder, env, schema string, count int) []finsControl {
	t.Helper()
	var d struct {
		Schema string
		Cases  []finsControl
	}
	raw := finsReadBatch(t, folder, env, "controls.json")
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Equal(t, schema, d.Schema)
	require.Len(t, d.Cases, count)
	if os.Getenv(env) == "" {
		all, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		controlSHA := fmt.Sprintf("%x", sha256.Sum256(raw))
		for _, c := range d.Cases {
			bound := 0
			for _, batch := range all {
				for _, input := range batch.Cases {
					if input.ID != folder+"/"+c.ID {
						continue
					}
					require.Equal(t, c.SHA256, input.Input.SHA256)
					require.Equal(t, "common/pcapx/pcaputil/"+folder+"/"+c.Capture, input.Input.OriginalPath)
					require.Equal(t, c.Packets, input.Facts.PacketCount)
					for _, expectation := range input.Expectations {
						var link struct {
							File string `json:"answer_file"`
							SHA  string `json:"answer_sha256"`
						}
						require.NoError(t, json.Unmarshal(expectation.PayloadConstraints, &link))
						if link.File == "controls.json" {
							require.Equal(t, controlSHA, link.SHA)
							bound++
						}
					}
				}
			}
			require.Equal(t, 1, bound, c.ID)
		}
	}
	for _, c := range d.Cases {
		b := finsReadBatch(t, folder, env, c.Answer)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
		var a finsControl
		require.NoError(t, json.Unmarshal(b, &a))
		require.Equal(t, c.Events, a.Events)
		require.Equal(t, c.Close, a.Close)
	}
	return d.Cases
}
func finsResolve(v any, events []*ProtocolEvent) any {
	switch v := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(v))
		for k, x := range v {
			m[k] = finsResolve(x, events)
		}
		return m
	case []any:
		a := make([]any, len(v))
		for i, x := range v {
			a[i] = finsResolve(x, events)
		}
		return a
	case string:
		if strings.HasPrefix(v, "$event:") {
			var i int
			fmt.Sscanf(v, "$event:%d", &i)
			return events[i].ID
		}
		return v
	default:
		return v
	}
}
func finsAssert(t *testing.T, c finsControl, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, events, len(c.Events)+len(c.Close))
	ids := map[uint64]bool{}
	for i, w := range c.Events {
		e := events[i]
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		require.Equal(t, "fins", e.Protocol)
		require.Equal(t, finsProfile, e.Profile)
		require.Equal(t, c.Transport, e.Transport)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, len(w.Raw)/2, e.Length)
		if w.Reply == nil {
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
			require.Equal(t, events[*w.Reply].FlowID, e.FlowID)
		}
		f, err := e.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error != "" {
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.Zero(t, e.TransactionID)
			assertOwnedProtocolRefusal(t, e, w.Error, err)
			continue
		}
		want := finsResolve(w.Fields, events).(map[string]any)
		require.Empty(t, e.Error)
		rocEqualFields(t, want, f)
		rocEqualFields(t, want, e.Session)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		expectedID := e.ResponseTo
		inner := want
		if child, ok := want["fins"].(map[string]any); ok {
			inner = child
		}
		if expectedID == 0 {
			expectedID, _ = inner["observed_request_id"].(uint64)
		}
		require.Equal(t, expectedID, e.TransactionID)
		originalSession := cloneSession(e.Session)
		clear(f)
		clear(e.Session)
		e.Fields = map[string]any{"fake": true}
		e.Structured = map[string]any{"fields": e.Fields}
		e.Raw = bytes.Repeat([]byte{0xff}, len(e.Raw))
		e.Protocol = "caller-mutated"
		f, err = e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want, f)
		v, err := e.Decode()
		require.NoError(t, err)
		rocEqualFields(t, want, v["session"].(map[string]any))
		e.Protocol = "fins"
		e.Session = originalSession
		e.Raw = wrapperWire(t, w.Raw)
	}
	for i, w := range c.Close {
		e := events[len(c.Events)+i]
		require.Equal(t, "fins", e.Protocol)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, "incomplete", e.Status)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		_, err := e.GetFields()
		rocTypedError(t, "NeedMore", err)
		rocEqualFields(t, map[string]any{"Outstanding": w.Count}, e.Session)
	}
}
func TestFINSSealedMatrix(t *testing.T) {
	finsSealedMatrix(t, "fins-selected", "FINS_TEST_ROOT", finsControls(t))
}
func finsSealedMatrix(t *testing.T, folder, env string, controls []finsControl) {
	for _, c := range controls {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, folder, env, c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var extra []CaptureOption
				if c.Transport == "udp" {
					extra = append(extra, WithProtocolDecodeAs("udp", 9600, "fins"))
				}
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, extra...)
				expected := c
				if c.Transport == "udp" && c.Events[0].Direction == 1 {
					expected.Events = append([]finsAnswer(nil), c.Events...)
					for i := range expected.Events {
						expected.Events[i].Direction = 1 - expected.Events[i].Direction
					}
				}
				finsAssert(t, expected, events, deferred)
				require.Zero(t, stats.BufferedBytes)
				for i, w := range c.Events {
					e := events[i]
					src, dst := "192.0.2.1:40000", "192.0.2.2:9600"
					if w.Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					require.Len(t, e.SourceBytes.PacketRefs, len(w.PacketRefs))
					for j, ref := range e.SourceBytes.PacketRefs {
						require.EqualValues(t, w.PacketRefs[j], ref.Number)
						require.Equal(t, e.Domain, ref.Domain)
					}
				}
			})
		})
	}
}
func TestFINSSealedSessionChunks(t *testing.T) {
	finsSealedChunks(t, finsControls(t))
}
func finsSealedChunks(t *testing.T, controls []finsControl) {
	for _, c := range controls {
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{1, 7, 64, 4096} {
				if c.Transport == "udp" && chunk != 4096 {
					continue
				}
				t.Run(fmt.Sprintf("%s/%t/%d", c.ID, deferred, chunk), func(t *testing.T) {
					opts := []ProtocolSessionOption{WithSessionTransport(c.Transport), WithSessionPorts(40000, 9600)}
					if c.Transport == "udp" {
						opts = append(opts, WithSessionFINS(9600))
					}
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for _, w := range c.Events {
						wire := wrapperWire(t, w.Raw)
						for at := 0; at < len(wire); at += chunk {
							in := bytes.Clone(wire[at:min(len(wire), at+chunk)])
							out := s.Feed(w.Direction, time.Unix(1, 0), in)
							clear(in)
							events = append(events, out.Events...)
						}
					}
					events = append(events, s.Close("end")...)
					finsAssert(t, c, events, deferred)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

func finsSession(t *testing.T, transport string, b ParserBudget) ProtocolSession {
	t.Helper()
	opts := []ProtocolSessionOption{WithSessionTransport(transport), WithSessionPorts(40000, 9600)}
	if transport == "udp" {
		opts = append(opts, WithSessionFINS(9600))
	}
	s, err := NewProtocolSessionWithOptions(b, opts...)
	require.NoError(t, err)
	return s
}
func finsCase(t *testing.T, id string) finsControl {
	t.Helper()
	for _, c := range finsControls(t) {
		if c.ID == id {
			return c
		}
	}
	t.Fatal("missing control", id)
	return finsControl{}
}
func TestFINSNodeRenegotiationRetiresOldRequest(t *testing.T) {
	c := finsCase(t, "tcp-node-cpu-notifications-tcp")
	s := finsSession(t, "tcp", DefaultParserBudget())
	defer s.Close("end")
	feed := func(i int) *ProtocolEvent {
		w := c.Events[i]
		o := s.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
		require.Len(t, o.Events, 1)
		return o.Events[0]
	}
	feed(0)
	feed(1)
	feed(2)
	feed(0)
	e := feed(3)
	require.Zero(t, e.ResponseTo, "a repeated node-address exchange must retire earlier request eligibility")
	f, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, "unverified-response", f["fins"].(map[string]any)["association_status"])
}

func TestFINSUDPIdleReuseAndOwnership(t *testing.T) {
	c := finsCase(t, "cpu-option0-udp")
	s := finsSession(t, "udp", DefaultParserBudget())
	cs := s.(*captureSession)
	q := wrapperWire(t, c.Events[0].Raw)
	r := wrapperWire(t, c.Events[1].Raw)
	out := s.Feed(1, time.Unix(1, 0), q)
	require.Len(t, out.Events, 1)
	first := out.Events[0].ID
	before := s.Stats().BufferedBytes
	clear(q)
	clear(out.Events[0].Session)
	out.Events[0].Raw[0] = 0
	out = s.Feed(0, time.Unix(32, 0), r)
	require.Len(t, out.Events, 1)
	require.Zero(t, out.Events[0].ResponseTo)
	f, err := out.Events[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, "unverified-response", f["association_status"])
	require.Less(t, s.Stats().BufferedBytes, before)
	out = s.Feed(1, time.Unix(33, 0), wrapperWire(t, c.Events[0].Raw))
	require.Len(t, out.Events, 1)
	require.Zero(t, out.Events[0].TransactionID)
	f, err = out.Events[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, "unverified-sid-reuse", f["association_status"])
	out = s.Feed(0, time.Unix(34, 0), r)
	require.Len(t, out.Events, 1)
	require.Zero(t, out.Events[0].ResponseTo)
	require.NotEqual(t, first, out.Events[0].ID)
	require.Empty(t, s.Close("end"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, cs.f.a.udpSessions)
}
func TestFINSUDPRefusalAndDatagramBoundaries(t *testing.T) {
	c := finsCase(t, "cpu-option0-udp")
	q, r := wrapperWire(t, c.Events[0].Raw), wrapperWire(t, c.Events[1].Raw)
	for _, mode := range []string{"split", "coalesced", "truncated", "bytes", "depth", "elements", "pool"} {
		t.Run(mode, func(t *testing.T) {
			b := DefaultParserBudget()
			s := finsSession(t, "udp", b)
			cs := s.(*captureSession)
			if mode == "pool" {
				cs.f.a.config.MaxBufferedBytes = 1
				out := s.Feed(0, time.Unix(1, 0), q)
				require.Len(t, out.Events, 1)
				_, err := out.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.True(t, cs.f.a.finsUntrackedRefusal)
				cs.f.a.config.MaxBufferedBytes = b.MaxBufferedBytes
			} else {
				out := s.Feed(0, time.Unix(1, 0), q)
				require.Len(t, out.Events, 1)
				switch mode {
				case "split":
					out = s.Feed(0, time.Unix(2, 0), q[:7])
					require.Len(t, out.Events, 1)
					_, err := out.Events[0].GetFields()
					rocTypedError(t, "MalformedMessage", err)
					out = s.Feed(0, time.Unix(3, 0), q[7:])
					require.Len(t, out.Events, 1)
					require.NotEmpty(t, out.Events[0].Error)
				case "coalesced":
					out = s.Feed(0, time.Unix(2, 0), append(bytes.Clone(q), q...))
					require.Len(t, out.Events, 1)
					_, err := out.Events[0].GetFields()
					rocTypedError(t, "MalformedMessage", err)
				case "truncated":
					e := &ProtocolEvent{Source: cs.f.endpoints[0], Destination: cs.f.endpoints[1], Timestamp: time.Unix(2, 0), Transport: "udp"}
					cs.f.a.decodeFINSDatagram(e, q, protocolError(ErrNeedMore, "capture truncated"))
					_, err := e.GetFields()
					rocTypedError(t, "NeedMore", err)
				case "bytes":
					cs.f.a.budget.MaxFrameBytes = 12
					out = s.Feed(0, time.Unix(2, 0), q)
					require.Len(t, out.Events, 1)
					_, err := out.Events[0].GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					cs.f.a.budget.MaxFrameBytes = b.MaxFrameBytes
				case "depth":
					cs.f.a.budget.MaxRecursionDepth = 3
					out = s.Feed(0, time.Unix(2, 0), q)
					require.Len(t, out.Events, 1)
					_, err := out.Events[0].GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					cs.f.a.budget.MaxRecursionDepth = b.MaxRecursionDepth
				case "elements":
					cs.f.a.budget.MaxCollectionElements = 32
					out = s.Feed(1, time.Unix(2, 0), r)
					require.Len(t, out.Events, 1)
					_, err := out.Events[0].GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Zero(t, out.Events[0].ResponseTo)
					cs.f.a.budget.MaxCollectionElements = b.MaxCollectionElements
				}
			}
			out := s.Feed(0, time.Unix(4, 0), q)
			require.Len(t, out.Events, 1)
			f, err := out.Events[0].GetFields()
			require.NoError(t, err)
			require.NotEqual(t, "request-observed", f["association_status"])
			out = s.Feed(1, time.Unix(5, 0), r)
			require.Len(t, out.Events, 1)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Zero(t, out.Events[0].TransactionID)
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestFINSTCPReservedEnvelopeAndFatalLength(t *testing.T) {
	c := finsCase(t, "cpu-option0-tcp")
	for _, cmd := range []uint32{0, 2, 6} {
		t.Run(fmt.Sprint(cmd), func(t *testing.T) {
			s := finsSession(t, "tcp", DefaultParserBudget())
			var w []byte
			switch cmd {
			case 0:
				w = wrapperWire(t, "46494e530000000c000000001234567800000000")
			case 2:
				w = wrapperWire(t, c.Events[0].Raw)
				copy(w[12:16], []byte{0x12, 0x34, 0x56, 0x78})
			case 6:
				w = wrapperWire(t, "46494e53000000080000000612345678")
			}
			out := s.Feed(0, time.Unix(1, 0), w)
			require.Len(t, out.Events, 1)
			f, err := out.Events[0].GetFields()
			require.NoError(t, err)
			require.EqualValues(t, 0x12345678, f["tcp_error"])
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, wire := range []string{"46494e5300000007", "46494e53ffffffff", "46494e53000000080000000200000000"} {
		t.Run(wire, func(t *testing.T) {
			s := finsSession(t, "tcp", DefaultParserBudget())
			out := s.Feed(0, time.Unix(1, 0), wrapperWire(t, wire))
			require.Len(t, out.Events, 1)
			require.NotEmpty(t, out.Events[0].Error)
			require.Zero(t, out.Events[0].ResponseTo)
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestFINSCaptureDomainIsolation(t *testing.T) {
	c := finsCase(t, "cpu-option0-udp")
	q, r := wrapperWire(t, c.Events[0].Raw), wrapperWire(t, c.Events[1].Raw)
	for _, domain := range []CaptureDomain{{Interface: 1}, {Section: 1}, {Encapsulation: "vxlan:42"}} {
		t.Run(fmt.Sprint(domain), func(t *testing.T) {
			s := finsSession(t, "udp", DefaultParserBudget())
			a := s.(*captureSession).f.a
			makeEvent := func(reverse bool, d CaptureDomain) *ProtocolEvent {
				src, dst := "192.0.2.1:40000", "192.0.2.2:9600"
				if reverse {
					src, dst = dst, src
				}
				return &ProtocolEvent{Transport: "udp", Source: src, Destination: dst, Domain: d, Timestamp: time.Unix(1, 0)}
			}
			first := makeEvent(false, CaptureDomain{})
			a.decodeFINSDatagram(first, q, nil)
			require.Nil(t, first.sessionError)
			other := makeEvent(true, domain)
			a.decodeFINSDatagram(other, r, nil)
			require.Zero(t, other.ResponseTo)
			_, err := other.GetFields()
			require.NoError(t, err)
			matching := makeEvent(true, CaptureDomain{})
			a.decodeFINSDatagram(matching, r, nil)
			require.Equal(t, first.ID, matching.ResponseTo)
			require.Equal(t, first.FlowID, matching.FlowID)
			require.NotEqual(t, other.FlowID, matching.FlowID)
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestFINSTCPCoalescedAndPartialClose(t *testing.T) {
	c := finsCase(t, "cpu-option0-tcp")
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			s := finsSession(t, "tcp", DefaultParserBudget())
			q := wrapperWire(t, c.Events[0].Raw)
			d := 0
			if reverse {
				d = 1
			}
			twice := append(bytes.Clone(q), q...)
			out := s.Feed(d, time.Unix(1, 0), twice)
			require.Len(t, out.Events, 2)
			f, err := out.Events[1].GetFields()
			require.NoError(t, err)
			require.Equal(t, "active-request-retry", f["fins"].(map[string]any)["association_status"])
			require.Equal(t, out.Events[0].ID, out.Events[1].TransactionID)
			out = s.Feed(1-d, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
			require.Len(t, out.Events, 1)
			require.Nil(t, out.Err)
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, transport := range []string{"tcp", "udp"} {
		for _, d := range []int{0, 1} {
			t.Run(fmt.Sprintf("close-%s-%d", transport, d), func(t *testing.T) {
				s := finsSession(t, transport, DefaultParserBudget())
				c := finsCase(t, "cpu-option0-"+transport)
				o := s.Feed(d, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
				require.Len(t, o.Events, 1)
				close := s.Close("end")
				require.Len(t, close, 1)
				require.Equal(t, d, close[0].Direction)
				require.Equal(t, "incomplete", close[0].Status)
				_, err := close[0].GetFields()
				rocTypedError(t, "NeedMore", err)
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestFINSSelectedAdmissionAndIncompleteClose(t *testing.T) {
	_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionFINS(9600), WithSessionTransport("tcp"))
	require.Error(t, err)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionFINS(9600), WithSessionTransport("udp"))
	require.NoError(t, err)
	s.Close("end")
	c := finsCase(t, "cpu-option0-tcp")
	s = finsSession(t, "tcp", DefaultParserBudget())
	w := wrapperWire(t, c.Events[0].Raw)
	out := s.Feed(0, time.Unix(1, 0), w[:len(w)-1])
	require.Empty(t, out.Events)
	require.True(t, out.NeedMore)
	events := s.Close("end")
	require.Len(t, events, 1)
	require.Equal(t, "incomplete", events[0].Status)
	require.NotEmpty(t, events[0].Error)
	_, err = events[0].GetFields()
	rocTypedError(t, "NeedMore", err)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	// Reuse the same sealed node0/1 bytes; only observed TCP direction changes.
	// All12 capture configurations and IPv4/IPv6 must reject the false link.
	nodes := finsCase(t, "tcp-node-cpu-notifications-tcp")
	executed := 0
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		for _, ipv6 := range []bool{false, true} {
			raw := sessionTestPCAP(t, []sessionStep{{dir: 1, wire: wrapperWire(t, nodes.Events[0].Raw)}, {dir: 0, wire: wrapperWire(t, nodes.Events[1].Raw)}}, layers.TCPPort(9600), 0, ipv6, false)
			events, stats := discoveryReplay(t, raw, workers, deferred, observe)
			require.Len(t, events, 2)
			for i, e := range events {
				require.Equal(t, 1-i, e.Direction)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				require.Equal(t, nodes.Events[i].Raw, hex.EncodeToString(e.Raw))
				want := cloneSession(nodes.Events[i].Fields)
				want["allocation_status"] = "unverified-response"
				if i == 0 {
					want["allocation_status"] = "unverified-tcp-role"
				}
				f, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, want, f)
				rocEqualFields(t, want, e.Session)
			}
			require.Equal(t, events[0].FlowID, events[1].FlowID)
			require.Zero(t, stats.BufferedBytes)
			executed++
		}
	})
	require.Equal(t, 24, executed)

}

func TestFINSLiveOwnershipAndExactAssociation(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", transport, deferred), func(t *testing.T) {
				c := finsCase(t, "cpu-option0-"+transport)
				s := finsSession(t, transport, DefaultParserBudget())
				s.(*captureSession).f.a.config.Deferred = deferred
				q := wrapperWire(t, c.Events[0].Raw)
				first := s.Feed(0, time.Unix(1, 0), q)
				require.Len(t, first.Events, 1)
				e := first.Events[0]
				id := e.ID
				clear(q)
				clear(e.Raw)
				clear(e.Session)
				e.ID, e.TransactionID = 999999, 999999
				e.Protocol = "caller-mutated"
				e.Fields = map[string]any{"forged": true}
				e.Structured = map[string]any{"fields": e.Fields}
				out := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
				require.Len(t, out.Events, 1)
				reply := out.Events[0]
				require.Equal(t, id, reply.ResponseTo)
				require.Equal(t, id, reply.TransactionID)
				f, err := reply.GetFields()
				require.NoError(t, err)
				want := finsResolve(c.Events[1].Fields, []*ProtocolEvent{{ID: id}}).(map[string]any)
				rocEqualFields(t, want, f)
				rocEqualFields(t, want, reply.Session)
				require.Empty(t, s.Close("end"))
				require.Zero(t, s.Stats().BufferedBytes)

				if transport == "tcp" {
					clocked := finsSession(t, "tcp", DefaultParserBudget())
					clocked.(*captureSession).f.a.config.Deferred = deferred
					first := clocked.Feed(0, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
					require.Len(t, first.Events, 1)
					confirm := clocked.Feed(1, time.Unix(32, 0), wrapperWire(t, "46494e53000000080000000600000000"))
					require.Len(t, confirm.Events, 1)
					late := clocked.Feed(1, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
					require.Len(t, late.Events, 1)
					require.Zero(t, late.Events[0].ResponseTo)
					require.Zero(t, late.Events[0].TransactionID)
					want := cloneSession(c.Events[1].Fields)
					inner := want["fins"].(map[string]any)
					inner["association_status"], inner["option_context_observed"] = "unverified-response", false
					delete(inner, "observed_request_id")
					f, err := late.Events[0].GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, f)
					rocEqualFields(t, want, late.Events[0].Session)
					q, r := wrapperWire(t, c.Events[0].Raw), wrapperWire(t, c.Events[1].Raw)
					q[25], r[25] = 1, 1
					fresh := clocked.Feed(0, time.Unix(2, 0), q)
					require.Len(t, fresh.Events, 1)
					response := clocked.Feed(1, time.Unix(3, 0), r)
					require.Len(t, response.Events, 1)
					require.Equal(t, fresh.Events[0].ID, response.Events[0].ResponseTo)
					require.Empty(t, clocked.Close("end"))
					require.Zero(t, clocked.Stats().BufferedBytes)
				}
			})
		}
	}
}
