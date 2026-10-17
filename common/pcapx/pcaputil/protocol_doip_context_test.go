package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDoIPContextWireValidationBeforeAmbiguity(t *testing.T) {
	rq := doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, 3)
	positive := doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, 3, 0, 50, 1, 244)
	cases := []struct {
		name string
		kind uint16
		body []byte
		want ProtocolErrorKind
	}{
		{"bad_ack_code", 0x8002, []byte{9}, ErrMalformedMessage},
		{"bad_nack_code", 0x8003, []byte{0}, ErrMalformedMessage},
		{"echo_unknown_service", 0x8002, []byte{0, 0x11, 3}, ErrUnsupportedFeature},
		{"echo_zero_session", 0x8002, []byte{0, 0x10, 0}, ErrMalformedMessage},
		{"positive_short", 0x8001, []byte{0x50, 3, 0, 50, 1}, ErrMalformedMessage},
		{"negative_short", 0x8001, []byte{0x7f, 0x10}, ErrMalformedMessage},
		{"negative_zero", 0x8001, []byte{0x7f, 0x10, 0}, ErrMalformedMessage},
		{"unknown_service", 0x8001, []byte{0x51, 3}, ErrUnsupportedFeature},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := doipPublicActive(t)
			defer s.Close("test")
			require.Nil(t, doipPublicFeed(t, s, 0, rq).Err)
			require.Nil(t, doipPublicFeed(t, s, 1, positive).Err)
			require.Nil(t, doipPublicFeed(t, s, 0, rq).Err)
			bad := doipPublicFeed(t, s, 1, doipTestWire(c.kind, append([]byte{0x40, 0x10, 0x0e, 0x80}, c.body...)...))
			require.NotNil(t, bad.Err)
			require.Equal(t, c.want, bad.Err.Kind, "malformed/unsupported wire must not be converted into recoverable association ambiguity")
			require.Nil(t, bad.Events[0].Session)
			require.Zero(t, bad.Events[0].ResponseTo)
			f, err := bad.Events[0].GetFields()
			require.Nil(t, f)
			rocTypedError(t, string(c.want), err)
			s.Close("test")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("adjacent_valid_reused_reply", func(t *testing.T) {
		s := doipPublicActive(t)
		defer s.Close("test")
		require.Nil(t, doipPublicFeed(t, s, 0, rq).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, positive).Err)
		require.Nil(t, doipPublicFeed(t, s, 0, rq).Err)
		r := doipPublicFeed(t, s, 1, positive)
		require.Equal(t, ErrContextRequired, r.Err.Kind)
		require.Zero(t, r.Events[0].ResponseTo)
	})
}

