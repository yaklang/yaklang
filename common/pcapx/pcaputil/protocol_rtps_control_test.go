package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Preserve immutable historical wire/answer files while applying the explicit
// independent PSM correction. Both old input and answer hashes must match;
// a different input cannot silently inherit this erratum.
func rtpsControlWireCases(t *testing.T) []rtpsControl {
	t.Helper()
	cases := rtpsBatchControls(t, "rtps-controls", 256)
	raw, err := trafficfixture.ReadFile("rtps-control-wire/corrections.json")
	require.NoError(t, err)
	var corrections struct {
		Schema string
		Cases  []struct {
			ID          string `json:"id"`
			CaptureSHA  string `json:"capture_sha256"`
			OriginalSHA string `json:"original_answer_sha256"`
			Answer      string `json:"answer"`
			AnswerSHA   string `json:"answer_sha256"`
		} `json:"corrections"`
	}
	require.NoError(t, json.Unmarshal(raw, &corrections))
	require.Equal(t, "rtps-control-wire-errata/v1", corrections.Schema)
	require.Len(t, corrections.Cases, 4)
	seen := map[string]bool{}
	for _, correction := range corrections.Cases {
		require.False(t, seen[correction.ID])
		seen[correction.ID] = true
		found := false
		for i := range cases {
			c := &cases[i]
			if c.ID != correction.ID {
				continue
			}
			found = true
			require.Equal(t, c.SHA256, correction.CaptureSHA)
			require.Equal(t, c.AnswerSHA, correction.OriginalSHA)
			c.Answer = "common/pcapx/pcaputil/rtps-control-wire/" + correction.Answer
			c.AnswerSHA = correction.AnswerSHA
		}
		require.True(t, found)
	}
	return cases
}

func TestRTPSControlSealedReplay(t *testing.T) {
	testRTPSSealedReplayProfile(t, rtpsControlWireCases(t), "rtps-control")
}

func newRTPSControlTestSession(t *testing.T, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSControl(28400))
	require.NoError(t, err)
	return s
}

