package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type rocControl struct {
	ID, Capture, Answer, SHA256 string
	AnswerSHA                   string `json:"answer_sha256"`
	Packets                     int
}
type rocAnswer struct {
	Steps []struct {
		Dir  int
		Wire string
	}
	Events []struct {
		Fields             map[string]any
		Error, Raw, Status string
		Protocol           *string
	}
	PureErrors       []string         `json:"pure_errors"`
	PureFields       []map[string]any `json:"pure_fields"`
	CloseOutstanding int              `json:"close_outstanding"`
	Packets, Port    int
}

func rocControls(t *testing.T) []rocControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("roc-plus-clock/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []rocControl
	}
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, "pr5013-roc-plus-clock/v1", m.Schema)
	require.Len(t, m.Cases, 45)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	found := map[string]trafficfixture.FrozenCase{}
	for _, b := range all {
		for _, c := range b.Cases {
			found[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := found["roc-plus-clock/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.Capture, v.Input.File)
		require.Equal(t, c.SHA256, v.Input.SHA256)
		require.Equal(t, c.Packets, v.Facts.PacketCount)
		require.Len(t, v.Expectations, 1)
		var binding struct {
			AnswerFile string `json:"answer_file"`
			AnswerSHA  string `json:"answer_sha256"`
		}
		require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
		require.Equal(t, c.Answer, binding.AnswerFile)
		require.Equal(t, c.AnswerSHA, binding.AnswerSHA)
	}
	return m.Cases
}
func rocInput(t *testing.T, c rocControl) ([]byte, rocAnswer) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("roc-plus-clock/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	a, err := trafficfixture.ReadFile("roc-plus-clock/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(a)))
	var want rocAnswer
	require.NoError(t, json.Unmarshal(a, &want))
	require.Equal(t, c.Packets, want.Packets)
	require.Len(t, want.Events, len(want.Steps))
	require.Len(t, want.PureErrors, len(want.Steps))
	require.Len(t, want.PureFields, len(want.Steps))
	return raw, want
}
func rocEqualFields(t *testing.T, want map[string]any, got map[string]any) {
	t.Helper()
	b, err := json.Marshal(got)
	require.NoError(t, err)
	var normalized map[string]any
	require.NoError(t, json.Unmarshal(b, &normalized))
	require.Equal(t, want, normalized)
}
func rocTypedError(t *testing.T, kind string, err error) {
	t.Helper()
	if kind == "" {
		require.NoError(t, err)
		return
	}
	var pe *ProtocolError
	require.Error(t, err)
	require.True(t, errors.As(err, &pe))
	require.Equal(t, kind, string(pe.Kind))
}
func rocCheckEvents(t *testing.T, want rocAnswer, events []*ProtocolEvent, endpoints bool) {
	t.Helper()
	require.Len(t, events, len(want.Events)+want.CloseOutstanding)
	requests := map[float64]uint64{}
	for i, answer := range want.Events {
		e := events[i]
		protocol := "roc-plus"
		if answer.Protocol != nil {
			protocol = *answer.Protocol
		}
		require.Equal(t, protocol, e.Protocol)
		require.Equal(t, answer.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, want.Steps[i].Dir, e.Direction)
		require.Zero(t, e.TransactionID, "ROC has no transaction ID on the wire")
		if endpoints {
			src, dst := "192.0.2.10:38000", fmt.Sprintf("192.0.2.20:%d", want.Port)
			if e.Direction == 1 {
				src, dst = dst, src
			}
			require.Equal(t, src, e.Source)
			require.Equal(t, dst, e.Destination)
			require.NotEmpty(t, e.SourceBytes.PacketRefs)
			for _, ref := range e.SourceBytes.PacketRefs {
				require.GreaterOrEqual(t, ref.Number, uint64(4))
				require.LessOrEqual(t, ref.Number, uint64(want.Packets))
				require.Equal(t, e.Domain, ref.Domain)
			}
		}
		if answer.Fields != nil {
			require.Contains(t, []string{"decoded", "deferred"}, e.Status)
			require.Empty(t, e.Error)
			require.Equal(t, "roc-plus-tcp-clock-2022", e.Profile)
			fields, err := e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, answer.Fields, fields)
			rocEqualFields(t, answer.Fields, e.Session)
			x := answer.Fields["Observed Exchange"].(float64)
			if answer.Fields["Association"] == "matched" {
				require.NotZero(t, requests[x])
				require.Equal(t, requests[x], e.ResponseTo)
			} else {
				require.Zero(t, e.ResponseTo)
				requests[x] = e.ID
			}
			fields["Observation"] = "caller-mutated"
			require.Equal(t, "unverified-station-clock", e.Session["Observation"])
			if clock, ok := fields["Clock"].(map[string]any); ok {
				clock["Year"] = 0
				require.NotZero(t, e.Session["Clock"].(map[string]any)["Year"])
			}
			if pairs, ok := fields["Errors"].([]map[string]any); ok {
				pairs[0]["Code"] = 999
				require.NotEqualValues(t, 999, e.Session["Errors"].([]map[string]any)[0]["Code"])
			}
		} else if answer.Error != "" {
			rocTypedError(t, answer.Error, e.sessionError)
			require.NotEmpty(t, e.Error)
			require.Nil(t, e.Session)
			require.Nil(t, e.Structured)
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, answer.Status, e.Status)
			require.Empty(t, e.Error)
			require.Nil(t, e.Session)
		}
	}
	if want.CloseOutstanding > 0 {
		e := events[len(want.Events)]
		require.Equal(t, "roc-plus", e.Protocol)
		require.Equal(t, "incomplete", e.Status)
		require.EqualValues(t, want.CloseOutstanding, e.Session["Outstanding"])
		require.Empty(t, e.Raw)
	}
}

