package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func wcfUpgradeRequestTestWire() []byte {
	protocol := []byte("application/ssl-tls")
	wire := append([]byte{wcfRecordUpgradeRequest}, wcfTestSize(len(protocol))...)
	return append(wire, protocol...)
}

func TestWCFUpgradeAndPreambleRecordDirections(t *testing.T) {
	for _, initiator := range []int{0, 1} {
		for _, record := range []struct {
			name string
			wire []byte
		}{
			{"upgrade-request", wcfUpgradeRequestTestWire()},
			{"mode", []byte{wcfRecordMode, 2}},
			{"encoding", []byte{wcfRecordKnownEncoding, 3}},
		} {
			t.Run(fmt.Sprintf("initiator%d/receiver-%s", initiator, record.name), func(t *testing.T) {
				s := newWCFTestSession(t, DefaultParserBudget())
				defer s.Close("FIN")
				preamble := wcfTestPreamble(2, 3)
				first := s.Feed(initiator, time.Unix(1, 0), preamble[:len(preamble)-1])
				require.Nil(t, first.Err)
				result := s.Feed(1-initiator, time.Unix(2, 0), record.wire)
				require.NotNil(t, result.Err)
				require.Equal(t, ErrMalformedMessage, result.Err.Kind)
				require.Contains(t, result.Err.Error(), "receiver")
			})
		}
		for _, wrong := range []bool{false, true} {
			t.Run(fmt.Sprintf("initiator%d/response-wrong%v", initiator, wrong), func(t *testing.T) {
				s := newWCFTestSession(t, DefaultParserBudget())
				defer s.Close("FIN")
				preamble := wcfTestPreamble(2, 3)
				preamble = append(bytes.Clone(preamble[:len(preamble)-1]), wcfUpgradeRequestTestWire()...)
				require.Nil(t, s.Feed(initiator, time.Unix(1, 0), preamble).Err)
				dir := 1 - initiator
				if wrong {
					dir = initiator
				}
				result := s.Feed(dir, time.Unix(2, 0), []byte{wcfRecordUpgradeResponse})
				if wrong {
					require.NotNil(t, result.Err)
					require.Equal(t, ErrMalformedMessage, result.Err.Kind)
					for _, e := range result.Events {
						require.NotEqual(t, true, e.Session["Upgrade Accepted"])
					}
					return
				}
				require.Nil(t, result.Err)
				require.Len(t, result.Events, 1)
				require.Equal(t, true, result.Events[0].Session["Upgrade Accepted"])
				require.Equal(t, "wcf->carried", result.Events[0].Session["Protocol Transition"])
			})
		}
	}
}

func TestWCFPooledLogicalSessionsKeepRecordAndContextBoundaries(t *testing.T) {
	for _, initiator := range []int{0, 1} {
		for _, width := range []int{1, 2, 7, 4096} {
			t.Run(fmt.Sprintf("initiator%d/chunk%d", initiator, width), func(t *testing.T) {
				s := newWCFTestSession(t, DefaultParserBudget())
				defer s.Close("FIN")
				var previous []*ProtocolEvent
				var immutable []byte
				for generation := uint64(1); generation <= 3; generation++ {
					var got []*ProtocolEvent
					feed := func(dir int, wire []byte) {
						for len(wire) > 0 {
							n := min(width, len(wire))
							r := s.Feed(dir, time.Unix(int64(generation), 0), wire[:n])
							if r.Err != nil {
								require.Equal(t, ErrNeedMore, r.Err.Kind)
								require.True(t, r.NeedMore)
							}
							got = append(got, r.Events...)
							wire = wire[n:]
						}
					}
					feed(initiator, wcfTestPreamble(2, byte(generation+2)))
					feed(1-initiator, []byte{wcfRecordPreambleAck})
					feed(initiator, append(wcfTestSized(wcfTextEnvelope()), wcfRecordEnd))
					feed(1-initiator, append(wcfTestSized(wcfTextEnvelope()), wcfRecordEnd))
					require.Len(t, got, 10)
					for _, e := range got {
						require.Equal(t, "decoded", e.Status)
						require.Equal(t, generation, e.Session["Observed Session Index"])
					}
					require.NotContains(t, got[0].Session, "Mode", "new Version cannot borrow the prior mode")
					require.NotContains(t, got[0].Session, "Encoding", "new Version cannot borrow the prior encoding")
					require.Equal(t, uint64(1), got[6].Session["Envelope Sequence"])
					require.Equal(t, uint64(2), got[8].Session["Envelope Sequence"])
					if generation == 1 {
						previous = got
						var err error
						immutable, err = json.Marshal(previous)
						require.NoError(t, err)
					} else {
						current, err := json.Marshal(previous)
						require.NoError(t, err)
						require.Equal(t, immutable, current)
					}
				}
			})
		}
	}
}

