package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func natsTestHeaderFrame(op string, header, body []byte) []byte {
	args := "native.headers"
	if op == "HMSG" {
		args += " 1"
	}
	return []byte(fmt.Sprintf("%s %s %d %d\r\n%s%s\r\n", op, args, len(header), len(header)+len(body), header, body))
}

func TestProtocolSessionNATSHeaderSemanticsRegression(t *testing.T) {
	t.Run("native ASCII header values", func(t *testing.T) {
		// This byte sequence was independently sent and received by official nats.go v1.39.1.
		h := []byte("NATS/1.0\r\nASCII: \x00\x01\x7f\r\nX-Trace: alpha\r\nX-Trace: beta, gamma\r\nx-trace: lower\r\n\r\n")
		info, err := natsFrameInfoWithBudget(natsTestHeaderFrame("HMSG", h, nil), DefaultParserBudget())
		require.NoError(t, err)
		require.Equal(t, map[string]any{"ASCII": []string{"\x00\x01\x7f"}, "X-Trace": []string{"alpha", "beta, gamma"}, "x-trace": []string{"lower"}}, info.fields["Headers"])
	})
	t.Run("missing colon", func(t *testing.T) {
		_, err := natsFrameInfoWithBudget(natsTestHeaderFrame("HPUB", []byte("NATS/1.0\r\nnot-a-header\r\n\r\n"), nil), DefaultParserBudget())
		require.Error(t, err)
	})
	t.Run("invalid status", func(t *testing.T) {
		_, err := natsFrameInfoWithBudget(natsTestHeaderFrame("HMSG", []byte("NATS/1.0 arbitrary\r\n\r\n"), nil), DefaultParserBudget())
		require.Error(t, err)
	})
	t.Run("declared header swallows body", func(t *testing.T) {
		_, err := natsFrameInfoWithBudget(natsTestHeaderFrame("HPUB", []byte("NATS/1.0\r\n\r\nbody\r\n\r\n"), nil), DefaultParserBudget())
		require.Error(t, err)
	})
}