func TestROCPlusClockSealedReplay(t *testing.T) {
	for _, c := range rocControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := rocInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							err := ReplayPcap(bytes.NewReader(raw), opts...)
							require.NoError(t, err)
							require.EqualValues(t, want.Packets, assembly.CapturedPackets)
							if observe {
								require.EqualValues(t, want.Packets, seen.Load())
							}
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, assembly.UnreassembledBytes)
							require.Zero(t, assembly.UnreassembledSegments)
							rocCheckEvents(t, want, events, true)
						})
					}
				}
			}
		})
	}
}

func TestROCPlusClockBoundariesAndLifetime(t *testing.T) {
	var rq, rp []byte
	for _, c := range rocControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, want := rocInput(t, c)
			for i, step := range want.Steps {
				w, err := hex.DecodeString(step.Wire)
				require.NoError(t, err)
				fields, err := decodeROCClock(w, 4096)
				rocTypedError(t, want.PureErrors[i], err)
				if err == nil {
					rocEqualFields(t, want.PureFields[i], fields)
				} else {
					require.Nil(t, fields)
				}
				if c.ID == "read-set-segment0-port4000" && i < 2 {
					if i == 0 {
						rq = append([]byte(nil), w...)
					} else {
						rp = append([]byte(nil), w...)
					}
				}
			}
			for _, chunk := range []int{1, 7, 4096} {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
				require.NoError(t, err)
				first, _ := hex.DecodeString(want.Steps[0].Wire)
				for i := 0; i < 3; i++ {
					s.Probe(first)
					require.Zero(t, s.Stats().Messages)
					require.Zero(t, s.Stats().BufferedBytes)
				}
				invalid := s.Feed(2, time.Unix(1700000000, 0), first)
				require.Zero(t, invalid.Consumed)
				require.Empty(t, invalid.Events)
				require.Equal(t, ErrMalformedMessage, invalid.Err.Kind)
				var events []*ProtocolEvent
				for _, step := range want.Steps {
					w, _ := hex.DecodeString(step.Wire)
					for at := 0; at < len(w); at += chunk {
						b := w[at:min(at+chunk, len(w))]
						r := s.Feed(step.Dir, time.Unix(1700000000, 0), b)
						require.Equal(t, len(b), r.Consumed)
						events = append(events, r.Events...)
					}
					clear(w)
				}
				events = append(events, s.Close("fixture-end")...)
				rocCheckEvents(t, want, events, false)
				require.Zero(t, s.Stats().BufferedBytes)
				require.Empty(t, s.Close("again"))
				r := s.Feed(0, time.Now(), []byte{1})
				require.Equal(t, ErrFatalSessionError, r.Err.Kind)
				require.Empty(t, r.Events)
			}
		})
	}
	require.NotEmpty(t, rq)
	require.NotEmpty(t, rp)
	// The protocol manual's independent CRC vector, not generated Yak output.
	v, _ := hex.DecodeString("0102010011034d4f43")
	require.Equal(t, uint16(0x1885), rocCRC(v))
	for cut := 0; cut < len(rq); cut++ {
		require.NotEqual(t, ProbeAccept, probeROCPlus(rq[:cut], 1024).Verdict)
	}
	badCRC := append([]byte(nil), rq...)
	badCRC[len(badCRC)-1] ^= 1
	require.Equal(t, ProbeReject, probeROCPlus(badCRC, 1024).Verdict)
	for _, limit := range []int{255, 256} {
		budget := DefaultParserBudget()
		budget.MaxBufferedBytes = limit
		budget.MaxMessageBytes = 64
		s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("tcp"), WithSessionClientDirection(0))
		require.NoError(t, err)
		r := s.Feed(0, time.Now(), rq)
		if limit < 256 {
			require.NotNil(t, r.Err)
			require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		} else {
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, int64(256), s.(*captureSession).f.sessionBytes)
		}
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	_, err := rocFrameSize(rp, len(rp)-1)
	rocTypedError(t, "ResourceExceeded", err)
	s := &binROCPlus{clientKnown: true, exchange: math.MaxUint64}
	_, _, err = s.consume(0, rq, 1, 1)
	rocTypedError(t, "ResourceExceeded", err)
	require.Nil(t, s.pending)
	s = &binROCPlus{clientKnown: true}
	_, _, err = s.consume(0, rq, 1, 0)
	rocTypedError(t, "ResourceExceeded", err)
	require.Nil(t, s.pending)
	unknown, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	r := unknown.Feed(0, time.Now(), rq)
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	unknown.Close("end")
	require.Zero(t, unknown.Stats().BufferedBytes)
	udp, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"))
	require.NoError(t, err)
	require.NotEqual(t, "roc-plus", udp.Probe(rq).Protocol)
	udp.Close("end")
	// Every strict response prefix ends incomplete, with no fabricated reply.
	for cut := 0; cut < len(rp); cut++ {
		session, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
		require.NoError(t, err)
		require.Nil(t, session.Feed(0, time.Now(), rq).Err)
		response := session.Feed(1, time.Now(), rp[:cut])
		require.Empty(t, response.Events)
		closed := session.Close("truncated-response")
		require.Len(t, closed, 1+func() int {
			if cut > 0 {
				return 1
			}
			return 0
		}())
		var slot bool
		for _, e := range closed {
			require.Equal(t, "incomplete", e.Status)
			require.Zero(t, e.ResponseTo)
			if e.Session["Outstanding"] != nil {
				require.EqualValues(t, 1, e.Session["Outstanding"])
				slot = true
			}
		}
		require.True(t, slot)
		require.Zero(t, session.Stats().BufferedBytes)
	}
	// Coalescing a complete wrong-calendar frame and the correct response
	// must preserve the exact boundary and the original request association.
	for _, c := range rocControls(t) {
		if c.ID == "bad-february-30" {
			_, want := rocInput(t, c)
			session, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			first := session.Feed(0, time.Now(), rq)
			var merged []byte
			for _, step := range want.Steps[1:] {
				wire, _ := hex.DecodeString(step.Wire)
				merged = append(merged, wire...)
			}
			next := session.Feed(1, time.Now(), merged)
			require.Equal(t, len(merged), next.Consumed)
			events := append(first.Events, next.Events...)
			events = append(events, session.Close("merged")...)
			rocCheckEvents(t, want, events, false)
			require.Zero(t, session.Stats().BufferedBytes)
		}
	}
	// Two code/offset entries need two collection slots, exactly.
	for _, c := range rocControls(t) {
		if c.ID == "known-and-future-error-code" {
			_, want := rocInput(t, c)
			wire, _ := hex.DecodeString(want.Steps[1].Wire)
			for _, limit := range []int{1, 2} {
				fields, err := decodeROCClock(wire, limit)
				if limit == 1 {
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, fields)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, want.PureFields[1], fields)
				}
			}
		}
	}
	// Resource failures are not recoverable message diagnostics.
	for _, c := range rocControls(t) {
		if c.ID == "known-and-future-error-code" {
			_, want := rocInput(t, c)
			wire, _ := hex.DecodeString(want.Steps[1].Wire)
			budget := DefaultParserBudget()
			budget.MaxCollectionElements = 1
			session, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			require.Nil(t, session.Feed(0, time.Now(), rq).Err)
			r := session.Feed(1, time.Now(), wire)
			require.Equal(t, ErrResourceExceeded, r.Err.Kind)
			require.Len(t, r.Events, 1)
			require.Nil(t, r.Events[0].Session)
			require.Nil(t, session.(*captureSession).f.rocplus)
			require.Zero(t, session.Stats().BufferedBytes)
			r = session.Feed(1, time.Now(), rp)
			require.Equal(t, ErrFatalSessionError, r.Err.Kind)
			require.Empty(t, r.Events)
			session.Close("limited")
			require.Zero(t, session.Stats().BufferedBytes)
		}
	}
	// One flow's slot and retained result must never alias a peer or its input.
	a, b := &binROCPlus{clientKnown: true}, &binROCPlus{clientKnown: true}
	_, _, err = a.consume(0, rq, 31, 2)
	require.NoError(t, err)
	_, _, err = b.consume(1, rp, 32, 2)
	rocTypedError(t, "ContextRequired", err)
	fields, reply, err := a.consume(1, rp, 33, 2)
	require.NoError(t, err)
	require.Equal(t, uint64(31), reply)
	clear(rp)
	require.EqualValues(t, 2024, fields["Clock"].(map[string]any)["Year"])
}
