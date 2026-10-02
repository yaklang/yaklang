package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func natsNativeDir() string {
	return filepath.Join("..", "..", "bin-parser", "testdata", "protocol-native", "nats")
}
func natsNativeRead(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(natsNativeDir(), name))
	require.NoError(t, e)
	return b
}

type natsNativePacket struct {
	stream, dir int
	wire        []byte
}

func natsNativePackets(t testing.TB) []natsNativePacket {
	t.Helper()
	var server struct {
		Port int `json:"port"`
	}
	require.NoError(t, json.Unmarshal(natsNativeRead(t, "server-varz.json"), &server))
	require.Positive(t, server.Port)
	records, err := csv.NewReader(bytes.NewReader(natsNativeRead(t, "tshark-tcp.csv"))).ReadAll()
	require.NoError(t, err)
	require.Equal(t, []string{"frame.number", "tcp.stream", "ip.src", "tcp.srcport", "ip.dst", "tcp.dstport", "tcp.seq", "tcp.len", "tcp.payload"}, records[0])
	packets := []natsNativePacket{}
	streams := map[[2]int][]byte{}
	for _, row := range records[1:] {
		require.Len(t, row, 9)
		stream, err := strconv.Atoi(row[1])
		require.NoError(t, err)
		dir := 0
		if row[3] == strconv.Itoa(server.Port) {
			dir = 1
		}
		seq, err := strconv.Atoi(row[6])
		require.NoError(t, err)
		size, err := strconv.Atoi(row[7])
		require.NoError(t, err)
		wire, err := hex.DecodeString(row[8])
		require.NoError(t, err)
		require.Len(t, wire, size)
		key := [2]int{stream, dir}
		old := streams[key]
		start := seq - 1
		require.LessOrEqual(t, start, len(old), "independent TCP oracle must not have a gap")
		overlap := min(len(old)-start, len(wire))
		require.True(t, bytes.Equal(old[start:start+overlap], wire[:overlap]))
		wire = wire[overlap:]
		streams[key] = append(old, wire...)
		if len(wire) > 0 {
			packets = append(packets, natsNativePacket{stream: stream, dir: dir, wire: wire})
		}
	}
	require.Len(t, streams, 4)
	for key, wire := range streams {
		dir := "client"
		if key[1] == 1 {
			dir = "server"
		}
		require.Equal(t, natsNativeRead(t, fmt.Sprintf("connection-%d-%s.bin", key[0]+1, dir)), wire)
	}
	return packets
}