func TestWCFSessionReuseWaitsForObservedPeerEndAndPreamble(t *testing.T) {
	for _, order := range []string{"peer-end-late", "next-ack-first"} {
		t.Run(order, func(t *testing.T) {
			s := newWCFTestSession(t, DefaultParserBudget())
			defer s.Close("FIN")
			feed := func(dir int, wire []byte) []*ProtocolEvent {
				r := s.Feed(dir, time.Unix(2, 0), wire)
				if r.Err != nil {
					require.Equal(t, ErrNeedMore, r.Err.Kind)
					require.True(t, r.NeedMore)
				}
				return r.Events
			}
			feed(0, wcfTestPreamble(2, 3))
			feed(1, []byte{wcfRecordPreambleAck})
			feed(0, append(wcfTestSized(wcfTextEnvelope()), wcfRecordEnd))
			feed(1, wcfTestSized(wcfTextEnvelope()))
			var current []*ProtocolEvent
			if order == "peer-end-late" {
				require.Empty(t, feed(0, wcfTestPreamble(2, 8)))
				current = feed(1, []byte{wcfRecordEnd})
				require.Len(t, current, 6)
				require.Equal(t, uint64(1), current[0].Session["Observed Session Index"])
				require.Equal(t, uint64(2), current[1].Session["Observed Session Index"])
				current = feed(1, []byte{wcfRecordPreambleAck})
				require.Len(t, current, 1)
			} else {
				feed(1, []byte{wcfRecordEnd})
				require.Empty(t, feed(1, []byte{wcfRecordPreambleAck}))
				current = feed(0, wcfTestPreamble(2, 8))
				require.Len(t, current, 6)
				require.Equal(t, uint64(2), current[5].Session["Observed Session Index"])
			}
			for _, e := range current {
				require.Equal(t, "decoded", e.Status)
			}
			feed(0, append(wcfTestSized(wcfTextEnvelope()), wcfRecordEnd))
			feed(1, append(wcfTestSized(wcfTextEnvelope()), wcfRecordEnd))
		})
	}
}

func TestWCFSingletonEnvelopeLimitIsPerDirection(t *testing.T) {
	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	require.Nil(t, s.Feed(0, time.Unix(1, 0), wcfTestPreamble(1, 3)).Err)
	require.Nil(t, s.Feed(1, time.Unix(1, 0), []byte{wcfRecordPreambleAck}).Err)
	for _, dir := range []int{0, 1} {
		r := s.Feed(dir, time.Unix(2, 0), append(wcfTestUnsized([]byte("opaque owned SOAP")), wcfRecordEnd))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 2)
	}
}

func TestWCFPreambleSplitSizeRemainsUndetermined(t *testing.T) {
	via := []byte("net.tcp://example.test/" + string(bytes.Repeat([]byte("x"), 140)))
	wire := []byte{0, 1, 0, 1, 2, 2}
	wire = append(wire, wcfTestSize(len(via))...)
	wire = append(wire, via...)
	for _, length := range []int{6, 7, 8, len(wire) - 1} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			p := probeWCF(wire[:length], 4096)
			require.Equal(t, ProbeNeedMore, p.Verdict)
			require.Greater(t, p.NeedBytes, 0)
		})
	}
	require.Equal(t, ProbeAccept, probeWCF(wire, 4096).Verdict)
}

func TestWCFOriginalOneDirectionUpgradeVectorIsRefused(t *testing.T) {
	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	preamble := wcfTestPreamble(2, 3)
	wire := append(bytes.Clone(preamble[:len(preamble)-1]), wcfUpgradeRequestTestWire()...)
	wire = append(wire, wcfRecordPreambleEnd, wcfRecordUpgradeResponse)
	wire = append(wire, []byte("not-a-tls-record")...)
	r := s.Feed(0, time.Unix(3, 0), wire)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)
	for _, e := range r.Events {
		require.NotEqual(t, true, e.Session["Upgrade Accepted"])
	}
}

