package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func natsPublish(subject, reply string, body []byte) []byte {
	if reply == "" {
		return append([]byte(fmt.Sprintf("PUB %s %d\r\n", subject, len(body))), append(append([]byte(nil), body...), '\r', '\n')...)
	}
	return append([]byte(fmt.Sprintf("PUB %s %s %d\r\n", subject, reply, len(body))), append(append([]byte(nil), body...), '\r', '\n')...)
}

func natsHeaderMessage(op, subject, sid, reply string, body []byte) []byte {
	header := []byte("NATS/1.0\r\nBar: Baz\r\n\r\n")
	args := subject + " " + sid
	if reply != "" {
		args += " " + reply
	}
	return []byte(fmt.Sprintf("%s %s %d %d\r\n%s%s\r\n", op, args, len(header), len(header)+len(body), header, body))
}

func TestProtocolSessionNATSClientProtocolFraming(t *testing.T) {
	body := []byte{'A', '\r', '\n', 'P', 'I', 'N', 'G', '\r', '\n'}
	hpubHeader := []byte("NATS/1.0\r\nBar: Baz\r\n\r\n")
	hpubBody := []byte{'H', '\r', '\n', 'i'}
	hpub := []byte(fmt.Sprintf("HPUB foo.bar reply.1 %d %d\r\n%s%s\r\n", len(hpubHeader), len(hpubHeader)+len(hpubBody), hpubHeader, hpubBody))
	info := []byte("INFO {\"server_id\":\"local\",\"headers\":true}\r\n")
	connect := []byte("CONNECT {\"verbose\":false,\"lang\":\"go\"}\r\n")
	steps := []sessionStep{
		{dir: 1, wire: info},
		{dir: 0, wire: connect},
		{dir: 0, wire: append(natsPublish("foo.bar", "", body), []byte("PING\r\n")...)},
		{dir: 1, wire: []byte("PONG\r\n")},
		{dir: 0, wire: hpub},
		{dir: 0, wire: []byte("SUB foo.* workers 7\r\nUNSUB 7 10\r\n")},
		{dir: 1, wire: []byte("MSG foo.bar 7 reply.9 11\r\nHello World\r\n")},
		{dir: 1, wire: natsHeaderMessage("HMSG", "foo.bar", "7", "reply.9", []byte("body"))},
		{dir: 1, wire: []byte("+OK\r\n-ERR 'Authorization Violation'\r\nPING\r\n")},
		{dir: 0, wire: []byte("PONG\r\n")},
	}
	events, f := sessionTestFlow(t, "nats", steps, 0, false)
	assertSessionEvents(t, events, "nats", false)
	require.Equal(t, "nats", f.protocol)
	require.Len(t, events, 14)
	require.Equal(t, "server", events[0].Session["Direction Role"])
	require.Equal(t, "client", events[1].Session["Direction Role"])
	require.Equal(t, "foo.bar", events[2].Session["Subject"])
	require.Equal(t, len(body), events[2].Session["Payload Length"])
	require.Equal(t, "NATSPayloadFrame", events[2].Entry)
	require.Equal(t, "PING", events[3].Session["Operation"])
	require.Equal(t, len(hpubHeader), events[5].Session["Header Length"])
	require.Equal(t, "workers", events[6].Session["Queue Group"])
	require.Equal(t, uint64(10), events[7].Session["Maximum Message Count"])
	require.Equal(t, "reply.9", events[8].Session["Reply Subject"])
	require.Equal(t, len(hpubHeader), events[9].Session["Header Length"])
	require.Equal(t, "'Authorization Violation'", events[11].Session["Error Text"])
	require.Equal(t, "nats-plaintext-client", events[0].Profile)
	wantFrames := [][]byte{
		info,
		connect,
		natsPublish("foo.bar", "", body),
		[]byte("PING\r\n"),
		[]byte("PONG\r\n"),
		hpub,
		[]byte("SUB foo.* workers 7\r\n"),
		[]byte("UNSUB 7 10\r\n"),
		[]byte("MSG foo.bar 7 reply.9 11\r\nHello World\r\n"),
		natsHeaderMessage("HMSG", "foo.bar", "7", "reply.9", []byte("body")),
		[]byte("+OK\r\n"),
		[]byte("-ERR 'Authorization Violation'\r\n"),
		[]byte("PING\r\n"),
		[]byte("PONG\r\n"),
	}
	for _, event := range events {
		require.Equal(t, false, event.Session["Payload Decoded"])
		require.Equal(t, false, event.Session["TCP Reassembly Performed"])
	}
	for i, event := range events {
		require.Equal(t, wantFrames[i], event.Raw, "frame %d must retain the exact command, payload, and trailer bytes", i)
	}
}