// These frames are explicit synthetic boundary/negative cases. Native bytes and
// independent official client results live in protocol-native/nats instead.
func TestProtocolSessionNATSHeaderValidationAndBudgets(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		kind         ProtocolErrorKind
	}{
		{"empty-section", "NATS/1.0\r\n\r\n", ""},
		{"503", "NATS/1.0 503\r\n\r\n", ""},
		{"100-description", "NATS/1.0 100 Idle Heartbeat\r\nX: y\r\n\r\n", ""},
		{"future-status-code", "NATS/1.0 799 Unknown status\r\n\r\n", ""},
		{"folding", "NATS/1.0\r\nX: first\r\n second\r\n\tthird\r\nX: next\r\n\r\n", ""},
		{"printable-key", "NATS/1.0\r\nX(): okay\r\n\r\n", ""},
		{"future-version", "NATS/2.0\r\n\r\n", ErrUnsupportedVersion},
		{"future-minor-version", "NATS/1.1\r\n\r\n", ErrUnsupportedVersion},
		{"invalid-version", "NATS/1.x\r\n\r\n", ErrMalformedMessage},
		{"lowercase-version", "nats/1.0\r\n\r\n", ErrMalformedMessage},
		{"version-no-boundary", "NATS/1.0extra\r\n\r\n", ErrMalformedMessage},
		{"one-digit-status", "NATS/1.0 5\r\n\r\n", ErrMalformedMessage},
		{"two-digit-status", "NATS/1.0 50\r\n\r\n", ErrMalformedMessage},
		{"four-digit-status", "NATS/1.0 5030\r\n\r\n", ErrMalformedMessage},
		{"empty-status", "NATS/1.0 \r\n\r\n", ErrMalformedMessage},
		{"status-no-boundary", "NATS/1.0 503No Responders\r\n\r\n", ErrMalformedMessage},
		{"status-control", "NATS/1.0 503 No\x00Responders\r\n\r\n", ErrMalformedMessage},
		{"bare-LF", "NATS/1.0\r\nX: a\nb\r\n\r\n", ErrMalformedMessage},
		{"bare-CR", "NATS/1.0\r\nX: a\rb\r\n\r\n", ErrMalformedMessage},
		{"missing-end", "NATS/1.0\r\nX: a\r\n", ErrMalformedMessage},
		{"empty-key", "NATS/1.0\r\n: value\r\n\r\n", ErrMalformedMessage},
		{"space-key", "NATS/1.0\r\nBad Key: value\r\n\r\n", ErrMalformedMessage},
		{"control-key", "NATS/1.0\r\nBad\x00: value\r\n\r\n", ErrMalformedMessage},
		{"unattached-continuation", "NATS/1.0\r\n value\r\n\r\n", ErrMalformedMessage},
		{"non-ASCII-value", "NATS/1.0\r\nX: \xff\r\n\r\n", ErrMalformedMessage},
		{"extra-empty-line", "NATS/1.0\r\n\r\n\r\n", ErrMalformedMessage},
	} {
		for _, op := range []string{"HPUB", "HMSG"} {
			t.Run(op+"/"+tc.name, func(t *testing.T) {
				wire := natsTestHeaderFrame(op, []byte(tc.header), []byte("\x00\r\nbody\r\n\r\n"))
				info, err := natsFrameInfoWithBudget(wire, DefaultParserBudget())
				if tc.kind == "" {
					require.NoError(t, err)
					require.Equal(t, len(wire), info.length)
					require.Equal(t, true, info.fields["Headers Decoded"])
					if tc.name == "folding" {
						require.Equal(t, []string{"first second third", "next"}, info.fields["Headers"].(map[string]any)["X"])
					}
				} else {
					var pe *ProtocolError
					require.ErrorAs(t, err, &pe)
					require.Equal(t, tc.kind, pe.Kind)
				}
			})
		}
	}
	for _, h := range []string{"NATS/1.0\r\nA: 1\r\nA: 2\r\nA: 3\r\n\r\n", "NATS/1.0\r\nA: 1\r\n 2\r\n 3\r\n\r\n"} {
		wire := natsTestHeaderFrame("HPUB", []byte(h), nil)
		budget := DefaultParserBudget()
		budget.MaxCollectionElements = 3
		_, err := natsFrameInfoWithBudget(wire, budget)
		require.NoError(t, err)
		budget.MaxCollectionElements = 2
		_, err = natsFrameInfoWithBudget(wire, budget)
		var pe *ProtocolError
		require.ErrorAs(t, err, &pe)
		require.Equal(t, ErrResourceExceeded, pe.Kind)
		budget = DefaultParserBudget()
		budget.MaxMessageBytes = len(wire) - 1
		_, err = natsFrameInfoWithBudget(wire, budget)
		require.ErrorAs(t, err, &pe)
		require.Equal(t, ErrResourceExceeded, pe.Kind)
	}
	for _, wire := range []string{"HPUB a 0 0\r\n\r\n", "HMSG a 1 0 0\r\n\r\n", "HPUB a 12 11\r\n", "HMSG a 1 12 11\r\n", "HPUB a 12 12\r\nNATS/1.0\r\n\r\nX\n"} {
		_, err := natsFrameInfoWithBudget([]byte(wire), DefaultParserBudget())
		var pe *ProtocolError
		require.ErrorAs(t, err, &pe)
		require.Equal(t, ErrMalformedMessage, pe.Kind)
	}
	// Invalid headers fail as soon as the declared header bytes are present,
	// without waiting for a declared, but absent, application body.
	bad := []byte("NATS/1.0\r\ninvalid\r\n\r\n")
	_, err := natsFrameInfoWithBudget([]byte(fmt.Sprintf("HPUB a %d 10000\r\n%s", len(bad), bad)), DefaultParserBudget())
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, ErrMalformedMessage, pe.Kind)
}