func TestDoIPContextDistinctRequestAfterUncertainReply(t *testing.T) {
	rq := func(v byte) []byte { return doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, v) }
	positive := func(v byte) []byte { return doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, v, 0, 50, 1, 244) }
	s := doipPublicActive(t)
	defer s.Close("test")
	require.Nil(t, doipPublicFeed(t, s, 0, rq(2)).Err)
	require.Nil(t, doipPublicFeed(t, s, 1, positive(2)).Err)
	previous := doipPublicFeed(t, s, 0, rq(2))
	require.Nil(t, previous.Err)
	refused := doipPublicFeed(t, s, 1, positive(2))
	require.Equal(t, ErrContextRequired, refused.Err.Kind)
	require.Zero(t, refused.Events[0].ResponseTo)
	next := doipPublicFeed(t, s, 0, rq(3))
	require.Nil(t, next.Err, "a distinct valid observed request after an unassignable response must remain visible without claiming old completion")
	require.Equal(t, true, next.Events[0].Session["Previous Diagnostic Completion Unverified"])
	require.Equal(t, previous.Events[0].ID, next.Events[0].Session["Unresolved Previous Request Event"])
	require.Equal(t, 2, next.Events[0].Session["Unresolved Previous Session Type"])
	require.Zero(t, next.Events[0].ResponseTo)
	reply := doipPublicFeed(t, s, 1, positive(3))
	require.Nil(t, reply.Err)
	require.Equal(t, next.Events[0].ID, reply.Events[0].ResponseTo)
	require.Equal(t, 3, reply.Events[0].Session["Session Type"])
	require.Empty(t, s.Close("test"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDoIPContextLateRetiredDescriptorPreservesFreshRequest(t *testing.T) {
	rq := func(v byte) []byte { return doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, v) }
	positive := func(v byte) []byte { return doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, v, 0, 50, 1, 244) }
	s := doipPublicActive(t)
	defer s.Close("test")
	require.Nil(t, doipPublicFeed(t, s, 0, rq(2)).Err)
	require.Nil(t, doipPublicFeed(t, s, 1, positive(2)).Err)
	require.Nil(t, doipPublicFeed(t, s, 0, rq(2)).Err)
	r := doipPublicFeed(t, s, 1, positive(2))
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	fresh := doipPublicFeed(t, s, 0, rq(3))
	require.Nil(t, fresh.Err)
	late := doipPublicFeed(t, s, 1, positive(2))
	require.NotNil(t, late.Err)
	require.Equal(t, ErrContextRequired, late.Err.Kind)
	require.Zero(t, late.Events[0].ResponseTo)
	require.Nil(t, late.Events[0].Session)
	current := doipPublicFeed(t, s, 1, positive(3))
	require.Nil(t, current.Err)
	require.Equal(t, fresh.Events[0].ID, current.Events[0].ResponseTo)
	require.Empty(t, s.Close("test"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDoIPContextContinuationBudgetAndOwnership(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxCollectionElements = 2
	s, err := NewProtocolSession(b)
	require.NoError(t, err)
	defer s.Close("test")
	require.Nil(t, doipPublicFeed(t, s, 0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0)).Err)
	require.Nil(t, doipPublicFeed(t, s, 1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x10, 0, 0, 0, 0)).Err)
	rq := func(v byte) []byte { return doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, v) }
	pos := func(v byte) []byte { return doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, v, 0, 50, 1, 244) }
	require.Nil(t, doipPublicFeed(t, s, 0, rq(2)).Err)
	require.Nil(t, doipPublicFeed(t, s, 1, pos(2)).Err)
	old := doipPublicFeed(t, s, 0, rq(2))
	r := doipPublicFeed(t, s, 1, pos(2))
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	fresh := doipPublicFeed(t, s, 0, rq(3))
	require.Nil(t, fresh.Err)
	fields, err := fresh.Events[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, old.Events[0].ID, fields["Unresolved Previous Request Event"])
	clear(fields)
	clear(fresh.Events[0].Session)
	clear(fresh.Events[0].Raw)
	fields, err = fresh.Events[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, old.Events[0].ID, fields["Unresolved Previous Request Event"])
	r = doipPublicFeed(t, s, 1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x7f, 0x10, 0x22))
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	r = doipPublicFeed(t, s, 0, rq(4))
	require.Equal(t, ErrResourceExceeded, r.Err.Kind, "uncertainty cannot bypass the retained identity budget")
	s.Close("test")
	require.Zero(t, s.Stats().BufferedBytes)
	require.Empty(t, s.Close("again"))
}