func TestProtocolSessionNATSFragmentationAndIncompleteBody(t *testing.T) {
	steps := []sessionStep{{dir: 1, wire: []byte("INFO {\"server_id\":\"local\"}\r\n")}, {dir: 0, wire: natsPublish("a", "b", []byte("one\r\ntwo"))}}
	whole, _ := sessionTestFlow(t, "nats", steps, 0, false)
	for _, chunk := range []int{1, 2, 5, 9} {
		got, _ := sessionTestFlow(t, "nats", steps, chunk, false)
		require.Len(t, got, len(whole), "chunk=%d", chunk)
		for i := range got {
			require.Equal(t, whole[i].Raw, got[i].Raw, "chunk=%d event=%d", chunk, i)
			require.Equal(t, whole[i].Session, got[i].Session, "chunk=%d event=%d", chunk, i)
		}
	}

	s := newReviewSession(t, ParserBudget{})
	require.Nil(t, s.Feed(1, time.Unix(1, 0), []byte("INFO {\"server_id\":\"local\"}\r\n")).Err)
	cut := s.Feed(0, time.Unix(1, 0), []byte("PUB foo 8\r\nabc"))
	require.Equal(t, "nats", cut.State)
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
	closed := s.Close("FIN")
	require.Len(t, closed, 1)
	require.Equal(t, "nats", closed[0].Protocol)
	require.Equal(t, "incomplete", closed[0].Status)
}

