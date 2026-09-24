package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
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
