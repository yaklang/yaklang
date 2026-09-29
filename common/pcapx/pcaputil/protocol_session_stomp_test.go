package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const stomp12Connect = "STOMP\naccept-version:1.2\nhost:broker.example\nheart-beat:0,0\n\n\x00"

func TestProtocolSessionSTOMPProbeAndBidirectionalFrames(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)

	p := s.Probe([]byte(stomp12Connect))
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "stomp", p.Protocol)
	r := s.Feed(0, ts, []byte(stomp12Connect))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "STOMP", r.Events[0].Session["Command"])
	require.Equal(t, "stomp-bounded-frame", r.Events[0].Profile)

	r = s.Feed(1, ts, []byte("CONNECTED\r\nversion:1.2\r\nheart-beat:0,0\r\n\r\n\x00"))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "CONNECTED", r.Events[0].Session["Command"])
	require.Equal(t, "1.2", r.Events[0].Session["Version"])

	r = s.Feed(0, ts, []byte("\n"))
	require.Nil(t, r.Err)
	require.Equal(t, "Heart-beat", r.Events[0].Entry)
	require.Equal(t, true, r.Events[0].Session["Heartbeat"])
	r = s.Feed(1, ts, []byte("\r\n"))
	require.Nil(t, r.Err)
	require.Equal(t, "Heart-beat", r.Events[0].Entry)

	message := []byte("MESSAGE\nsubscription:sub-1\nmessage-id:m-1\ndestination:/queue/a\ncontent-length:3\n\nA\x00B\x00")
	errorFrame := []byte("ERROR\nmessage:rejected\n\nnope\x00")
	r = s.Feed(1, ts, append(message, errorFrame...))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 2)
	require.Equal(t, "MESSAGE", r.Events[0].Session["Command"])
	require.Equal(t, []byte{'A', 0, 'B'}, r.Events[0].Session["Body"])
	require.Equal(t, "ERROR", r.Events[1].Session["Command"])
	require.Equal(t, []byte("nope"), r.Events[1].Session["Body"])
}

func TestProtocolSessionSTOMPConnectAlternative(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	wire := []byte("CONNECT\r\naccept-version:1.2\r\nhost:broker.example\r\n\r\n\x00")
	require.Equal(t, ProbeAccept, s.Probe(wire).Verdict)
	r := s.Feed(0, time.Unix(1, 0), wire)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "CONNECT", r.Events[0].Session["Command"])
}

func TestProtocolSessionSTOMPFragmentedHandshake(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	wire := []byte(stomp12Connect)
	for i := 0; i < len(wire); i++ {
		require.NotEqual(t, ProbeAccept, probeSTOMP(wire[:i], DefaultParserBudget().ProbeBytes).Verdict, "truncated prefix length %d", i)
	}

	for i, b := range wire {
		r := s.Feed(0, time.Unix(2, 0), []byte{b})
		if i+1 < len(wire) {
			require.Empty(t, r.Events)
			require.True(t, r.NeedMore)
			continue
		}
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, "STOMP", r.Events[0].Session["Command"])
	}
}