func TestReplayPcapFileNATSNativeHeadersCorpus(t *testing.T) {
	var manifest struct {
		Kind           string            `json:"kind"`
		Files          map[string]string `json:"files"`
		GeneratorFiles map[string]string `json:"generator_files"`
	}
	require.NoError(t, json.Unmarshal(natsNativeRead(t, "manifest.json"), &manifest))
	require.Equal(t, "native", manifest.Kind)
	for file, want := range manifest.Files {
		sum := sha256.Sum256(natsNativeRead(t, file))
		require.Equal(t, want, hex.EncodeToString(sum[:]), file)
	}
	for file, want := range manifest.GeneratorFiles {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "protocol-tests", "generate-nats-native", file))
		require.NoError(t, err)
		sum := sha256.Sum256(data)
		require.Equal(t, want, hex.EncodeToString(sum[:]), "generator source %s must match provenance", file)
	}
	var oracle struct {
		Client       string `json:"nats_go_version"`
		Server       string `json:"server_version"`
		Observations []struct {
			Name, Subject, Reply string
			Headers              map[string][]string `json:"headers"`
			Body                 []byte              `json:"body_base64"`
			Error                string
		}
		Stats map[string]struct{ InMsgs, OutMsgs, InBytes, OutBytes uint64 } `json:"client_stats"`
	}
	require.NoError(t, json.Unmarshal(natsNativeRead(t, "client-oracle.json"), &oracle))
	require.Equal(t, "1.39.1", oracle.Client)
	require.Equal(t, "2.10.26", oracle.Server)
	require.Len(t, oracle.Observations, 6)
	var server struct {
		Version                            string `json:"version"`
		InMsgs, OutMsgs, InBytes, OutBytes uint64
	}
	// Monitoring JSON uses snake_case, unlike nats.go's Statistics fields.
	var varz map[string]any
	require.NoError(t, json.Unmarshal(natsNativeRead(t, "server-varz.json"), &varz))
	var inMsgs, outMsgs, inBytes, outBytes uint64
	for _, stats := range oracle.Stats {
		inMsgs += stats.OutMsgs
		outMsgs += stats.InMsgs
		inBytes += stats.OutBytes
		outBytes += stats.InBytes
	}
	require.Equal(t, float64(inMsgs), varz["in_msgs"])
	// nats-server v2.10.26 client.go's no-responder branch enqueues its
	// 16-byte 503 status directly, outside delivery message/byte counters.
	require.Equal(t, float64(outMsgs-1), varz["out_msgs"])
	require.Equal(t, float64(inBytes), varz["in_bytes"])
	require.Equal(t, float64(outBytes-16), varz["out_bytes"])
	require.NoError(t, json.Unmarshal(natsNativeRead(t, "server-varz.json"), &server))
	require.Equal(t, oracle.Server, server.Version)
	packets := natsNativePackets(t)
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(natsNativeRead(t, "native-headers.pcapng")), WithTCPReassemblyWorkers(1), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.NotEmpty(t, events)
	headers := map[string][]*ProtocolEvent{}
	statuses := map[string]*ProtocolEvent{}
	rawFrames := map[string]int{}
	for _, e := range events {
		require.Equal(t, "nats", e.Protocol)
		require.Equal(t, "decoded", e.Status, "%s: %s", e.Summary, e.Error)
		require.NotNil(t, e.Session)
		rawFrames[string(e.Raw)]++
		op := e.Session["Operation"].(string)
		if op == "HPUB" || op == "HMSG" {
			headers[op] = append(headers[op], e)
			require.Equal(t, "enabled", e.Session["Header Negotiation"])
			require.Equal(t, true, e.Session["Headers Decoded"])
			fields, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, e.Session["Headers"], fields["Headers"])
			if status, ok := e.Session["Header Status"].(string); ok {
				require.NotContains(t, statuses, status)
				statuses[status] = e
			}
		}
	}
	require.Len(t, headers["HPUB"], 3)
	require.Len(t, headers["HMSG"], 6)
	require.Len(t, statuses, 3)
	compare := func(e *ProtocolEvent, at int) {
		o := oracle.Observations[at]
		want := map[string]any{}
		for k, v := range o.Headers {
			want[k] = v
		}
		require.Equal(t, want, e.Session["Headers"], o.Name)
		require.Equal(t, o.Subject, e.Session["Subject"])
		require.Equal(t, len(o.Body), e.Session["Body Length"])
		require.Equal(t, o.Body, e.Raw[len(e.Raw)-2-len(o.Body):len(e.Raw)-2])
		if o.Reply != "" {
			require.Equal(t, o.Reply, e.Session["Reply Subject"])
		}
	}
	for i := 0; i < 3; i++ {
		compare(headers["HPUB"][i], i)
		compare(headers["HMSG"][i], i)
	}
	require.Equal(t, "nats: no responders available for request", oracle.Observations[3].Error)
	require.Contains(t, statuses, "503")
	require.Equal(t, 0, statuses["503"].Session["Body Length"])
	compare(statuses["404"], 4)
	compare(statuses["100"], 5)
	// Fragment the independently captured TCP payloads without rebuilding NATS
	// frames. Their complete raw-frame multiset must agree with ReplayPcap.
	for _, chunk := range []int{1, 7, 64} {
		sessions := map[int]ProtocolSession{}
		got := map[string]int{}
		for _, p := range packets {
			s := sessions[p.stream]
			if s == nil {
				s = newReviewSession(t, ParserBudget{})
				sessions[p.stream] = s
			}
			for wire := p.wire; len(wire) > 0; {
				n := min(chunk, len(wire))
				r := s.Feed(p.dir, time.Time{}, wire[:n])
				wire = wire[n:]
				if r.Err != nil {
					require.Equal(t, ErrNeedMore, r.Err.Kind)
				}
				for _, e := range r.Events {
					require.Equal(t, "decoded", e.Status)
					got[string(e.Raw)]++
				}
			}
		}
		for _, s := range sessions {
			require.Empty(t, s.Close("FIN"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
		require.Equal(t, rawFrames, got, "chunk=%d", chunk)
	}
}

func FuzzNATSStateSequence(f *testing.F) {
	seeds := map[int][]byte{}
	appendRecord := func(dst []byte, dir int, wire []byte) []byte {
		dst = append(dst, byte(dir), byte(len(wire)>>8), byte(len(wire)))
		return append(dst, wire...)
	}
	for _, p := range natsNativePackets(f) {
		seeds[p.stream] = appendRecord(seeds[p.stream], p.dir, p.wire)
	}
	for _, seed := range seeds {
		f.Add(seed, byte(1))
		f.Add(seed, byte(63))
	}
	// Synthetic violation followed by valid-looking control bytes: failure must
	// never leak state or mutate already emitted event snapshots.
	bad := appendRecord(nil, 1, []byte("INFO {\"headers\":true}\r\n"))
	bad = appendRecord(bad, 0, []byte("CONNECT {\"headers\":true}\r\n"))
	bad = appendRecord(bad, 0, append(natsTestHeaderFrame("HPUB", []byte("NATS/1.0\r\nbad\r\n\r\n"), nil), []byte("PING\r\n")...))
	f.Add(bad, byte(3))
	// Synthetic lifecycle records leave a native-shaped header frame partial,
	// then close via FIN or idle timeout before further bytes arrive.
	for _, event := range []int{0x80, 0xc0} {
		tape := appendRecord(nil, 1, []byte("INFO {\"headers\":true}\r\n"))
		tape = appendRecord(tape, 0, []byte("CONNECT {\"headers\":true}\r\n"))
		tape = appendRecord(tape, 0, []byte("HPUB native.partial 12 20\r\nNATS/1.0\r\n\r\nabc"))
		tape = appendRecord(tape, event, nil)
		tape = appendRecord(tape, 1, []byte("PONG\r\n"))
		f.Add(tape, byte(3))
	}
	f.Fuzz(func(t *testing.T, data []byte, fragment byte) {
		if len(data) > 65536 {
			t.Skip()
		}
		budget := ParserBudget{MaxFrameBytes: 8192, MaxMessageBytes: 8192, MaxBufferedBytes: 65536, MaxCollectionElements: 64, MaxRecursionDepth: 8}
		if fragment&128 != 0 {
			budget.MaxBufferedBytes, budget.MaxMessageBytes, budget.MaxFrameBytes, budget.ProbeBytes = 64, 64, 64, 16
		}
		session, err := NewProtocolSession(budget)
		require.NoError(t, err)
		// Exercise the NATS state machine even if mutation destroys admission bytes;
		// native corpus tests separately cover automatic Probe/Feed recognition.
		s := session.(*captureSession)
		s.f.protocol = "nats"
		type saved struct {
			event    *ProtocolEvent
			raw      []byte
			snapshot []byte
		}
		snapshots := []saved{}
		remember := func(events []*ProtocolEvent) {
			for _, e := range events {
				snapshot, err := json.Marshal(e.Session)
				require.NoError(t, err)
				snapshots = append(snapshots, saved{e, bytes.Clone(e.Raw), snapshot})
			}
		}
		closed := false
		defer func() { session.Close("fuzz cleanup") }()
		for steps := 0; len(data) >= 3 && steps < 256; steps++ {
			dir := int(data[0] & 1)
			// Native records use only bit 0. Mutated high bits inject a
			// lifecycle event without changing the original seed behavior.
			if data[0]&0x80 != 0 {
				reason := string(TrafficFlowCloseReason_FIN)
				if data[0]&0x40 != 0 {
					reason = string(TrafficFlowCloseReason_INACTIVE)
				}
				remember(session.Close(reason))
				require.Empty(t, session.Close(reason))
				closed = true
				probe := session.Feed(dir, time.Time{}, []byte("PING\r\n"))
				require.Equal(t, "closed", probe.State)
				require.Empty(t, probe.Events)
				require.Zero(t, session.Stats().BufferedBytes)
			}
			n := min(int(data[1])<<8|int(data[2]), len(data)-3)
			wire := data[3 : 3+n]
			data = data[3+n:]
			for len(wire) > 0 {
				take := min(int(fragment&63)+1, len(wire))
				r := session.Feed(dir, time.Time{}, wire[:take])
				wire = wire[take:]
				require.GreaterOrEqual(t, session.Stats().BufferedBytes, int64(0))
				if closed {
					require.Equal(t, "closed", r.State)
					require.Empty(t, r.Events)
					require.Zero(t, session.Stats().BufferedBytes)
				}
				remember(r.Events)
			}
		}
		remember(session.Close("FIN"))
		require.Empty(t, session.Close("FIN"))
		require.Zero(t, session.Stats().BufferedBytes)
		require.Nil(t, s.f.a.err.Load(), "mutation must not reach panic recovery")
		require.Zero(t, s.f.a.panics.Load())
		for _, saved := range snapshots {
			now, err := json.Marshal(saved.event.Session)
			require.NoError(t, err)
			require.Equal(t, saved.snapshot, now)
			require.Equal(t, saved.raw, saved.event.Raw)
		}
	})
}