func TestProtocolSessionNATSHeaderNegotiationAndDirections(t *testing.T) {
	header := []byte("NATS/1.0\r\nX: first\r\nX: second\r\n\r\n")
	for _, tc := range []struct {
		name, info, connect string
		kind                ProtocolErrorKind
	}{
		{"enabled", `{"headers":true}`, `{"headers":true}`, ""},
		{"client-disabled", `{"headers":true}`, `{"headers":false}`, ErrUnsupportedFeature},
		{"client-omitted", `{"headers":true}`, `{}`, ErrUnsupportedFeature},
		{"server-disabled", `{"headers":false}`, `{"headers":true}`, ErrUnsupportedFeature},
		{"server-unobserved", `{}`, `{"headers":true}`, ""},
	} {
		for _, op := range []string{"HPUB", "HMSG"} {
			t.Run(tc.name+"/"+op, func(t *testing.T) {
				s := newReviewSession(t, ParserBudget{})
				require.Nil(t, s.Feed(1, time.Time{}, []byte("INFO "+tc.info+"\r\n")).Err)
				require.Nil(t, s.Feed(0, time.Time{}, []byte("CONNECT "+tc.connect+"\r\n")).Err)
				require.Nil(t, s.Feed(1, time.Time{}, []byte("INFO {}\r\n")).Err, "asynchronous update cannot revoke headers")
				dir := 0
				if op == "HMSG" {
					dir = 1
				}
				r := s.Feed(dir, time.Time{}, natsTestHeaderFrame(op, header, nil))
				if tc.kind == "" {
					require.Nil(t, r.Err)
					require.Equal(t, "decoded", r.Events[0].Status)
				} else {
					require.NotNil(t, r.Err)
					require.Equal(t, tc.kind, r.Err.Kind)
					require.Equal(t, "context-required", r.Events[0].Status)
				}
			})
		}
	}
	for _, op := range []string{"HPUB", "HMSG"} {
		for _, dir := range []int{0, 1} {
			t.Run(fmt.Sprintf("midstream-%s-%d", op, dir), func(t *testing.T) {
				s := newReviewSession(t, ParserBudget{})
				r := s.Feed(dir, time.Time{}, natsTestHeaderFrame(op, header, nil))
				require.Nil(t, r.Err)
				want := "client"
				if op == "HMSG" {
					want = "server"
				}
				require.Equal(t, want, r.Events[0].Session["Direction Role"])
				require.Equal(t, "unknown", r.Events[0].Session["Header Negotiation"])
			})
		}
	}
	for _, op := range []string{"HPUB", "HMSG"} {
		s := newReviewSession(t, ParserBudget{})
		require.Nil(t, s.Feed(1, time.Time{}, []byte("INFO {\"headers\":true}\r\n")).Err)
		wrongDir := 1
		if op == "HMSG" {
			wrongDir = 0
		}
		r := s.Feed(wrongDir, time.Time{}, natsTestHeaderFrame(op, header, nil))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrMalformedMessage, r.Err.Kind)
	}
	for _, value := range []string{`null`, `"true"`, `1`, `[]`, `{}`} {
		for _, op := range []string{"INFO", "CONNECT"} {
			_, err := natsFrameInfoWithBudget([]byte(op+` {"headers":`+value+"}\r\n"), DefaultParserBudget())
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
		}
	}
}