func TestRTPSControlSealedSession(t *testing.T) {
	for _, c := range rtpsControlWireCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := rtpsInput(t, c)
			s := newRTPSControlTestSession(t, DefaultParserBudget())
			for i, encoded := range a.Wires {
				wire, err := hex.DecodeString(encoded)
				require.NoError(t, err)
				f, err := decodeRTPSControl(wire, 1024)
				if a.PureErrors[i] == "" {
					require.NoError(t, err)
					rocEqualFields(t, a.Events[i].Fields, f)
				} else {
					var pe *ProtocolError
					require.ErrorAs(t, err, &pe)
					require.Equal(t, a.PureErrors[i], string(pe.Kind))
					require.Nil(t, f)
				}
				for j := 0; j < 2; j++ {
					p := s.Probe(wire)
					if a.PureErrors[i] == "" {
						require.Equal(t, ProbeAccept, p.Verdict)
					} else {
						require.Equal(t, ProbeReject, p.Verdict)
					}
				}
				input := bytes.Clone(wire)
				r := s.Feed(i%2, time.Unix(1700000000, int64(i)), input)
				clear(input)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				require.Equal(t, wire, e.Raw)
				require.Equal(t, "rtps", e.Protocol)
				require.Equal(t, "rtps-control-native", e.Profile)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				f, err = e.GetFields()
				if a.PureErrors[i] == "" {
					require.NoError(t, err)
					rocEqualFields(t, a.Events[i].Fields, f)
					rocEqualFields(t, a.Events[i].Fields, e.Session)
					mutateRTPSQuery(f)
					mutateRTPSQuery(e.Session)
					clear(e.Raw)
					testNativeObservationSessionQueries(t, e, a.Events[i].Fields)
				} else {
					require.Nil(t, f)
					assertOwnedProtocolRefusal(t, e, a.PureErrors[i], err)
				}
				require.Zero(t, s.Stats().BufferedBytes)
			}
			require.Empty(t, s.Close("end"))
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

func rtpsControlCase(t *testing.T, id string) ([]byte, rtpsControlAnswer) {
	t.Helper()
	for _, c := range rtpsControlWireCases(t) {
		if c.ID == id {
			_, a := rtpsInput(t, c)
			wire, err := hex.DecodeString(a.Wires[0])
			require.NoError(t, err)
			return wire, a
		}
	}
	t.Fatalf("missing independently constructed case %q", id)
	return nil, rtpsControlAnswer{}
}

func TestRTPSControlResourcesRecovery(t *testing.T) {
	plain, pa := rtpsControlCase(t, "v5-07-le")
	bitmap, ba := rtpsControlCase(t, "bitmap-06-33-le")
	// One submessage, two words and three selected offsets require six units.
	for _, limit := range []int{5, 6} {
		b := DefaultParserBudget()
		b.MaxCollectionElements = limit
		s := newRTPSControlTestSession(t, b)
		for i, wire := range [][]byte{bitmap, plain} {
			before := s.Stats()
			p := s.Probe(wire)
			require.Equal(t, before, s.Stats(), "probe must not allocate session history")
			r := s.Feed(i%2, time.Unix(1700000000, int64(i)), wire)
			require.Len(t, r.Events, 1)
			f, err := r.Events[0].GetFields()
			if i == 0 && limit == 5 {
				require.Equal(t, ProbeReject, p.Verdict)
				assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
				require.Nil(t, f)
			} else {
				require.Equal(t, ProbeAccept, p.Verdict)
				require.NoError(t, err)
				want := pa.Events[0].Fields
				if i == 0 {
					want = ba.Events[0].Fields
				}
				rocEqualFields(t, want, f)
			}
			require.Zero(t, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
			require.Zero(t, s.Stats().BufferedBytes)
		}
		require.Empty(t, s.Close("done"))
	}
	for _, depth := range []int{3, 4} {
		b := DefaultParserBudget()
		b.MaxRecursionDepth = depth
		s := newRTPSControlTestSession(t, b)
		r := s.Feed(0, time.Unix(1700000000, 0), plain)
		require.Len(t, r.Events, 1)
		f, err := r.Events[0].GetFields()
		if depth == 3 {
			require.Nil(t, f)
			assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
		} else {
			require.NoError(t, err)
			rocEqualFields(t, pa.Events[0].Fields, f)
		}
		s.Close("done")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	b := DefaultParserBudget()
	b.MaxMessageBytes, b.MaxFrameBytes = 64, 64 // Public parser minimum; plain52 fits, combined84 exceeds.
	s := newRTPSControlTestSession(t, b)
	large := append(bytes.Clone(plain), plain[20:]...)
	require.Equal(t, ProbeReject, s.Probe(large).Verdict)
	r := s.Feed(0, time.Unix(1700000000, 0), large)
	require.Len(t, r.Events, 1)
	_, err := r.Events[0].GetFields()
	assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
	r = s.Feed(1, time.Unix(1700000001, 0), plain)
	require.Len(t, r.Events, 1)
	f, err := r.Events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, pa.Events[0].Fields, f)
	require.Zero(t, r.Events[0].ResponseTo)
	require.Zero(t, r.Events[0].TransactionID)
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestRTPSControlCarrierAndDatagramBoundary(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{
		{WithSessionTransport("tcp"), WithSessionRTPSControl(28400)},
		{WithSessionTransport("udp"), WithSessionRTPSControl(0)},
		{WithSessionTransport("udp"), WithSessionRTPSControl(28400), WithSessionRTPSFragments(28400)},
	} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
	wire, a := rtpsControlCase(t, "v5-07-le")
	s := newRTPSControlTestSession(t, DefaultParserBudget())
	for _, part := range [][]byte{wire[:35], wire[35:]} {
		for _, e := range s.Feed(0, time.Unix(1700000000, 0), part).Events {
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
	require.Zero(t, r.Events[0].ResponseTo)
	require.Zero(t, r.Events[0].TransactionID)
	require.Empty(t, s.Close("done"))
}
