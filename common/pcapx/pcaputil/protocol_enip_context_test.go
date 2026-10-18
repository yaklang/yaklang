package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func enipContextWire(cmd uint16, session uint32, context uint64, body []byte) []byte {
	w := make([]byte, 24+len(body))
	binary.LittleEndian.PutUint16(w, cmd)
	binary.LittleEndian.PutUint16(w[2:], uint16(len(body)))
	binary.LittleEndian.PutUint32(w[4:], session)
	binary.LittleEndian.PutUint64(w[12:], context)
	copy(w[24:], body)
	return w
}
func enipContextCPF(cip []byte) []byte {
	b := make([]byte, 16+len(cip))
	binary.LittleEndian.PutUint16(b[6:], 2)
	binary.LittleEndian.PutUint16(b[12:], 0xb2)
	binary.LittleEndian.PutUint16(b[14:], uint16(len(cip)))
	copy(b[16:], cip)
	return b
}
func enipContextQuery(tag string) []byte {
	path := append([]byte{0x91, byte(len(tag))}, []byte(tag)...)
	if len(path)%2 != 0 {
		path = append(path, 0)
	}
	return append(append([]byte{0x4c, byte(len(path) / 2)}, path...), 1, 0)
}
func enipContextReply(value uint32) []byte {
	w := []byte{0xcc, 0, 0, 0, 0xc4, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(w[6:], value)
	return w
}
func TestENIPContextStrictAssociation(t *testing.T) {
	run := func(t *testing.T, kind string) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 44818))
		require.NoError(t, err)
		defer s.Close("cleanup")
		var tick int64
		feed := func(dir int, w []byte) *ProtocolEvent {
			tick++
			out := s.Feed(dir, time.Unix(tick, 0), w)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			require.Equal(t, w, e.Raw)
			require.Equal(t, len(w), e.Length)
			require.Equal(t, "enip", e.Protocol)
			require.NotZero(t, e.ID)
			require.NotZero(t, e.FlowID)
			return e
		}
		good := func(e *ProtocolEvent, tag string, value *int32) {
			f, err := e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, f, e.Session)
			if tag != "" {
				require.Equal(t, tag, e.Session["Tag"])
			}
			if value != nil {
				require.Equal(t, *value, e.Session["Read Value"])
			}
			require.Equal(t, "ethernet-ip-cip-explicit", e.Profile)
		}
		refuse := func(e *ProtocolEvent) {
			f, err := e.GetFields()
			rocTypedError(t, "ContextRequired", err)
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
		}
		rq := enipContextWire(0x65, 0, 0x0102030405060708, []byte{1, 0, 0, 0})
		good(feed(0, rq), "", nil)
		rc := uint64(0x0102030405060708)
		if kind == "register-wrong-context" {
			rc++
		}
		rr := enipContextWire(0x65, 0x5013, rc, []byte{1, 0, 0, 0})
		reg := feed(1, rr)
		if kind == "register-wrong-context" {
			refuse(reg)
			return
		}
		good(reg, "", nil)
		query := func(tag string, ctx uint64) *ProtocolEvent {
			return feed(0, enipContextWire(0x6f, 0x5013, ctx, enipContextCPF(enipContextQuery(tag))))
		}
		reply := func(value uint32, ctx uint64) *ProtocolEvent {
			return feed(1, enipContextWire(0x6f, 0x5013, ctx, enipContextCPF(enipContextReply(value))))
		}
		good(query("Alpha", 11), "Alpha", nil)
		if kind == "distinct-context-pipeline" {
			good(query("Beta", 22), "Beta", nil)
			v := int32(2500)
			good(reply(2500, 22), "Beta", &v)
			v = 1500
			good(reply(1500, 11), "Alpha", &v)
		} else if kind == "reply-wrong-context" {
			refuse(reply(1500, 22))
		} else {
			v := int32(1500)
			good(reply(1500, 11), "Alpha", &v)
			if kind == "completed-context-reuse" {
				e := query("Beta", 11)
				f, err := e.GetFields()
				if err != nil {
					rocTypedError(t, "ContextRequired", err)
					require.Nil(t, f)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				} else {
					good(e, "Beta", nil)
					refuse(reply(1500, 11))
				}
			}
		}
		// All observed sender contexts are opaque eight bytes, never host-endian IDs.
		require.True(t, bytes.Equal(rq[12:20], []byte{8, 7, 6, 5, 4, 3, 2, 1}))
		s.Close("end")
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
	for _, name := range []string{"adjacent-matched", "register-wrong-context", "reply-wrong-context", "distinct-context-pipeline", "completed-context-reuse"} {
		t.Run(name, func(t *testing.T) { run(t, name) })
	}
}
func TestENIPContextQueryOwnership(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted-field-ownership", true: "refusal-cannot-be-forged"}[bad], func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 44818))
			require.NoError(t, err)
			defer s.Close("cleanup")
			tick := int64(0)
			feed := func(dir int, w []byte) *ProtocolEvent {
				tick++
				out := s.Feed(dir, time.Unix(tick, 0), w)
				require.Len(t, out.Events, 1)
				return out.Events[0]
			}
			feed(0, enipContextWire(0x65, 0, 1, []byte{1, 0, 0, 0}))
			feed(1, enipContextWire(0x65, 0x5013, 1, []byte{1, 0, 0, 0}))
			wire := enipContextWire(0x6f, 0x5013, 11, enipContextCPF(enipContextQuery("Alpha")))
			e := feed(0, wire)
			want := map[string]any{"Command Code": uint16(0x6f), "Command": "SendRRData", "Length": uint16(len(wire) - 24), "Session Handle": uint32(0x5013), "Status": uint32(0), "Sender Context": []byte{11, 0, 0, 0, 0, 0, 0, 0}, "Options": uint32(0), "Encapsulation": "EtherNet/IP", "Interface Handle": uint32(0), "Timeout": uint16(0), "CIP Service Code": byte(0x4c), "CIP Service": "Read Tag Request", "Tag": "Alpha", "Element Count": uint16(1)}
			if bad {
				cip := enipContextReply(1500)
				cip[1] = 1
				e = feed(1, enipContextWire(0x6f, 0x5013, 11, enipContextCPF(cip)))
			} else {
				f, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, want, f)
				f["Tag"] = "edited"
				clear(f["Sender Context"].([]byte))
			}
			if e.Fields != nil {
				clear(e.Fields)
			}
			if e.Structured != nil {
				if tree, ok := e.Structured["fields"].(map[string]any); ok {
					clear(tree)
				}
			}
			if e.Session != nil {
				clear(e.Session)
			}
			e.Fields = map[string]any{"forged": true}
			e.Structured = map[string]any{"fields": e.Fields}
			e.Session = map[string]any{"forged": true}
			e.Protocol = "edited"
			e.Profile = "edited"
			e.Status = "decoded"
			clear(e.Raw)
			clear(wire)
			f, err := e.GetFields()
			if bad {
				rocTypedError(t, "MalformedMessage", err)
				require.Nil(t, f)
			} else {
				require.NoError(t, err)
				rocEqualFields(t, want, f)
				out, err := e.Decode()
				require.NoError(t, err)
				rocEqualFields(t, want, out["session"].(map[string]any))
			}
			s.Close("end")
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