func TestWCFRecordAliasesDoNotBorrowObservedContext(t *testing.T) {
	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	wire := append(wcfTestPreamble(2, 3), wcfTestSized([]byte("owned opaque body"))...)
	result := s.Feed(0, time.Unix(1, 0), wire)
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 6)
	for _, e := range result.Events {
		fields := e.DisplayFieldValues()
		name := e.Session["Record Name"]
		for _, f := range fields.Values {
			switch f.Name {
			case "wcf.mode":
				require.Equal(t, "Mode", name)
				require.Equal(t, "pdu", f.Origin)
				require.NotNil(t, f.Range)
				require.Equal(t, &DisplayFieldRange{Offset: 1, Length: 1}, f.Range)
			case "wcf.via":
				require.Equal(t, "Via", name)
				require.NotNil(t, f.Range)
			case "wcf.encoding":
				require.Equal(t, "Known Encoding", name)
				require.NotNil(t, f.Range)
			case "wcf.payload_length":
				require.Equal(t, "Sized Envelope", name)
				require.Equal(t, "derived", f.Origin)
				require.Nil(t, f.Range)
			}
		}
	}
	envelope := result.Events[5].DisplayFieldValues()
	mode, contextSeen := false, false
	for _, f := range envelope.Values {
		if f.Name == "wcf.mode" {
			mode = true
		}
		if f.Name == "wcf.context.mode" {
			contextSeen = true
			require.Equal(t, uint64(2), f.Value)
			require.Equal(t, "context", f.Origin)
			require.Nil(t, f.Range)
		}
	}
	require.False(t, mode)
	require.True(t, contextSeen)
}

func TestWCFReuseCannotInventMissingPeerEnd(t *testing.T) {
	s := newWCFTestSession(t, DefaultParserBudget())
	require.Nil(t, s.Feed(0, time.Unix(1, 0), wcfTestPreamble(2, 3)).Err)
	require.Nil(t, s.Feed(1, time.Unix(1, 0), []byte{wcfRecordPreambleAck}).Err)
	require.Nil(t, s.Feed(0, time.Unix(1, 0), []byte{wcfRecordEnd}).Err)
	waiting := s.Feed(0, time.Unix(2, 0), wcfTestPreamble(2, 8))
	require.True(t, waiting.NeedMore)
	require.Empty(t, waiting.Events)
	closed := s.Close("FIN")
	require.NotEmpty(t, closed)
	for _, e := range closed {
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, uint64(1), e.Session["Observed Session Index"])
		require.Contains(t, e.Session, "Session Context Pending")
	}
}

func TestWCFPeerAckBeforeEitherEndWaitsForCompleteNewPreamble(t *testing.T) {
	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	require.Nil(t, s.Feed(0, time.Unix(1, 0), wcfTestPreamble(2, 3)).Err)
	require.Nil(t, s.Feed(1, time.Unix(1, 0), []byte{wcfRecordPreambleAck, wcfRecordEnd}).Err)
	wait := s.Feed(1, time.Unix(2, 0), []byte{wcfRecordPreambleAck})
	require.True(t, wait.NeedMore)
	require.Empty(t, wait.Events)
	r := s.Feed(0, time.Unix(3, 0), []byte{wcfRecordEnd})
	require.Len(t, r.Events, 1)
	require.Equal(t, uint64(1), r.Events[0].Session["Observed Session Index"])
	preamble := wcfTestPreamble(2, 8)
	partial := s.Feed(0, time.Unix(4, 0), preamble[:7])
	require.True(t, partial.NeedMore)
	require.Empty(t, partial.Events)
	ready := s.Feed(0, time.Unix(5, 0), preamble[7:])
	require.Nil(t, ready.Err)
	require.Len(t, ready.Events, 6)
	for _, e := range ready.Events {
		require.Equal(t, uint64(2), e.Session["Observed Session Index"])
	}
	ack := ready.Events[5]
	require.Equal(t, []byte{wcfRecordPreambleAck}, ack.Raw)
	require.Equal(t, ready.Events[4].ID, ack.Session["Context Ready After PDU"])
}

