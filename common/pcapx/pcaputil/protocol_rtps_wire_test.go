package pcaputil

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func rtpsShapeTestBinding() RTPSShapeWriter {
	return RTPSShapeWriter{[12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, [4]byte{0, 0, 2, 2}}
}

func newRTPSWireSession(t *testing.T, budget ParserBudget, selected bool) ProtocolSession {
	t.Helper()
	opts := []ProtocolSessionOption{WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSWire(28400)}
	if selected {
		opts = append(opts, WithSessionRTPSShapeWriter(28400, rtpsShapeTestBinding()))
	}
	s, err := NewProtocolSessionWithOptions(budget, opts...)
	require.NoError(t, err)
	return s
}

func TestRTPSWireSealedReplay(t *testing.T) {
	for _, c := range rtpsBatchControls(t, "rtps-wire", 236) {
		var opts []CaptureOption
		if strings.HasSuffix(c.ID, "-shape") {
			opts = append(opts, WithProtocolRTPSShapeWriter(28400, rtpsShapeTestBinding()))
		}
		testRTPSSealedReplayProfile(t, []rtpsControl{c}, "rtps-wire", opts...)
	}
}

func TestRTPSWireSealedSession(t *testing.T) {
	for _, c := range rtpsBatchControls(t, "rtps-wire", 236) {
		t.Run(c.ID, func(t *testing.T) {
			_, answer := rtpsInput(t, c)
			s := newRTPSWireSession(t, DefaultParserBudget(), strings.HasSuffix(c.ID, "-shape"))
			for i, encoded := range answer.Wires {
				wire, err := hex.DecodeString(encoded)
				require.NoError(t, err)
				if c.ID == "le-le-norm-shape" {
					fixed, err := hex.DecodeString("0001000005000000424c554500000000220000006400000018000000")
					require.NoError(t, err)
					require.Equal(t, fixed, wire[len(wire)-len(fixed):], "OMG2.5 section10.7 fixed BLUE sample")
				}
				before := s.Stats()
				for j := 0; j < 2; j++ {
					p := s.Probe(wire)
					verdict := ProbeAccept
					if answer.PureErrors[i] != "" {
						verdict = ProbeReject
					}
					require.Equal(t, verdict, p.Verdict)
					require.Equal(t, before, s.Stats())
				}
				input := bytes.Clone(wire)
				r := s.Feed(i%2, time.Unix(1700000000, int64(i)), input)
				clear(input)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				require.Equal(t, wire, e.Raw)
				require.Equal(t, "rtps", e.Protocol)
				require.Equal(t, "rtps-wire-native", e.Profile)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				f, err := e.GetFields()
				if answer.PureErrors[i] == "" {
					require.NoError(t, err)
					rocEqualFields(t, answer.Events[i].Fields, f)
					rocEqualFields(t, answer.Events[i].Fields, e.Session)
					mutateRTPSQuery(f)
					mutateRTPSQuery(e.Session)
					clear(e.Raw)
					testNativeObservationSessionQueries(t, e, answer.Events[i].Fields)
				} else {
					require.Nil(t, f)
					assertOwnedProtocolRefusal(t, e, answer.PureErrors[i], err)
				}
				require.Zero(t, s.Stats().BufferedBytes)
			}
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			r := s.Feed(0, time.Unix(1700000002, 0), []byte("late"))
			require.Empty(t, r.Events)
			require.Zero(t, r.Consumed)
			var pe *ProtocolError
			require.ErrorAs(t, r.Err, &pe)
			require.Equal(t, ErrFatalSessionError, pe.Kind)
		})
	}
}

func rtpsWireCase(t *testing.T, id string) ([]byte, rtpsControlAnswer) {
	t.Helper()
	for _, c := range rtpsBatchControls(t, "rtps-wire", 236) {
		if c.ID == id {
			_, a := rtpsInput(t, c)
			wire, err := hex.DecodeString(a.Wires[0])
			require.NoError(t, err)
			return wire, a
		}
	}
	t.Fatalf("missing independently constructed case %s", id)
	return nil, rtpsControlAnswer{}
}

func TestRTPSWireSharedResourcesRecovery(t *testing.T) {
	plain, plainAnswer := rtpsWireCase(t, "le-le-norm-shape")
	for _, tc := range []struct {
		id   string
		cost int
	}{
		{"le-offset-qos-internal-pad-shape", 8}, // DATA + PAD + two QoS parameters + four fields.
		{"le-bitmap-budget-shape", 13},          // Four submessages + two bitmap words + three offsets + four fields.
	} {
		wire, a := rtpsWireCase(t, tc.id)
		for _, limit := range []int{tc.cost - 1, tc.cost} {
			b := DefaultParserBudget()
			b.MaxCollectionElements = limit
			s := newRTPSWireSession(t, b, true)
			for i, w := range [][]byte{wire, plain} {
				before := s.Stats()
				p := s.Probe(w)
				require.Equal(t, before, s.Stats())
				r := s.Feed(i%2, time.Unix(1700000000, int64(i)), w)
				require.Len(t, r.Events, 1)
				f, err := r.Events[0].GetFields()
				if i == 0 && limit < tc.cost {
					require.Equal(t, ProbeReject, p.Verdict)
					require.Nil(t, f)
					assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
				} else {
					require.Equal(t, ProbeAccept, p.Verdict)
					require.NoError(t, err)
					want := plainAnswer.Events[0].Fields
					if i == 0 {
						want = a.Events[0].Fields
					}
					rocEqualFields(t, want, f)
					rocEqualFields(t, want, r.Events[0].Session)
				}
				require.Zero(t, r.Events[0].ResponseTo)
				require.Zero(t, r.Events[0].TransactionID)
				require.Zero(t, s.Stats().BufferedBytes)
			}
			require.Empty(t, s.Close("done"))
		}
	}
	for _, depth := range []int{3, 4, 5} {
		b := DefaultParserBudget()
		b.MaxRecursionDepth = depth
		s := newRTPSWireSession(t, b, true)
		qos, qa := rtpsWireCase(t, "le-offset-qos-internal-pad-shape")
		for i, w := range [][]byte{qos, plain} {
			r := s.Feed(i%2, time.Unix(1700000000, int64(i)), w)
			require.Len(t, r.Events, 1)
			f, err := r.Events[0].GetFields()
			if depth < 4 || i == 0 && depth < 5 {
				require.Nil(t, f)
				assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
			} else {
				require.NoError(t, err)
				want := plainAnswer.Events[0].Fields
				if i == 0 {
					want = qa.Events[0].Fields
				}
				rocEqualFields(t, want, f)
				rocEqualFields(t, want, r.Events[0].Session)
			}
		}
		require.Empty(t, s.Close("done"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
	b := DefaultParserBudget()
	b.MaxMessageBytes, b.MaxFrameBytes = 64, 64
	s := newRTPSWireSession(t, b, true)
	r := s.Feed(0, time.Unix(1700000000, 0), plain)
	require.Len(t, r.Events, 1)
	_, err := r.Events[0].GetFields()
	assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
	control, ca := rtpsControlCase(t, "v5-07-le")
	r = s.Feed(1, time.Unix(1700000001, 0), control)
	require.Len(t, r.Events, 1)
	f, err := r.Events[0].GetFields()
	require.NoError(t, err)
	want := cloneSession(ca.Events[0].Fields)
	want["Observation"] = "unverified-wire-message"
	want["Messages"] = want["Controls"]
	delete(want, "Controls")
	rocEqualFields(t, want, f)
	rocEqualFields(t, want, r.Events[0].Session)
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestRTPSWireSelectionAndDatagramBoundary(t *testing.T) {
	binding := rtpsShapeTestBinding()
	for _, opts := range [][]ProtocolSessionOption{
		{WithSessionTransport("tcp"), WithSessionRTPSWire(28400)},
		{WithSessionTransport("udp"), WithSessionRTPSWire(0)},
		{WithSessionTransport("udp"), WithSessionRTPSWire(28400), WithSessionRTPSControl(28400)},
		{WithSessionTransport("udp"), WithSessionRTPSShapeWriter(28400, RTPSShapeWriter{})},
		{WithSessionTransport("udp"), WithSessionRTPSShapeWriter(28400, binding), WithSessionRTPSShapeWriter(28400, RTPSShapeWriter{[12]byte{99}, [4]byte{1}})},
	} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
	wire, a := rtpsWireCase(t, "le-le-norm-shape")
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSShapeWriter(28400, binding), WithSessionRTPSShapeWriter(28400, binding))
	require.NoError(t, err)
	clear(binding.SourceGUIDPrefix[:])
	clear(binding.WriterID[:])
	for _, part := range [][]byte{wire[:35], wire[35:]} {
		r := s.Feed(0, time.Unix(1700000000, 0), part)
		for _, e := range r.Events {
			if e.Protocol == "rtps" {
				f, err := e.GetFields()
				require.Error(t, err)
				require.Nil(t, f)
			}
		}
		require.Zero(t, s.Stats().BufferedBytes)
	}
	r := s.Feed(1, time.Unix(1700000001, 0), wire)
	require.Len(t, r.Events, 1)
	f, err := r.Events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, a.Events[0].Fields, f)
	require.Empty(t, s.Close("done"))
	// An explicitly selected wire-only destination wins over a typed source.
	s, err = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSShapeWriter(38000, rtpsShapeTestBinding()), WithSessionRTPSWire(28400))
	require.NoError(t, err)
	_, opaque := rtpsWireCase(t, "le-le-norm-wire")
	r = s.Feed(0, time.Unix(1700000000, 0), wire)
	require.Len(t, r.Events, 1)
	f, err = r.Events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, opaque.Events[0].Fields, f)
	require.Empty(t, s.Close("done"))
	for _, opts := range [][]CaptureOption{
		{WithProtocolRTPSShapeWriter(0, rtpsShapeTestBinding())},
		{WithProtocolRTPSShapeWriter(28400, RTPSShapeWriter{})},
		{WithProtocolDecodeAs("udp", 28400, "rtps-control"), WithProtocolRTPSShapeWriter(28400, rtpsShapeTestBinding())},
		{WithProtocolRTPSShapeWriter(28400, rtpsShapeTestBinding()), WithProtocolRTPSShapeWriter(28400, RTPSShapeWriter{[12]byte{99}, [4]byte{1}})},
	} {
		cfg := NewDefaultConfig()
		var err error
		for _, opt := range opts {
			if err = opt(cfg); err != nil {
				break
			}
		}
		require.Error(t, err)
	}
}