type enipContextControl struct {
	ID, Capture, SHA256, Answer, AnswerSHA string
	Packets, Outstanding                   int
	Events                                 []struct {
		Raw, Error         string
		Direction          int
		Fields             map[string]any
		Reply, Transaction *int
		PacketRefs         []int
	}
}

func enipContextFile(t *testing.T, n string) []byte {
	t.Helper()
	if root := os.Getenv("ENIP_CONTEXT_TEST_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, "enip-context", n))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile("enip-context/" + n)
	require.NoError(t, err)
	return b
}
func enipContextControls(t *testing.T) []enipContextControl {
	t.Helper()
	var d struct {
		Schema string
		Cases  []enipContextControl
	}
	require.NoError(t, decodeExactProtocolAnswer(enipContextFile(t, "controls.json"), &d))
	require.Equal(t, "owned-enip-context/v1", d.Schema)
	require.Len(t, d.Cases, 42)
	return d.Cases
}
func enipContextAssert(t *testing.T, c enipContextControl, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	n := len(c.Events)
	if c.Outstanding > 0 {
		n++
	}
	require.Len(t, events, n)
	seen := map[uint64]bool{}
	var flow uint64
	for i, w := range c.Events {
		e := events[i]
		require.Equal(t, "enip", e.Protocol)
		require.Equal(t, "tcp", e.Transport)
		require.Equal(t, "ethernet-ip-cip-explicit", e.Profile)
		require.Equal(t, wrapperWire(t, w.Raw), e.Raw)
		require.Equal(t, len(w.Raw)/2, e.Length)
		require.Equal(t, w.Direction, e.Direction)
		require.NotZero(t, e.ID)
		require.False(t, seen[e.ID])
		seen[e.ID] = true
		if flow == 0 {
			flow = e.FlowID
		}
		require.NotZero(t, flow)
		require.Equal(t, flow, e.FlowID)
		reply, txn := uint64(0), uint64(0)
		if w.Reply != nil {
			require.Less(t, *w.Reply, i)
			reply = events[*w.Reply].ID
		}
		if w.Transaction != nil {
			require.LessOrEqual(t, *w.Transaction, i)
			txn = events[*w.Transaction].ID
		}
		require.Equal(t, reply, e.ResponseTo)
		require.Equal(t, txn, e.TransactionID)
		f, err := e.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error != "" {
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
			assertOwnedProtocolRefusal(t, e, w.Error, err)
		} else {
			rocEqualFields(t, w.Fields, f)
			rocEqualFields(t, w.Fields, e.Session)
			status := "decoded"
			if deferred {
				status = "deferred"
			}
			require.Equal(t, status, e.Status)
			require.Equal(t, "message", e.Completeness)
			require.Empty(t, e.Error)
		}
	}
	if c.Outstanding > 0 {
		e := events[len(c.Events)]
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, "incomplete", e.Completeness)
		require.Empty(t, e.Raw)
		require.Zero(t, e.Length)
		require.Equal(t, flow, e.FlowID)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		rocEqualFields(t, map[string]any{"Outstanding": c.Outstanding}, e.Session)
		f, err := e.GetFields()
		rocTypedError(t, "NeedMore", err)
		require.Nil(t, f)
	}
}
func TestENIPContextSealedMatrix(t *testing.T) {
	for _, c := range enipContextControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := enipContextFile(t, c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(enipContextFile(t, c.Answer))))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe)
				enipContextAssert(t, c, events, deferred)
				require.Zero(t, stats.BufferedBytes)
				for i, w := range c.Events {
					e := events[i]
					src, dst := "192.0.2.1:40000", "192.0.2.2:44818"
					if w.Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					require.Equal(t, CaptureDomain{}, e.Domain)
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
func TestENIPContextSealedChunks(t *testing.T) {
	for _, c := range enipContextControls(t) {
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{1, 7, 64, 4096} {
				t.Run(fmt.Sprintf("%s/df%t/chunk%d", c.ID, deferred, chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 44818))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for i, w := range c.Events {
						raw := wrapperWire(t, w.Raw)
						for at := 0; at < len(raw); at += chunk {
							in := bytes.Clone(raw[at:min(len(raw), at+chunk)])
							out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), in)
							clear(in)
							events = append(events, out.Events...)
						}
					}
					events = append(events, s.Close("end")...)
					enipContextAssert(t, c, events, deferred)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

func TestENIPContextBudgetLifecycle(t *testing.T) {
	all := enipContextControls(t)
	var seed enipContextControl
	for _, c := range all {
		if c.ID == "read-match" {
			seed = c
		}
	}
	for _, mode := range []string{"pending-quota", "history-quota", "byte-quota"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/df%t", mode, deferred), func(t *testing.T) {
				b := DefaultParserBudget()
				if mode == "byte-quota" {
					b.MaxBufferedBytes = 2048
					b.MaxMessageBytes, b.MaxFrameBytes = 1024, 1024
				} else {
					b.MaxCollectionElements = 32
				}
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(40000, 44818))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				defer s.Close("cleanup")
				clock := int64(0)
				feed := func(d int, w []byte) *ProtocolEvent {
					clock++
					out := s.Feed(d, time.Unix(clock, 0), w)
					require.Len(t, out.Events, 1)
					require.LessOrEqual(t, s.Stats().BufferedBytes, int64(b.MaxBufferedBytes))
					return out.Events[0]
				}
				for _, w := range seed.Events[:2] {
					e := feed(w.Direction, wrapperWire(t, w.Raw))
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, f)
				}
				accepted := 0
				var rejected *ProtocolEvent
				for i := 0; i < 100; i++ {
					ctx := uint64(100 + i)
					raw := enipContextWire(0x6f, 0x5013, ctx, enipContextCPF(enipContextQuery("Alpha")))
					e := feed(0, raw)
					f, err := e.GetFields()
					if err != nil {
						rocTypedError(t, "ResourceExceeded", err)
						require.Nil(t, f)
						require.Nil(t, e.Session)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						assertOwnedProtocolRefusal(t, e, "ResourceExceeded", err)
						rejected = e
						break
					}
					accepted++
					want := cloneSession(seed.Events[2].Fields)
					want["Sender Context"] = base64.StdEncoding.EncodeToString(raw[12:20])
					rocEqualFields(t, want, f)
					rocEqualFields(t, want, e.Session)
					require.Equal(t, e.ID, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					if mode == "history-quota" {
						reply := feed(1, enipContextWire(0x6f, 0x5013, ctx, enipContextCPF(enipContextReply(1500))))
						f, err := reply.GetFields()
						require.NoError(t, err)
						want := cloneSession(seed.Events[3].Fields)
						want["Sender Context"] = base64.StdEncoding.EncodeToString(raw[12:20])
						rocEqualFields(t, want, f)
						require.Equal(t, e.ID, reply.ResponseTo)
						require.Equal(t, e.ID, reply.TransactionID)
					}
				}
				require.NotNil(t, rejected, "finite byte/history/pending state must refuse before the finite input bound")
				require.Positive(t, accepted)
				if mode != "byte-quota" {
					require.Equal(t, 32, accepted)
				}
				require.Zero(t, s.Stats().BufferedBytes, "terminal refusal releases private state immediately")
				late := s.Feed(1, time.Unix(1000, 0), enipContextWire(0x6f, 0x5013, 100, enipContextCPF(enipContextReply(9999))))
				require.Empty(t, late.Events)
				require.Empty(t, s.Close("end"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
func TestENIPContextLiveOwnership(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("df%t", deferred), func(t *testing.T) {
			var c enipContextControl
			for _, v := range enipContextControls(t) {
				if v.ID == "active-identical-retry" {
					c = v
				}
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 44818))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			defer s.Close("cleanup")
			var observed []*ProtocolEvent
			for i, w := range c.Events {
				wire := wrapperWire(t, w.Raw)
				out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wire)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				f, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, w.Fields, f)
				snapshot := *e
				snapshot.Raw = wrapperWire(t, w.Raw)
				snapshot.Session = cloneSession(w.Fields)
				observed = append(observed, &snapshot)
				f["Sender Context"] = "edited"
				f["Tag"] = "edited"
				if e.Fields != nil {
					clear(e.Fields)
				}
				if e.Structured != nil {
					clear(e.Structured)
				}
				clear(e.Session)
				clear(e.Raw)
				clear(wire)
				e.ID = 999999
				e.ResponseTo = 999999
				e.TransactionID = 999999
				e.Protocol = "edited"
				e.Profile = "edited"
				e.Status = "decoded"
				e.Fields = map[string]any{"forged": true}
				e.Structured = map[string]any{"fields": e.Fields}
				f, err = e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, w.Fields, f)
				result, err := e.Decode()
				require.NoError(t, err)
				rocEqualFields(t, w.Fields, result["session"].(map[string]any))
			}
			enipContextAssert(t, c, observed, deferred)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestENIPContextCoalescedPipeline(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, chunk := range []int{1, 7, 4096} {
			t.Run(fmt.Sprintf("df%t/chunk%d", deferred, chunk), func(t *testing.T) {
				var c enipContextControl
				for _, x := range enipContextControls(t) {
					if x.ID == "pipeline-reverse" {
						c = x
					}
				}
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 44818))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				defer s.Close("cleanup")
				var got []*ProtocolEvent
				groups := [][]int{{0}, {1}, {2, 3}, {4, 5}}
				for _, g := range groups {
					var wire []byte
					for _, i := range g {
						wire = append(wire, wrapperWire(t, c.Events[i].Raw)...)
					}
					for at := 0; at < len(wire); at += chunk {
						input := bytes.Clone(wire[at:min(len(wire), at+chunk)])
						out := s.Feed(c.Events[g[0]].Direction, time.Unix(int64(g[0]+1), 0), input)
						clear(input)
						got = append(got, out.Events...)
					}
				}
				enipContextAssert(t, c, got, deferred)
				require.Empty(t, s.Close("end"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestENIPContextHistorical(t *testing.T) {
	var answer []byte
	if root := os.Getenv("ENIP_CONTEXT_TEST_ROOT"); root != "" {
		var err error
		answer, err = os.ReadFile(filepath.Join(filepath.Dir(root), "tools", "historical-answer.json"))
		require.NoError(t, err)
	} else {
		var err error
		answer, err = trafficfixture.ReadFile("enip-context-tools/historical-answer.json")
		require.NoError(t, err)
	}
	var c enipContextControl
	require.NoError(t, decodeExactProtocolAnswer(answer, &c))
	raw, err := trafficfixture.ReadFile(filepath.Join("..", "..", "..", c.Capture))
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		events, stats := discoveryReplay(t, raw, workers, deferred, observe)
		enipContextAssert(t, c, events, deferred)
		require.Zero(t, stats.BufferedBytes)
		for i, e := range events {
			src, dst := "192.0.2.10:44418", "192.0.2.20:44818"
			if e.Direction == 1 {
				src, dst = dst, src
			}
			require.Equal(t, src, e.Source)
			require.Equal(t, dst, e.Destination)
			require.Len(t, e.SourceBytes.PacketRefs, len(c.Events[i].PacketRefs))
			for j, ref := range e.SourceBytes.PacketRefs {
				require.EqualValues(t, c.Events[i].PacketRefs[j], ref.Number)
				require.Equal(t, e.Domain, ref.Domain)
			}
		}
	})
}
