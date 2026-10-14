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

func doipPublicFeed(t *testing.T, s ProtocolSession, dir int, w []byte) FeedResult {
	t.Helper()
	r := s.Feed(dir, time.Unix(1700001000, 0), w)
	require.Equal(t, len(w), r.Consumed)
	require.Len(t, r.Events, 1)
	return r
}
func doipPublicActive(t *testing.T) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r := doipPublicFeed(t, s, 0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0))
	require.Nil(t, r.Err)
	r = doipPublicFeed(t, s, 1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x10, 0, 0, 0, 0))
	require.Nil(t, r.Err)
	return s
}
func TestDoIPGenerationPublicRegression(t *testing.T) {
	rq := func(st byte) []byte { return doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, st) }
	ack := func(st byte) []byte { return doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, st, 0, 50, 1, 244) }
	nrc := doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x7f, 0x10, 0x22)
	t.Run("AdjacentFirstGeneration", func(t *testing.T) {
		s := doipPublicActive(t)
		defer s.Close("test")
		require.Nil(t, doipPublicFeed(t, s, 0, rq(3)).Err)
		r := doipPublicFeed(t, s, 1, nrc)
		require.Nil(t, r.Err)
		require.Equal(t, "matched", r.Events[0].Session["Association"])
		require.Equal(t, 3, r.Events[0].Session["Session Type"])
		require.Empty(t, s.Close("test"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("LateNRCCannotChooseNewType", func(t *testing.T) {
		s := doipPublicActive(t)
		defer s.Close("test")
		require.Nil(t, doipPublicFeed(t, s, 0, rq(3)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, ack(3)).Err)
		require.Nil(t, doipPublicFeed(t, s, 0, rq(2)).Err)
		r := doipPublicFeed(t, s, 1, nrc)
		require.NotNil(t, r.Err, "service-only NRC cannot choose a DSC generation")
		if r.Err != nil {
			require.Equal(t, ErrContextRequired, r.Err.Kind)
		}
		require.Nil(t, r.Events[0].Session)
		require.Zero(t, r.Events[0].ResponseTo)
	})
	t.Run("ReusedTypeResponseCannotClaimGeneration", func(t *testing.T) {
		s := doipPublicActive(t)
		defer s.Close("test")
		require.Nil(t, doipPublicFeed(t, s, 0, rq(3)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, ack(3)).Err)
		r := doipPublicFeed(t, s, 0, rq(3))
		require.Nil(t, r.Err, "repeated request wire observation remains supported")
		r = doipPublicFeed(t, s, 1, ack(3))
		require.NotNil(t, r.Err, "identical reply has no generation identity")
		if r.Err != nil {
			require.Equal(t, ErrContextRequired, r.Err.Kind)
		}
		require.Nil(t, r.Events[0].Session)
		require.Zero(t, r.Events[0].ResponseTo)
	})
	t.Run("UnprefixedACKCannotChooseNewGeneration", func(t *testing.T) {
		s := doipPublicActive(t)
		defer s.Close("test")
		require.Nil(t, doipPublicFeed(t, s, 0, rq(3)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, ack(3)).Err)
		require.Nil(t, doipPublicFeed(t, s, 0, rq(2)).Err)
		r := doipPublicFeed(t, s, 1, doipTestWire(0x8002, 0x40, 0x10, 0x0e, 0x80, 0))
		require.NotNil(t, r.Err, "ACK omits DSC subfunction and cannot choose generation")
		if r.Err != nil {
			require.Equal(t, ErrContextRequired, r.Err.Kind)
		}
	})
	t.Run("ObservedResponseHasActualRequestEvent", func(t *testing.T) {
		s := doipPublicActive(t)
		defer s.Close("test")
		req := doipPublicFeed(t, s, 0, rq(3))
		r := doipPublicFeed(t, s, 1, ack(3))
		require.Nil(t, r.Err)
		require.NotZero(t, req.Events[0].ID)
		require.Equal(t, req.Events[0].ID, r.Events[0].ResponseTo)
	})
}

func TestDoIPGenerationPublicLifecycle(t *testing.T) {
	rq := func(target uint16, st byte) []byte {
		return doipTestWire(0x8001, 0x0e, 0x80, byte(target>>8), byte(target), 0x10, st)
	}
	ack := func(target uint16, st byte) []byte {
		return doipTestWire(0x8001, byte(target>>8), byte(target), 0x0e, 0x80, 0x50, st, 0, 50, 1, 244)
	}
	t.Run("DescriptorACKAndPositiveSurviveAmbiguousNRC", func(t *testing.T) {
		s := doipPublicActive(t)
		require.Nil(t, doipPublicFeed(t, s, 0, rq(0x4010, 3)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, ack(0x4010, 3)).Err)
		req := doipPublicFeed(t, s, 0, rq(0x4010, 2))
		require.Nil(t, req.Err)
		bad := doipPublicFeed(t, s, 1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x7f, 0x10, 0x22))
		require.Equal(t, ErrContextRequired, bad.Err.Kind)
		require.Zero(t, bad.Events[0].ResponseTo)
		r := doipPublicFeed(t, s, 1, doipTestWire(0x8002, 0x40, 0x10, 0x0e, 0x80, 0, 0x10, 2))
		require.Nil(t, r.Err)
		require.Equal(t, req.Events[0].ID, r.Events[0].ResponseTo)
		r = doipPublicFeed(t, s, 1, ack(0x4010, 2))
		require.Nil(t, r.Err)
		require.Equal(t, req.Events[0].ID, r.Events[0].ResponseTo)
		require.Empty(t, s.Close("test"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("DifferentLogicalTargetHasSeparateHistory", func(t *testing.T) {
		s := doipPublicActive(t)
		require.Nil(t, doipPublicFeed(t, s, 0, rq(0x4010, 3)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, ack(0x4010, 3)).Err)
		req := doipPublicFeed(t, s, 0, rq(0x4011, 3))
		require.Nil(t, req.Err)
		r := doipPublicFeed(t, s, 1, doipTestWire(0x8001, 0x40, 0x11, 0x0e, 0x80, 0x7f, 0x10, 0x22))
		require.Nil(t, r.Err)
		require.Equal(t, req.Events[0].ID, r.Events[0].ResponseTo)
		require.Empty(t, s.Close("test"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("IdentityLimitRetiresLateResponse", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxCollectionElements = 1
		s, err := NewProtocolSession(b)
		require.NoError(t, err)
		require.Nil(t, doipPublicFeed(t, s, 0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x10, 0, 0, 0, 0)).Err)
		require.Nil(t, doipPublicFeed(t, s, 0, rq(0x4010, 3)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, ack(0x4010, 3)).Err)
		r := doipPublicFeed(t, s, 0, rq(0x4010, 2))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		r = s.Feed(1, time.Unix(1700001000, 0), ack(0x4010, 2))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrFatalSessionError, r.Err.Kind)
		require.Empty(t, r.Events, "resource exhaustion closes eligibility, not a successful late observation")
		require.Empty(t, s.Close("test"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("MemoryReservationBeforeHistoryAllocation", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxBufferedBytes = 370
		b.MaxMessageBytes = 128
		s, err := NewProtocolSession(b)
		require.NoError(t, err)
		require.Nil(t, doipPublicFeed(t, s, 0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x10, 0, 0, 0, 0)).Err)
		r := doipPublicFeed(t, s, 0, rq(0x4010, 3))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		r = s.Feed(1, time.Unix(1700001000, 0), ack(0x4010, 3))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrFatalSessionError, r.Err.Kind)
		require.Empty(t, r.Events)
		require.Empty(t, s.Close("test"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("RejectedOversizeRequestClearsEarlierEligibility", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxMessageBytes = 64
		s, err := NewProtocolSession(b)
		require.NoError(t, err)
		require.Nil(t, doipPublicFeed(t, s, 0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0)).Err)
		require.Nil(t, doipPublicFeed(t, s, 1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x10, 0, 0, 0, 0)).Err)
		require.Nil(t, doipPublicFeed(t, s, 0, rq(0x4010, 3)).Err)
		r := doipPublicFeed(t, s, 0, []byte{2, 0xfd, 0x80, 1, 0, 0, 4, 0})
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		r = s.Feed(1, time.Unix(1700001000, 0), ack(0x4010, 3))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrFatalSessionError, r.Err.Kind)
		require.Empty(t, r.Events)
		require.Empty(t, s.Close("test"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("RoutingAliveAndDiagnosticUseActualEventIDs", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		req := doipPublicFeed(t, s, 0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0))
		r := doipPublicFeed(t, s, 1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x10, 0, 0, 0, 0))
		require.Equal(t, req.Events[0].ID, r.Events[0].ResponseTo)
		req = doipPublicFeed(t, s, 1, doipTestWire(7))
		r = doipPublicFeed(t, s, 0, doipTestWire(8, 0x0e, 0x80))
		require.Equal(t, req.Events[0].ID, r.Events[0].ResponseTo)
		require.Zero(t, r.Events[0].TransactionID)
		require.Empty(t, s.Close("test"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func TestDoIPGenerationCloseOwnedError(t *testing.T) {
	s := doipPublicActive(t)
	req := doipPublicFeed(t, s, 0, doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, 3))
	require.Nil(t, req.Err)
	es := s.Close("test")
	require.Len(t, es, 1)
	e := es[0]
	require.Equal(t, "doip-routing-dsc", e.Profile)
	rocTypedError(t, "NeedMore", e.sessionError)
	require.Equal(t, 1, e.Session["Outstanding"])
	require.Zero(t, e.ResponseTo)
	e.Protocol, e.Error, e.Status = "http", "", "decoded"
	e.Session = map[string]any{"forged": true}
	for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
		f, err := fn()
		require.Nil(t, f)
		rocTypedError(t, "NeedMore", err)
	}
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func doipGenerationFile(t *testing.T, n string) []byte {
	t.Helper()
	if root := os.Getenv("DOIP_GENERATION_TEST_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, n))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile("doip-generation/" + n)
	require.NoError(t, err)
	return b
}
func TestDoIPGenerationSealedMatrix(t *testing.T) {
	var m struct {
		Cases []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(doipGenerationFile(t, "manifest.json"), &m))
	require.Len(t, m.Cases, 36)
	var frozen map[string]trafficfixture.FrozenCase
	if os.Getenv("DOIP_GENERATION_TEST_ROOT") == "" {
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
				v, ok := frozen["doip-generation/"+c.ID]
				require.True(t, ok)
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, "doip-generation/"+c.Capture, v.Input.File)
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
			cap, answer := doipGenerationFile(t, c.Capture), doipGenerationFile(t, c.Answer)
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