func TestSTOMPRejectsHTTPTextAndMalformedFrames(t *testing.T) {
	partialCRLF := probeSTOMP([]byte("CONNECT\r\naccept-version:1.2\r\n"), DefaultParserBudget().ProbeBytes)
	require.Equal(t, ProbeNeedMore, partialCRLF.Verdict)
	require.Equal(t, "stomp", partialCRLF.Protocol)

	for name, wire := range map[string][]byte{
		"http-connect":        []byte("CONNECT broker.example:61613 HTTP/1.1\r\nHost: broker.example\r\n\r\n"),
		"http-get":            []byte("GET /stomp HTTP/1.1\r\nHost: broker.example\r\n\r\n"),
		"ordinary-text":       []byte("STOMP is a word in this sentence, not a command\n"),
		"server-frame-alone":  []byte("CONNECTED\nversion:1.2\n\n\x00"),
		"unsupported-version": []byte("STOMP\naccept-version:1.3\nhost:broker.example\n\n\x00"),
		"missing-host":        []byte("STOMP\naccept-version:1.1\n\n\x00"),
		"missing-nul":         []byte("STOMP\naccept-version:1.2\nhost:broker.example\n\n"),
	} {
		t.Run(name, func(t *testing.T) {
			p := probeSTOMP(wire, DefaultParserBudget().ProbeBytes)
			require.NotEqual(t, ProbeAccept, p.Verdict, "%q", wire)
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			if name == "http-connect" || name == "http-get" {
				probe := s.Probe(wire)
				require.Equal(t, ProbeAccept, probe.Verdict)
				require.Equal(t, "http", probe.Protocol)
			} else if name != "missing-nul" {
				require.Equal(t, ProbeReject, s.Probe(wire).Verdict)
			} else {
				require.NotEqual(t, ProbeAccept, s.Probe(wire).Verdict)
			}
		})
	}

	badFrames := map[string][]byte{
		"truncated-body":        []byte("MESSAGE\ndestination:/q\ncontent-length:3\n\nabc"),
		"truncated-length-body": []byte("MESSAGE\ndestination:/q\ncontent-length:3\n\nab\x00"),
		"length-overrun":        []byte("MESSAGE\ndestination:/q\ncontent-length:3\n\nabcX"),
		"invalid-length":        []byte("MESSAGE\ndestination:/q\ncontent-length:+1\n\nx\x00"),
		"body-on-connected":     []byte("CONNECTED\nversion:1.2\n\nbody\x00"),
		"unknown-escape":        []byte("MESSAGE\ndestination:/q\\tbad\n\n\x00"),
		"unescaped-colon":       []byte("MESSAGE\ndestination:/q:bad\n\n\x00"),
		"invalid-heartbeat":     []byte("CONNECTED\nversion:1.2\nheart-beat:1,\n\n\x00"),
		"heartbeat-on-message":  []byte("MESSAGE\ndestination:/q\nheart-beat:1,1\n\n\x00"),
		"unknown-command":       []byte("STOMPING\naccept-version:1.2\n\n\x00"),
		"bare-carriage-return":  []byte("CONNECTED\rversion:1.2\n\n\x00"),
	}
	for name, wire := range badFrames {
		t.Run(name, func(t *testing.T) {
			_, complete, err := parseSTOMPFrame(wire, 1<<20)
			if name == "truncated-body" || name == "truncated-length-body" {
				require.NoError(t, err)
				require.False(t, complete)
			} else {
				require.Error(t, err, "complete=%v", complete)
			}
		})
	}
	valid := []byte("MESSAGE\ndestination:/q\ncontent-length:3\n\na\x00b\x00")
	frame, complete, err := parseSTOMPFrame(valid, 1<<20)
	require.NoError(t, err)
	require.True(t, complete)
	require.Equal(t, []byte{'a', 0, 'b'}, frame.body)
	require.Equal(t, len(valid), frame.total)
	repeated := []byte("MESSAGE\ndestination:/first\ndestination:/second\n\n\x00")
	frame, complete, err = parseSTOMPFrame(repeated, 1<<20)
	require.NoError(t, err)
	require.True(t, complete)
	require.Equal(t, "/first", frame.headers["destination"])
	_, complete, err = parseSTOMPFrame([]byte("MESSAGE\ncontent-length:64\n\n"), 64)
	require.Error(t, err)
	require.False(t, complete)
}

func TestProtocolSessionSTOMPPinnedNDPIStream(t *testing.T) {
	// Pinned upstream nDPI regression pcapng (manifest id ndpi-stomp), source
	// commit 4cae778e7e8f846b34f11d4f8392504cdebd3db8, SHA-256
	// 18c9a606bfd29c7759dea701ba960ea81e5d44acfea24e371349471f7a1b5c86.
	// Its live-vs-generated origin is not asserted. The STOMP 1.1 opening
	// handshake omits the mandatory host header, and a later "message" command
	// is lowercase. Reject this malformed trace at admission instead of using
	// its apparent protocol label as a positive fixture.
	var commands []string
	var errorsSeen []string
	err := ReplayPcap(bytes.NewReader(binCorpusBytes(t, "ndpi/ndpi-stomp.pcapng")),
		WithTCPReassemblyWorkers(1),
		WithBinParser(func(e *BinParserEvent) {
			if e.Protocol != "stomp" {
				return
			}
			if e.Status != "decoded" {
				errorsSeen = append(errorsSeen, e.Status+": "+e.Error+" offset="+strconv.FormatUint(e.Offset, 10)+" raw="+fmt.Sprintf("%q", e.Raw))
				return
			}
			if command, ok := e.Session["Command"].(string); ok {
				commands = append(commands, command)
			}
		}),
	)
	require.NoError(t, err)
	require.Empty(t, commands)
	require.Empty(t, errorsSeen)
}