func TestProtocolSessionNATSMalformedHTTPAndLimits(t *testing.T) {
	for _, wire := range [][]byte{
		[]byte("PUB foo nope\r\n"),
		[]byte("PUB foo 3\r\nabcX\n"),
		[]byte("HPUB foo 20 10\r\nNATS/1.0\r\n\r\n"),
		[]byte("CONNECT {not-json}\r\n"),
		[]byte("INFO not-json\r\n"),
		[]byte("INFO\r\n"),
		[]byte("PING\n"),
		[]byte("PUB foo 1\nx\n"),
	} {
		s := newReviewSession(t, ParserBudget{})
		admitted := s.Feed(1, time.Unix(1, 0), []byte("INFO {\"server_id\":\"local\"}\r\n"))
		require.Nil(t, admitted.Err)
		r := s.Feed(0, time.Unix(1, 0), wire)
		require.NotNil(t, r.Err, string(wire))
		require.Equal(t, ErrMalformedMessage, r.Err.Kind, string(wire))
		require.Len(t, r.Events, 1, string(wire))
		require.Equal(t, "malformed", r.Events[0].Status)
		require.Equal(t, "nats", r.Events[0].Protocol)
	}

	limited := newReviewSession(t, ParserBudget{MaxMessageBytes: 64, MaxBufferedBytes: 1 << 20, ProbeBytes: 16})
	require.Nil(t, limited.Feed(1, time.Unix(1, 0), []byte("INFO {\"server_id\":\"local\"}\r\n")).Err)
	r := limited.Feed(0, time.Unix(1, 0), []byte("PUB foo 100\r\n"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	tooLarge := newReviewSession(t, ParserBudget{MaxMessageBytes: 64, MaxBufferedBytes: 1 << 20, ProbeBytes: 16})
	require.Nil(t, tooLarge.Feed(1, time.Unix(1, 0), []byte("INFO {\"server_id\":\"local\"}\r\n")).Err)
	r = tooLarge.Feed(0, time.Unix(2, 0), []byte("PUB foo 18446744073709551615\r\n"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)

	probe := newReviewSession(t, ParserBudget{})
	require.Equal(t, "nats", probe.Probe([]byte("connect {\"lang\":\"go\"}\r\n")).Protocol)
	require.Equal(t, "http", probe.Probe([]byte("GET / HTTP/1.1\r\nHost: example\r\n\r\n")).Protocol)
	require.Equal(t, "http", probe.Probe([]byte("CONNECT example:443 HTTP/1.1\r\nHost: example:443\r\n\r\n")).Protocol)
	unknown, _ := natsAdmission([]byte("hello there\r\n"))
	require.Equal(t, ProbeReject, unknown.Verdict)
}

func TestProtocolSessionNATSAdmissionDoesNotStealRESPOrGenericHeartbeat(t *testing.T) {
	s := newReviewSession(t, ParserBudget{})
	for _, wire := range [][]byte{
		[]byte("+OK\r\n"),
		[]byte("-ERR unknown command\r\n"),
	} {
		require.NotEqual(t, "nats", s.Probe(wire).Protocol, "%q also occurs outside NATS and cannot identify it alone", wire)
	}
	for _, wire := range [][]byte{
		[]byte("H"), []byte("P"),
		[]byte("PING\r\n"), []byte("PONG\r\n"), []byte("PING\rX"),
		[]byte("SUB foo 1\r\n"), []byte("PUB foo nope\r\n"),
	} {
		require.NotEqual(t, "nats", s.Probe(wire).Protocol, "%q is insufficient as a standalone NATS signature", wire)
	}
}

func TestReplayPcapFileNATSNdpiCorpus(t *testing.T) {
	corpus := filepath.Join("..", "..", "bin-parser", "testdata", "protocol-corpus")
	captureName := "captures/ndpi/ndpi-nats.pcap"
	capture, err := os.ReadFile(filepath.Join(corpus, filepath.FromSlash(captureName)))
	require.NoError(t, err)
	var manifest struct {
		Captures []struct {
			ID          string `json:"id"`
			CaptureFile string `json:"capture_file"`
			SHA256      string `json:"sha256"`
		} `json:"captures"`
	}
	manifestData, err := os.ReadFile(filepath.Join(corpus, "manifest.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(manifestData, &manifest))
	var manifestHash string
	for _, entry := range manifest.Captures {
		if entry.ID == "ndpi-nats" {
			require.Equal(t, captureName, entry.CaptureFile)
			manifestHash = entry.SHA256
			break
		}
	}
	require.NotEmpty(t, manifestHash, "ndpi-nats capture must remain independently pinned in manifest.json")
	var sources struct {
		Captures []struct {
			ID           string `json:"id"`
			SourceSHA256 string `json:"source_sha256"`
		} `json:"captures"`
	}
	sourceData, err := os.ReadFile(filepath.Join(corpus, "sources.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(sourceData, &sources))
	var sourceHash string
	for _, entry := range sources.Captures {
		if entry.ID == "ndpi-nats" {
			sourceHash = entry.SourceSHA256
			break
		}
	}
	require.NotEmpty(t, sourceHash, "ndpi-nats upstream source must remain pinned in sources.json")
	actualHash := sha256.Sum256(capture)
	require.Equal(t, manifestHash, hex.EncodeToString(actualHash[:]))
	require.Equal(t, sourceHash, manifestHash, "manifest capture must match the upstream nDPI source hash")

	var events []*ProtocolEvent
	err = ReplayPcap(bytes.NewReader(capture), WithTCPReassemblyWorkers(1), WithOnProtocolMessage(func(event *ProtocolEvent) {
		if event.Protocol == "nats" {
			events = append(events, event)
		}
	}))
	require.NoError(t, err)
	require.Len(t, events, 10, "the independent two-connection capture carries INFO/CONNECT/PING/PONG exchanges")
	counts := make(map[string]int)
	for _, event := range events {
		require.Equal(t, "decoded", event.Status, "%s: %s", event.Summary, event.Error)
		require.Equal(t, "nats-plaintext-client", event.Profile)
		require.Equal(t, false, event.Session["Payload Decoded"])
		operation, ok := event.Session["Operation"].(string)
		require.True(t, ok, "%v", event.Session)
		counts[operation]++
	}
	require.Equal(t, map[string]int{"INFO": 2, "CONNECT": 2, "PING": 3, "PONG": 3}, counts)
}