func TestProtocolSessionNATSHeaderFragmentationAndContextBudget(t *testing.T) {
	h := []byte("NATS/1.0\r\nX: first\r\nX: second\r\n\r\n")
	steps := []sessionStep{{dir: 1, wire: []byte("INFO {\"headers\":true}\r\n")}, {dir: 0, wire: []byte("CONNECT {\"headers\":true}\r\n")}, {dir: 0, wire: append(natsTestHeaderFrame("HPUB", h, []byte("\x00\r\nbody")), []byte("PING\r\n")...)}, {dir: 1, wire: append(natsTestHeaderFrame("HMSG", []byte("NATS/1.0 100 Idle Heartbeat\r\n\r\n"), nil), []byte("PONG\r\n")...)}}
	whole, _ := sessionTestFlow(t, "nats", steps, 0, false)
	deferred, _ := sessionTestFlow(t, "nats", steps, 0, true)
	require.Len(t, deferred, len(whole))
	for i, event := range deferred {
		require.Equal(t, whole[i].Session, event.Session)
		got, err := event.GetFields()
		require.NoError(t, err)
		want, err := whole[i].GetFields()
		require.NoError(t, err)
		require.Equal(t, want, got)
		decoded, err := event.Decode()
		require.NoError(t, err)
		require.Equal(t, event.Session, decoded["session"])
		if headers, ok := decoded["session"].(map[string]any)["Headers"].(map[string]any); ok {
			headers["caller-mutation"] = []string{"owned"}
			require.NotContains(t, event.Session["Headers"], "caller-mutation")
		}
	}
	for _, chunk := range []int{1, 2, 3, 7, 31, 64} {
		split, _ := sessionTestFlow(t, "nats", steps, chunk, false)
		require.Len(t, split, len(whole))
		for i := range split {
			require.Equal(t, whole[i].Raw, split[i].Raw)
			require.Equal(t, whole[i].Session, split[i].Session)
		}
	}
	s := newReviewSession(t, ParserBudget{}).(*captureSession)
	require.Nil(t, s.Feed(1, time.Time{}, steps[0].wire).Err)
	require.Equal(t, int64(64), s.f.sessionBytes)
	for i := 0; i < 20; i++ {
		require.Nil(t, s.Feed(0, time.Time{}, steps[2].wire).Err)
	}
	require.Equal(t, int64(64), s.f.sessionBytes, "header strings are not retained in connection state")
	s.Close("FIN")
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, s.f.nats)
	for _, first := range []struct {
		dir  int
		wire []byte
	}{{1, []byte("INFO {}\r\n")}, {0, []byte("CONNECT {}\r\n")}, {1, natsTestHeaderFrame("HMSG", []byte("NATS/1.0\r\n\r\n"), nil)}} {
		low := newReviewSession(t, ParserBudget{MaxBufferedBytes: 64, MaxMessageBytes: 64, MaxFrameBytes: 64, ProbeBytes: 16}).(*captureSession)
		// The public minimum buffer is 64 bytes, exactly the NATS state
		// reservation. Occupy one byte in another connection sharing this
		// capture, so the first NATS state allocation must fail atomically.
		other := &binFlow{a: low.f.a}
		require.NoError(t, other.reserveSession(1))
		r := low.Feed(first.dir, time.Time{}, first.wire)
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		require.Nil(t, low.f.nats, "failed reservation must not allocate session state")
		other.closeSession()
		low.Close("FIN")
		require.Zero(t, low.Stats().BufferedBytes)
	}
}

func TestProtocolSessionNATSHeaderFailureBoundary(t *testing.T) {
	for _, tc := range []struct {
		header string
		kind   ProtocolErrorKind
		status string
	}{
		{"NATS/1.0\r\nbad\r\n\r\n", ErrMalformedMessage, "malformed"},
		{"NATS/2.0\r\n\r\n", ErrUnsupportedVersion, "context-required"},
	} {
		s := newReviewSession(t, ParserBudget{})
		require.Nil(t, s.Feed(1, time.Time{}, []byte("INFO {\"headers\":true}\r\n")).Err)
		require.Nil(t, s.Feed(0, time.Time{}, []byte("CONNECT {\"headers\":true}\r\n")).Err)
		good := natsTestHeaderFrame("HPUB", []byte("NATS/1.0\r\nX: safe\r\n\r\n"), []byte("body"))
		bad := natsTestHeaderFrame("HPUB", []byte(tc.header), nil)
		wire := append(bytes.Clone(good), bad...)
		wire = append(wire, []byte("PING\r\n")...)
		r := s.Feed(0, time.Time{}, wire)
		require.NotNil(t, r.Err)
		require.Equal(t, tc.kind, r.Err.Kind)
		require.Len(t, r.Events, 2)
		require.Equal(t, "decoded", r.Events[0].Status)
		require.Equal(t, good, r.Events[0].Raw)
		require.Equal(t, tc.status, r.Events[1].Status, "a future header version is unsupported, not malformed")
		next := s.Feed(1, time.Time{}, natsTestHeaderFrame("HMSG", []byte("NATS/1.0\r\n\r\n"), nil))
		for _, event := range next.Events {
			require.NotEqual(t, "decoded", event.Status, "a failed connection cannot resume without context")
		}
		s.Close("FIN")
		require.Empty(t, s.Close("FIN"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