func doipContextFile(t *testing.T, prefix, name string) []byte {
	t.Helper()
	env := "DOIP_CONTEXT_TEST_ROOT"
	if prefix == "doip-context-echo" {
		env = "DOIP_ECHO_TEST_ROOT"
	}
	if prefix == "doip-context-late" {
		env = "DOIP_LATE_TEST_ROOT"
	}
	if root := os.Getenv(env); root != "" {
		b, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile(prefix + "/" + name)
	require.NoError(t, err)
	return b
}
func TestDoIPContextSealedMatrix(t *testing.T) { runDoIPContextMatrix(t, "doip-context", 48) }
func TestDoIPContextEchoHighBitSealedMatrix(t *testing.T) {
	runDoIPContextMatrix(t, "doip-context-echo", 9)
}
func runDoIPContextMatrix(t *testing.T, prefix string, count int) {
	var m struct {
		Cases []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(doipContextFile(t, prefix, "manifest.json"), &m))
	require.Len(t, m.Cases, count)
	var frozen map[string]trafficfixture.FrozenCase
	if os.Getenv("DOIP_CONTEXT_TEST_ROOT") == "" && os.Getenv("DOIP_ECHO_TEST_ROOT") == "" && os.Getenv("DOIP_LATE_TEST_ROOT") == "" {
		frozen = map[string]trafficfixture.FrozenCase{}
		all, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		for _, b := range all {
			for _, c := range b.Cases {
				frozen[c.ID] = c
			}
		}
	}
	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			if frozen != nil {
				v, ok := frozen[prefix+"/"+c.ID]
				require.True(t, ok)
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, prefix+"/"+c.Capture, v.Input.File)
				require.Equal(t, c.Packets, v.Facts.PacketCount)
				require.Len(t, v.Expectations, 1)
				var bind struct {
					Answer string `json:"answer_file"`
					SHA    string `json:"answer_sha256"`
				}
				require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &bind))
				require.Equal(t, c.Answer, bind.Answer)
				require.Equal(t, c.AnswerSHA, bind.SHA)
			}
			cap, answer := doipContextFile(t, prefix, c.Capture), doipContextFile(t, prefix, c.Answer)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(cap)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var a struct {
				Events []struct {
					ID               int
					Wire, Error, Why string
					Fields           map[string]any
					Refs             []uint64
					Direction        int
					ResponseTo       uint64 `json:"response_to"`
					Close            bool
				}
				PacketCount int `json:"packet_count"`
			}
			require.NoError(t, decodeExactProtocolAnswer(answer, &a))
			reader, err := pcapgo.NewReader(bytes.NewReader(cap))
			require.NoError(t, err)
			var packets [][]byte
			for {
				b, _, e := reader.ReadPacketData()
				if e == io.EOF {
					break
				}
				require.NoError(t, e)
				packets = append(packets, bytes.Clone(b))
			}
			require.Len(t, packets, c.Packets)
			for _, w := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", w, deferred, observe), func(t *testing.T) {
							var es []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(w), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
									require.Less(t, seen, len(packets))
									require.Equal(t, packets[seen], p.Data())
									seen++
								}))
							}
							input := bytes.Clone(cap)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, es, len(a.Events))
							require.Zero(t, stats.BufferedBytes)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
							for i, want := range a.Events {
								e := es[i]
								require.EqualValues(t, want.ID, e.ID)
								require.Equal(t, want.Direction, e.Direction)
								require.EqualValues(t, 1, e.FlowID)
								require.Zero(t, e.TransactionID)
								require.Equal(t, want.ResponseTo, e.ResponseTo)
								require.Equal(t, "doip", e.Protocol)
								require.Equal(t, "doip-routing-dsc", e.Profile)
								require.Len(t, e.SourceBytes.PacketRefs, len(want.Refs))
								for j, n := range want.Refs {
									require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
								}
								raw, err := hex.DecodeString(want.Wire)
								require.NoError(t, err)
								if want.Close {
									require.Nil(t, e.Raw)
								} else {
									require.Equal(t, raw, e.Raw)
								}
								if want.Error != "" {
									rocTypedError(t, want.Error, e.sessionError)
									require.Contains(t, e.Error, want.Why)
									if want.Close {
										rocEqualFields(t, want.Fields, e.Session)
									} else {
										require.Nil(t, e.Session)
									}
									e.Protocol, e.Error, e.Status = "http", "", "decoded"
									e.Session = map[string]any{"forged": true}
									e.Fields = e.Session
									for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										f, err := fn()
										require.Nil(t, f)
										rocTypedError(t, want.Error, err)
									}
									continue
								}
								require.Nil(t, e.sessionError)
								require.Empty(t, e.Error)
								rocEqualFields(t, want.Fields, e.Session)
								for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
									f, err := fn()
									require.NoError(t, err)
									rocEqualFields(t, want.Fields, protocolFields(f))
									clear(f)
								}
								clear(e.Raw)
								clear(e.Session)
								e.Protocol = "http"
								f, err := e.GetFields()
								require.NoError(t, err)
								rocEqualFields(t, want.Fields, protocolFields(f))
							}
						})
					}
				}
			}
		})
	}
}