func TestWCFModeTerminalRulesAndSimplexReuse(t *testing.T) {
	for _, mode := range []byte{1, 2, 3} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			s := newWCFTestSession(t, DefaultParserBudget())
			defer s.Close("FIN")
			require.Nil(t, s.Feed(0, time.Unix(1, 0), wcfTestPreamble(mode, 3)).Err)
			if mode == 1 {
				r := s.Feed(0, time.Unix(2, 0), []byte{wcfRecordEnd})
				require.NotNil(t, r.Err)
				require.Equal(t, ErrMalformedMessage, r.Err.Kind)
				return
			}
			require.Nil(t, s.Feed(0, time.Unix(2, 0), []byte{wcfRecordEnd}).Err)
			r := s.Feed(0, time.Unix(3, 0), wcfTestPreamble(mode, 7))
			if mode == 2 {
				require.True(t, r.NeedMore)
				require.Empty(t, r.Events)
			} else {
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 5)
				require.Equal(t, uint64(2), r.Events[0].Session["Observed Session Index"])
			}
		})
	}
}

func TestWCFNativeObservationSurvivesPublicEditsButCannotBeForged(t *testing.T) {
	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	r := s.Feed(0, time.Unix(1, 0), append(wcfTestPreamble(2, 3), wcfTestSized([]byte("owned"))...))
	require.Nil(t, r.Err)
	e := r.Events[5]
	obs, ok := e.NativeWCFObservation()
	require.True(t, ok)
	require.True(t, e.NativeWCFWireMatches())
	fields, ok := e.NativeWCFFields()
	require.True(t, ok)
	fields["Mode"] = uint64(99)
	e.Protocol, e.Status, e.Summary, e.Session = "edited", "malformed", "edited", fields
	e.Fields = map[string]any{"Mode": 99}
	query, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, uint8(2), query["Mode"])
	sealed, _ := e.NativeWCFObservation()
	require.Equal(t, obs, sealed)
	own, _ := e.NativeWCFFields()
	require.Equal(t, uint8(2), own["Mode"])
	for _, f := range e.DisplayFieldValues().Values {
		if f.Name == "wcf.context.mode" {
			require.Equal(t, uint64(2), f.Value)
		}
	}
	e.Raw[0] ^= 1
	require.False(t, e.NativeWCFWireMatches())
	for _, f := range e.DisplayFieldValues().Values {
		if f.Origin == "pdu" {
			require.Nil(t, f.Range, "edited Raw cannot retain original literal field ranges")
		}
	}
	wire, err := json.Marshal(e)
	require.NoError(t, err)
	var dto ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &dto))
	_, ok = dto.NativeWCFObservation()
	require.False(t, ok)
	_, ok = dto.NativeWCFFields()
	require.False(t, ok)
	require.False(t, dto.NativeWCFWireMatches())
}

func TestWCFFaultAndCounterBoundariesCannotRestartSession(t *testing.T) {
	for _, kind := range []string{"fault", "counter"} {
		t.Run(kind, func(t *testing.T) {
			s := newWCFTestSession(t, DefaultParserBudget())
			defer s.Close("FIN")
			require.Nil(t, s.Feed(0, time.Unix(1, 0), wcfTestPreamble(2, 3)).Err)
			f := s.(*captureSession).f
			if kind == "fault" {
				require.Nil(t, s.Feed(1, time.Unix(2, 0), []byte{8, 1, 'x'}).Err)
				r := s.Feed(1, time.Unix(3, 0), []byte{11})
				require.NotNil(t, r.Err)
				require.Equal(t, ErrMalformedMessage, r.Err.Kind)
				for _, e := range r.Events {
					require.NotEqual(t, "decoded", e.Status)
				}
			} else {
				require.Nil(t, s.Feed(1, time.Unix(2, 0), []byte{11, 7}).Err)
				require.Nil(t, s.Feed(0, time.Unix(3, 0), []byte{7}).Err)
				f.wcf.observedIndex = ^uint64(0)
				old := f.wcf
				r := s.Feed(0, time.Unix(4, 0), wcfTestPreamble(2, 7))
				require.NotNil(t, r.Err)
				require.Equal(t, ErrResourceExceeded, r.Err.Kind)
				require.Equal(t, ^uint64(0), old.observedIndex)
				require.Nil(t, f.wcf, "fatal resource refusal releases connection state")
			}
		})
	}
}