// Synthetic STOMP 1.2 fixtures derived from the specification, not an
// independently captured broker session. Authentication commonly pushes the
// opening frame beyond the generic 64-byte signature window.
func TestProtocolSessionSTOMPLongHandshakeBoundaries(t *testing.T) {
	wire := []byte("CONNECT\naccept-version:1.2\nhost:broker.example\nlogin:user\npasscode:password\nheart-beat:10000,10000\n\n\x00")
	message := []byte("SEND\ndestination:/queue/a\nx-label:hello\tworld\ncontent-length:3\n\na\x00b\x00")
	combined := messagingRegressionBytes(t, "stomp12-authenticated-stream.bin")
	require.Equal(t, append(bytes.Clone(wire), message...), combined)
	for split := 0; split <= len(combined); split++ {
		s := newReviewSession(t, ParserBudget{})
		require.Equal(t, ProbeAccept, s.Probe(wire).Verdict, "split=%d", split)
		var events []*ProtocolEvent
		for _, chunk := range [][]byte{combined[:split], combined[split:]} {
			r := s.Feed(0, time.Unix(1, 0), chunk)
			if r.Err != nil {
				require.Equal(t, ErrNeedMore, r.Err.Kind, "split=%d: %s", split, r.Err)
			}
			events = append(events, r.Events...)
		}
		require.Len(t, events, 2, "split=%d", split)
		require.Equal(t, wire, events[0].Raw)
		require.Equal(t, message, events[1].Raw)
		require.Equal(t, "decoded", events[0].Status)
		require.Equal(t, "decoded", events[1].Status)
		headers := events[1].Session["Headers"].(map[string]any)
		require.Equal(t, "hello\tworld", headers["x-label"])
		require.Equal(t, []byte{'a', 0, 'b'}, events[1].Session["Body"])
	}
	for _, lineEnd := range []string{"\n", "\r\n"} {
		handshake := bytes.ReplaceAll(wire, []byte("\n"), []byte(lineEnd))
		events, _ := sessionTestFlow(t, "stomp", []sessionStep{{0, handshake}}, 1, false)
		require.Len(t, events, 1)
		assertSessionEvents(t, events, "stomp", false)
	}
}

func TestProtocolSessionSTOMPHandshakeBudgets(t *testing.T) {
	// Synthetic complete and incomplete handshakes exercise both configured
	// frame limits and the fixed line/header bounds without retaining forever.
	for _, wire := range [][]byte{
		[]byte("CONNECT\naccept-version:1.2\nhost:broker.example\nlogin:" + strings.Repeat("x", 96)),
		[]byte("STOMP\naccept-version:1.2\nhost:" + strings.Repeat("x", stompMaxLineBytes+2)),
	} {
		s := newReviewSession(t, ParserBudget{MaxFrameBytes: 128})
		require.NotEqual(t, ProbeAccept, s.Probe(wire).Verdict)
		r := s.Feed(0, time.Unix(1, 0), wire)
		require.NotNil(t, r.Err)
		require.False(t, r.NeedMore)
		require.NotEqual(t, "stomp", r.State)
	}
	for _, command := range []string{"CONNECT", "STOMP"} {
		wire := []byte(command + "\naccept-version:1.2\nhost:broker.example\nlogin:a\tb\n\n\x00")
		s := newReviewSession(t, ParserBudget{})
		r := s.Feed(0, time.Unix(1, 0), wire)
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, "a\tb", r.Events[0].Session["Headers"].(map[string]any)["login"])
	}
	_, complete, err := parseSTOMPFrame([]byte("SEND\ndestination:/q\nx-label:hello\\tworld\n\nx\x00"), 1024)
	require.Error(t, err, "a literal TAB is legal; an undefined backslash-t escape is not")
	require.False(t, complete)
}