func TestDoIPContextRetiredEchoHighBitCannotPanic(t *testing.T) {
	for _, echo := range []byte{0x80, 0x83, 0xff} {
		t.Run(fmt.Sprintf("echo=%02x", echo), func(t *testing.T) {
			s := doipPublicActive(t)
			defer s.Close("test")
			rq := func(v byte) []byte { return doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, v) }
			pos := func(v byte) []byte { return doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, v, 0, 50, 1, 244) }
			require.Nil(t, doipPublicFeed(t, s, 0, rq(2)).Err)
			require.Nil(t, doipPublicFeed(t, s, 1, pos(2)).Err)
			require.Nil(t, doipPublicFeed(t, s, 0, rq(2)).Err)
			r := doipPublicFeed(t, s, 1, pos(2))
			require.Equal(t, ErrContextRequired, r.Err.Kind)
			require.Nil(t, doipPublicFeed(t, s, 0, rq(3)).Err)
			require.NotPanics(t, func() {
				r = s.Feed(1, time.Unix(1700001000, 0), doipTestWire(0x8002, 0x40, 0x10, 0x0e, 0x80, 0, 0x10, echo))
				require.Zero(t, s.Stats().CallbackPanics, "wire validation must not enter panic recovery")
				require.Len(t, r.Events, 1)
				require.NotNil(t, r.Err)
				want := ErrUnsupportedFeature
				if echo == 0x80 {
					want = ErrMalformedMessage
				}
				require.Equal(t, want, r.Err.Kind)
				require.Zero(t, r.Events[0].ResponseTo)
				require.Nil(t, r.Events[0].Session)
			}, "untrusted echoed subfunction cannot index outside the 128-bit history")
			s.Close("test")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

// Passive late complete known wire cannot certify completion or consume future eligibility.
func TestDoIPLateCompletedContextBoundary(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprintf("late=%t", late), func(t *testing.T) {
			s := doipPublicActive(t)
			defer s.Close("test")
			rq := func(v byte) []byte { return doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, v) }
			rp := func(v byte) []byte { return doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, v, 0, 50, 1, 244) }
			first := doipPublicFeed(t, s, 0, rq(2))
			require.Nil(t, first.Err)
			done := doipPublicFeed(t, s, 1, rp(2))
			require.Nil(t, done.Err)
			require.Equal(t, first.Events[0].ID, done.Events[0].ResponseTo)
			if late {
				x := doipPublicFeed(t, s, 1, rp(2))
				require.NotNil(t, x.Err)
				require.Equal(t, ErrContextRequired, x.Err.Kind, "complete known late bytes lack active request context; do not misclassify wire")
				require.Zero(t, x.Events[0].ResponseTo)
				require.Nil(t, x.Events[0].Session)
				f, e := x.Events[0].GetFields()
				require.Nil(t, f)
				rocTypedError(t, string(ErrContextRequired), e)
			}
			next := doipPublicFeed(t, s, 0, rq(3))
			require.Nil(t, next.Err)
			done = doipPublicFeed(t, s, 1, rp(3))
			require.Nil(t, done.Err)
			require.Equal(t, next.Events[0].ID, done.Events[0].ResponseTo)
			require.Empty(t, s.Close("test"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDoIPLateCompletedWireAndContextBoundaries(t *testing.T) {
	cases := []struct {
		name           string
		kind           uint16
		source, target uint16
		dir            int
		body           []byte
		want           ProtocolErrorKind
		recoverable    bool
	}{
		{"known-positive", 0x8001, 0x4010, 0x0e80, 1, []byte{0x50, 2, 0, 50, 1, 244}, ErrContextRequired, true},
		{"known-ack-echo", 0x8002, 0x4010, 0x0e80, 1, []byte{0, 0x10, 2}, ErrContextRequired, true},
		{"known-nack-echo", 0x8003, 0x4010, 0x0e80, 1, []byte{6, 0x10, 2}, ErrContextRequired, true},
		{"known-short-ack", 0x8002, 0x4010, 0x0e80, 1, []byte{0}, ErrContextRequired, true},
		{"known-service-nrc", 0x8001, 0x4010, 0x0e80, 1, []byte{0x7f, 0x10, 0x78}, ErrContextRequired, true},
		{"malformed-positive", 0x8001, 0x4010, 0x0e80, 1, []byte{0x50, 2, 0, 50, 1}, ErrMalformedMessage, false},
		{"invalid-ack-code", 0x8002, 0x4010, 0x0e80, 1, []byte{9}, ErrMalformedMessage, false},
		{"suppressed-echo", 0x8002, 0x4010, 0x0e80, 1, []byte{0, 0x10, 0x82}, ErrUnsupportedFeature, false},
		{"unknown-service", 0x8001, 0x4010, 0x0e80, 1, []byte{0x51, 2}, ErrUnsupportedFeature, false},
		{"unknown-descriptor", 0x8001, 0x4010, 0x0e80, 1, []byte{0x50, 4, 0, 50, 1, 244}, ErrMalformedMessage, false},
		{"unknown-source", 0x8001, 0x4020, 0x0e80, 1, []byte{0x50, 2, 0, 50, 1, 244}, ErrMalformedMessage, false},
		{"wrong-target", 0x8001, 0x4010, 0x0e81, 1, []byte{0x50, 2, 0, 50, 1, 244}, ErrMalformedMessage, false},
		{"wrong-direction", 0x8001, 0x4010, 0x0e80, 0, []byte{0x50, 2, 0, 50, 1, 244}, ErrMalformedMessage, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := doipPublicActive(t)
			defer s.Close("test")
			rq := func(v byte) []byte { return doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, v) }
			rp := func(v byte) []byte { return doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, v, 0, 50, 1, 244) }
			first := doipPublicFeed(t, s, 0, rq(2))
			require.Nil(t, first.Err)
			done := doipPublicFeed(t, s, 1, rp(2))
			require.Nil(t, done.Err)
			require.Equal(t, first.Events[0].ID, done.Events[0].ResponseTo)
			body := append([]byte{byte(c.source >> 8), byte(c.source), byte(c.target >> 8), byte(c.target)}, c.body...)
			x := doipPublicFeed(t, s, c.dir, doipTestWire(c.kind, body...))
			require.NotNil(t, x.Err)
			require.Equal(t, c.want, x.Err.Kind)
			require.Zero(t, x.Events[0].ResponseTo)
			require.Nil(t, x.Events[0].Session)
			require.Equal(t, c.dir, x.Events[0].Direction)
			f, e := x.Events[0].GetFields()
			require.Nil(t, f)
			rocTypedError(t, string(c.want), e)
			require.Zero(t, s.Stats().CallbackPanics)
			if c.recoverable {
				next := doipPublicFeed(t, s, 0, rq(3))
				require.Nil(t, next.Err)
				done = doipPublicFeed(t, s, 1, rp(3))
				require.Nil(t, done.Err)
				require.Equal(t, next.Events[0].ID, done.Events[0].ResponseTo)
				require.Equal(t, 3, done.Events[0].Session["Session Type"])
			}
			require.Empty(t, s.Close("test"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDoIPLateCompletedContextRetainsIdentityBudget(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxCollectionElements = 1
	s, err := NewProtocolSession(b)
	require.NoError(t, err)
	defer s.Close("test")
	for _, a := range []struct {
		d int
		b []byte
	}{{0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0)}, {1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x10, 0, 0, 0, 0)}} {
		require.Nil(t, doipPublicFeed(t, s, a.d, a.b).Err)
	}
	req := func(v byte) []byte { return doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, v) }
	positive := doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, 2, 0, 50, 1, 244)
	r := doipPublicFeed(t, s, 0, req(2))
	require.Nil(t, r.Err)
	done := doipPublicFeed(t, s, 1, positive)
	require.Nil(t, done.Err)
	require.Equal(t, r.Events[0].ID, done.Events[0].ResponseTo)
	late := doipPublicFeed(t, s, 1, positive)
	require.Equal(t, ErrContextRequired, late.Err.Kind)
	require.Zero(t, late.Events[0].ResponseTo)
	rejected := doipPublicFeed(t, s, 0, req(3))
	require.Equal(t, ErrResourceExceeded, rejected.Err.Kind)
	require.Zero(t, rejected.Events[0].ResponseTo)
	require.Nil(t, rejected.Events[0].Session)
	require.Empty(t, s.Close("test"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDoIPLateCompletedSealedMatrix(t *testing.T) {
	runDoIPContextMatrix(t, "doip-context-late", 42)
}