func TestWCFPooledStateIsBoundedAndBudgetRefusalIsAtomic(t *testing.T) {
	s := newWCFTestSession(t, ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 4096, MaxBufferedBytes: 8192})
	f := s.(*captureSession).f
	for generation := uint64(1); generation <= 100; generation++ {
		for _, step := range []struct {
			dir  int
			wire []byte
		}{{0, wcfTestPreamble(2, 3)}, {1, []byte{11}}, {0, []byte{7}}, {1, []byte{7}}} {
			r := s.Feed(step.dir, time.Unix(int64(generation), 0), step.wire)
			require.Nil(t, r.Err)
			for _, e := range r.Events {
				require.Equal(t, generation, e.Session["Observed Session Index"])
			}
		}
		require.LessOrEqual(t, f.sessionBytes, int64(2048))
	}
	old := f.wcf
	index := old.observedIndex
	// Pooled traffic reuses its existing high-water reservation. Exercise a
	// refused replacement with a separate empty reservation, rather than
	// pretending that reducing a limit revokes memory already reserved.
	tight := newWCFTestSession(t, DefaultParserBudget())
	tightFlow := tight.(*captureSession).f
	tightFlow.wcf = old
	tightFlow.a.config.MaxBufferedBytes = 64
	err := tightFlow.consumeWCF(0, &ProtocolEvent{Entry: "WCFSessionVersion", Raw: []byte{0, 1, 0}})
	require.Error(t, err)
	require.Contains(t, err.Error(), string(ErrResourceExceeded))
	require.Same(t, old, tightFlow.wcf)
	require.Equal(t, index, old.observedIndex, "refused replacement must not mutate the former session")
	tight.Close("FIN")
	require.Zero(t, tight.Stats().BufferedBytes)
	s.Close("FIN")
	require.Zero(t, s.Stats().BufferedBytes)
}

func FuzzWCFPooledSessionModel(f *testing.F) {
	for _, seed := range [][]byte{{0, 1, 2, 3}, {1, 0, 7, 31}, {0, 0, 255}, {1, 1, 0}, {0, 2, 3, 4, 5}, {1, 3, 2, 1, 0}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, choices []byte) {
		if len(choices) == 0 {
			return
		}
		s := newWCFTestSession(t, ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 4096, MaxBufferedBytes: 8192})
		defer s.Close("FIN")
		initiator := int(choices[0] & 1)
		count := 1 + len(choices)%5
		var retained []*ProtocolEvent
		var snapshot []byte
		for generation := 1; generation <= count; generation++ {
			choice := choices[(generation-1)%len(choices)]
			mode := byte(1 + choice%3)
			encoding := byte(choice % 9)
			envelope := wcfTestSized([]byte{0, choice, 255})
			if mode == 1 {
				envelope = wcfTestUnsized([]byte{0, choice}, []byte{255})
			}
			steps := []struct {
				dir  int
				wire []byte
			}{{initiator, wcfTestPreamble(mode, encoding)}}
			if mode != 3 {
				steps = append(steps, struct {
					dir  int
					wire []byte
				}{1 - initiator, []byte{11}})
			}
			steps = append(steps, struct {
				dir  int
				wire []byte
			}{initiator, append(envelope, 7)})
			if mode != 3 {
				steps = append(steps, struct {
					dir  int
					wire []byte
				}{1 - initiator, append(envelope, 7)})
			}
			var events []*ProtocolEvent
			for _, step := range steps {
				wire := step.wire
				for len(wire) > 0 {
					n := min(len(wire), 1+int(choice%17))
					r := s.Feed(step.dir, time.Unix(int64(generation), 0), wire[:n])
					if r.Err != nil {
						require.Equal(t, ErrNeedMore, r.Err.Kind)
					}
					events = append(events, r.Events...)
					wire = wire[n:]
				}
			}
			want := 10
			if mode == 3 {
				want = 7
			}
			require.Len(t, events, want)
			for _, e := range events {
				require.Equal(t, "decoded", e.Status)
				require.Equal(t, uint64(generation), e.Session["Observed Session Index"])
			}
			if retained == nil {
				retained = events
				snapshot, _ = json.Marshal(retained)
			} else {
				after, _ := json.Marshal(retained)
				require.Equal(t, snapshot, after)
			}
		}
	})
}