func TestProtocolSessionSTOMPMaximumHeaderLineCRLF(t *testing.T) {
	// Synthetic maximum-size header with equivalent LF and CRLF encoding.
	for _, eol := range []string{"\n", "\r\n"} {
		line := "x:" + strings.Repeat("x", stompMaxLineBytes-2)
		wire := []byte("SEND" + eol + "destination:/q" + eol + line + eol + eol + "body\x00")
		if eol == "\r\n" {
			require.Equal(t, wire, messagingRegressionBytes(t, "stomp-max-crlf-header.bin"))
		}
		for _, split := range []int{0, len("SEND"+eol+"destination:/q"+eol) + len(line), len("SEND"+eol+"destination:/q"+eol) + len(line) + 1} {
			s := newReviewSession(t, ParserBudget{})
			require.Nil(t, s.Feed(0, time.Unix(1, 0), []byte(stomp12Connect)).Err)
			if split > 0 {
				r := s.Feed(0, time.Unix(1, 0), wire[:split])
				require.True(t, r.NeedMore)
				require.Empty(t, r.Events)
			}
			r := s.Feed(0, time.Unix(1, 0), wire[split:])
			require.Nil(t, r.Err, "eol=%q split=%d", eol, split)
			require.Len(t, r.Events, 1)
			require.Equal(t, []byte("body"), r.Events[0].Session["Body"])
		}
	}
}

func TestSTOMPRejectsHeaderLineBeyondBoundary(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		wire := []byte("SEND" + eol + "x:" + strings.Repeat("x", stompMaxLineBytes-1) + eol + eol + "body\x00")
		_, complete, err := parseSTOMPFrame(wire, 1<<20)
		require.Error(t, err)
		require.False(t, complete)
	}
}

func TestProtocolSessionSTOMPRejectsUnknownExplicitVersion(t *testing.T) {
	for _, version := range []string{"1.3", "2.0", ""} {
		s := newReviewSession(t, ParserBudget{})
		require.Nil(t, s.Feed(0, time.Unix(1, 0), []byte(stomp12Connect)).Err)
		wire := []byte("CONNECTED\nversion:" + version + "\n\n\x00")
		if version == "1.3" {
			require.Equal(t, wire, messagingRegressionBytes(t, "stomp-unknown-version.bin"))
		}
		r := s.Feed(1, time.Unix(1, 0), wire)
		require.NotNil(t, r.Err, "version=%q", version)
		require.Equal(t, ErrUnsupportedVersion, r.Err.Kind)
		require.Len(t, r.Events, 1)
		require.Equal(t, "context-required", r.Events[0].Status)
	}
	for _, header := range []string{"", "version:1.0\n", "version:1.1\n", "version:1.2\n"} {
		s := newReviewSession(t, ParserBudget{})
		require.Nil(t, s.Feed(0, time.Unix(1, 0), []byte(stomp12Connect)).Err)
		r := s.Feed(1, time.Unix(1, 0), []byte("CONNECTED\n"+header+"\n\x00"))
		require.Nil(t, r.Err, "header=%q", header)
		require.Len(t, r.Events, 1)
		require.Equal(t, "decoded", r.Events[0].Status)
	}
}

func TestProtocolSessionSTOMPSENDRequiresDestination(t *testing.T) {
	for _, wire := range [][]byte{messagingRegressionBytes(t, "stomp-send-missing-destination.bin"), []byte("SEND\ncontent-length:4\n\nbody\x00")} {
		for split := 0; split <= len(wire); split++ {
			s := newReviewSession(t, ParserBudget{})
			require.Nil(t, s.Feed(0, time.Unix(1, 0), []byte(stomp12Connect)).Err)
			var failure *ProtocolError
			var events []*ProtocolEvent
			for _, chunk := range [][]byte{wire[:split], wire[split:]} {
				if len(chunk) == 0 {
					continue
				}
				r := s.Feed(0, time.Unix(1, 0), chunk)
				if r.Err != nil && r.Err.Kind != ErrNeedMore {
					failure = r.Err
				}
				events = append(events, r.Events...)
			}
			require.NotNil(t, failure, "split=%d", split)
			require.Equal(t, ErrMalformedMessage, failure.Kind)
			require.Len(t, events, 1)
			require.Equal(t, "malformed", events[0].Status)
		}
	}
}