func TestWCFChunkAndEncodedSizeBoundsAreExplicitRefusals(t *testing.T) {
	for _, chunks := range []int{2, 3} {
		t.Run(fmt.Sprint(chunks), func(t *testing.T) {
			s := newWCFTestSession(t, ParserBudget{MaxCollectionElements: 2})
			defer s.Close("FIN")
			require.Nil(t, s.Feed(0, time.Unix(1, 0), wcfTestPreamble(1, 3)).Err)
			body := make([][]byte, chunks)
			for i := range body {
				body[i] = []byte{byte(i)}
			}
			r := s.Feed(0, time.Unix(2, 0), wcfTestUnsized(body...))
			if chunks == 2 {
				require.Nil(t, r.Err)
				require.Equal(t, uint64(2), r.Events[0].Session["Chunk Count"])
			} else {
				require.NotNil(t, r.Err)
				require.Equal(t, ErrResourceExceeded, r.Err.Kind)
				require.Equal(t, "limited", r.Events[0].Status)
			}
		})
	}
	_, _, _, err := wcfReadSize([]byte{0xff, 0xff, 0xff, 0xff, 8}, 0, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "31-bit")
	value, length, complete, err := wcfReadSize([]byte{0xff, 0xff, 0xff, 0xff, 7}, 0, 10)
	require.NoError(t, err)
	require.True(t, complete)
	require.Equal(t, uint64(0x7fffffff), value)
	require.Equal(t, 5, length)
}

func TestWCFReceiverEnvelopeRequiresObservedAck(t *testing.T) {
	for _, record := range [][]byte{wcfTestSized([]byte("opaque")), {wcfRecordEnd}} {
		s := newWCFTestSession(t, DefaultParserBudget())
		require.Nil(t, s.Feed(0, time.Unix(1, 0), wcfTestPreamble(2, 3)).Err)
		r := s.Feed(1, time.Unix(2, 0), record)
		require.NotNil(t, r.Err)
		require.Equal(t, ErrContextRequired, r.Err.Kind)
		require.Len(t, r.Events, 1)
		require.Equal(t, "context-required", r.Events[0].Status)
		require.NotContains(t, r.Events[0].Session, "Envelope Body SHA256")
		s.Close("FIN")
	}
}

func TestWCFContextLevelDescribesObservedPreambleState(t *testing.T) {
	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	r := s.Feed(0, time.Unix(1, 0), wcfTestPreamble(2, 3))
	require.Nil(t, r.Err)
	for _, e := range r.Events[:4] {
		require.Equal(t, "preamble-in-progress", e.Session["Context Level"])
	}
	require.Equal(t, "preamble-complete", r.Events[4].Session["Context Level"])
	require.Nil(t, s.Feed(1, time.Unix(2, 0), []byte{11, 7}).Err)
	require.Nil(t, s.Feed(0, time.Unix(3, 0), []byte{7}).Err)
	second := s.Feed(0, time.Unix(4, 0), wcfTestPreamble(2, 7))
	require.Nil(t, second.Err)
	require.Equal(t, uint64(2), second.Events[0].Session["Observed Session Index"])
	require.Equal(t, "preamble-in-progress", second.Events[0].Session["Context Level"])
	partial := s.Feed(0, time.Unix(5, 0), []byte{6, 4, 1})
	require.True(t, partial.NeedMore)
	closed := s.Close("FIN")
	require.Len(t, closed, 1)
	require.Equal(t, "preamble-complete", closed[0].Session["Context Level"])
	mid := newWCFTestSession(t, DefaultParserBudget())
	defer mid.Close("FIN")
	observed := mid.Feed(0, time.Unix(1, 0), wcfTestSized(wcfTextEnvelope()))
	require.Nil(t, observed.Err)
	require.Len(t, observed.Events, 1)
	require.Equal(t, "midstream", observed.Events[0].Session["Context Level"])
}
