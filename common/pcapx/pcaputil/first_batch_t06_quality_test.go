package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

type t06Profile struct {
	name          string
	raw           []byte
	options       []CaptureOption
	expected      map[string]int
	dns           map[string]nativeDNSCanonical
	semanticBytes uint64
}
type t06Result struct {
	Messages, Bytes, Opaque, Unknown, Limited, Drops, Envelopes uint64
	Peak                                                        int64
	Digest, FieldDigest, OrderedDigest                          string
}

func t06RawKey(e *ProtocolEvent) string {
	return fmt.Sprintf("%s/%x", e.Protocol, sha256.Sum256(e.Raw))
}
func t06FilterUDP(t testing.TB, raw []byte) []byte {
	t.Helper()
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	var out bytes.Buffer
	w := pcapgo.NewWriter(&out)
	require.NoError(t, w.WriteFileHeader(65535, r.LinkType()))
	for {
		data, ci, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		packet := gopacket.NewPacket(data, r.LinkType(), gopacket.NoCopy)
		if packet.Layer(layers.LayerTypeUDP) != nil {
			require.NoError(t, w.WritePacket(ci, data))
		}
	}
	return out.Bytes()
}
func t06Profiles(t testing.TB) []t06Profile {
	t.Helper()
	root := filepath.Join("..", "..", "bin-parser", "testdata", "protocol-native", "dns-doh")
	var profiles []t06Profile
	for _, mode := range []string{"dns", "h2"} {
		raw, rows, options := nativeDNSLoad(t, root, mode)
		p := t06Profile{name: "udp-heavy-native", raw: raw, options: options, dns: map[string]nativeDNSCanonical{}}
		if mode == "dns" {
			p.raw = t06FilterUDP(t, raw)
		} else {
			p.name = "tls-h2-native"
		}
		oracleRaw, err := os.ReadFile(filepath.Join(root, mode+"-client-oracle.json"))
		require.NoError(t, err)
		var sizes []struct {
			Transport                 string
			RequestWire, ResponseWire []byte `json:"-"`
			RequestEncoded            []byte `json:"request_wire"`
			ResponseEncoded           []byte `json:"response_wire"`
		}
		require.NoError(t, json.Unmarshal(oracleRaw, &sizes))
		for i, row := range rows {
			transport := "udp"
			if mode == "dns" && row.Transport != "udp" {
				continue
			}
			if mode != "dns" {
				transport = "doh"
			}
			p.dns[nativeDNSKey(row.Request, transport)] = row.Request
			p.dns[nativeDNSKey(row.Response, transport)] = row.Response
			p.semanticBytes += uint64(len(sizes[i].RequestEncoded) + len(sizes[i].ResponseEncoded))
		}
		profiles = append(profiles, p)
	}
	makeHTTP := func(name string, steps []sessionStep) t06Profile {
		p := t06Profile{name: name, raw: sessionTestPCAP(t, steps, 80, 1440, false, false), expected: map[string]int{}}
		for _, s := range steps {
			p.expected[t06RawKey(&ProtocolEvent{Protocol: "http", Raw: s.wire})]++
			p.semanticBytes += uint64(len(s.wire))
		}
		return p
	}
	var steps []sessionStep
	for i := 0; i < 64; i++ {
		steps = append(steps, sessionStep{0, []byte(fmt.Sprintf("GET /item/%d HTTP/1.1\r\nHost: quality.test\r\n\r\n", i))}, sessionStep{1, []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: 8\r\n\r\n%08d", i))})
	}
	profiles = append(profiles, makeHTTP("long-connection-synthetic", steps))
	body := bytes.Repeat([]byte("large body evidence\n"), 16384)
	profiles = append(profiles, makeHTTP("large-body-synthetic", []sessionStep{{0, []byte("GET /large HTTP/1.1\r\nHost: quality.test\r\n\r\n")}, {1, append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", len(body))), body...)}}))
	mix, _, _ := binBenchmarkCaptureFlows(t, true, 12)
	http := []byte("POST /events HTTP/1.1\r\nHost: example.test\r\nContent-Length: 512\r\n\r\n" + string(bytes.Repeat([]byte{'x'}, 512)))
	publish := binMQTTPublish(512)
	profiles = append(profiles, t06Profile{name: "mixed-small-synthetic", raw: mix, semanticBytes: uint64(512*(len(http)+len(publish)) + 4*len(binMQTTConnect)), expected: map[string]int{t06RawKey(&ProtocolEvent{Protocol: "http", Raw: http}): 512, t06RawKey(&ProtocolEvent{Protocol: "mqtt", Raw: publish}): 512, t06RawKey(&ProtocolEvent{Protocol: "mqtt", Raw: binMQTTConnect}): 4}})
	return profiles
}

func t06CheckFields(t testing.TB, e *ProtocolEvent, fields map[string]any) {
	t.Helper()
	scalar := func(name string) string {
		value := findSessionField(fields, name)
		if bytes, ok := value.([]byte); ok {
			return string(bytes)
		}
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
	switch e.Protocol {
	case "http":
		first, _, _ := bytes.Cut(e.Raw, []byte("\r\n"))
		columns := strings.Split(string(first), " ")
		if bytes.HasPrefix(first, []byte("HTTP/")) {
			require.Equal(t, columns[1], scalar("Status"))
		} else {
			require.Equal(t, columns[0], scalar("Method"))
			require.Equal(t, columns[1], scalar("Path"))
		}
		_, body, _ := bytes.Cut(e.Raw, []byte("\r\n\r\n"))
		require.Equal(t, string(body), scalar("Octets"))
	case "mqtt":
		if e.Raw[0]>>4 == 3 {
			require.Equal(t, "a", scalar("Topic Name"))
			require.Equal(t, strings.Repeat("x", 512), scalar("Application Message"))
		} else {
			require.Equal(t, "MQTT", scalar("Protocol Name"))
			require.Equal(t, "id", scalar("Client Identifier"))
		}
	default:
		t.Fatalf("missing independent field oracle for %s", e.Protocol)
	}
}

func t06Replay(t testing.TB, p t06Profile, workers int, mode string) t06Result {
	t.Helper()
	var mu sync.Mutex
	observed := map[string]int{}
	actualDNS := map[string]nativeDNSCanonical{}
	fieldDigests := map[string]int{}
	ordered := map[string][]string{}
	result := t06Result{Bytes: p.semanticBytes}
	var stats BinParserStats
	var reassembly TCPReassemblyStats
	options := append([]CaptureOption{}, p.options...)
	options = append(options, WithTCPReassemblyWorkers(workers), WithBinParserDeferred(mode == "deferred-fields" || mode == "framed-capture"), WithBinParserStats(func(s BinParserStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { reassembly = s }), WithBinParser(func(e *ProtocolEvent) {
		mu.Lock()
		defer mu.Unlock()
		require.Empty(t, e.Error, "%s: %s", p.name, e.Summary)
		if e.Session["Content Visibility"] == "encrypted" {
			result.Opaque++
			return
		}
		if p.dns != nil {
			dns, ok := e.Session["DNS"].(map[string]any)
			if !ok {
				if p.name == "tls-h2-native" {
					nativeDNSRequireEnvelope(t, "h2", e)
				} else {
					t.Errorf("unexpected %s event without DNS semantics", e.Protocol)
				}
				result.Envelopes++
				return
			}
			c := nativeDNSCanonicalize(t, dns)
			transport := "udp"
			if p.name == "tls-h2-native" {
				transport = "doh"
			}
			key := nativeDNSKey(c, transport)
			require.NotContains(t, actualDNS, key)
			actualDNS[key] = c
		} else {
			observed[t06RawKey(e)]++
		}
		result.Messages++
		source, destination := e.Source, e.Destination
		if source > destination {
			source, destination = destination, source
		}
		flow := fmt.Sprintf("%#v/%s/%s/%s", e.Domain, e.Transport, source, destination)
		ordered[flow] = append(ordered[flow], e.Source+"->"+e.Destination+"/"+t06RawKey(e))
		var fields map[string]any
		if mode != "framed-capture" {
			var err error
			fields, err = e.GetFields()
			require.NoError(t, err)
			require.NotEmpty(t, fields)
			if p.dns == nil {
				t06CheckFields(t, e, fields)
			}
			encoded, err := json.Marshal(fields)
			require.NoError(t, err)
			fieldDigests[t06RawKey(e)+fmt.Sprintf("/%x", sha256.Sum256(encoded))]++
		}
		if mode == "export" {
			_, err := json.Marshal(struct {
				Protocol string
				Fields   any
				Raw      []byte
			}{e.Protocol, fields, e.Raw})
			require.NoError(t, err)
		}
	}))
	require.NoError(t, ReplayPcap(bytes.NewReader(p.raw), options...))
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.Malformed)
	require.Zero(t, stats.CallbackPanics)
	require.Zero(t, stats.LimitedBytes)
	if p.dns != nil {
		require.Equal(t, p.dns, actualDNS)
	} else {
		require.Equal(t, p.expected, observed)
	}
	result.Unknown = stats.Unknown
	result.Limited = stats.LimitedBytes
	result.Peak = stats.PeakBufferedBytes
	result.Drops = reassembly.RejectedPackets
	canonical, err := json.Marshal(struct {
		Raw map[string]int
		DNS map[string]nativeDNSCanonical
	}{observed, actualDNS})
	require.NoError(t, err)
	result.Digest = fmt.Sprintf("%x", sha256.Sum256(canonical))
	sequence, err := json.Marshal(ordered)
	require.NoError(t, err)
	result.OrderedDigest = fmt.Sprintf("%x", sha256.Sum256(sequence))
	if mode != "framed-capture" {
		encoded, err := json.Marshal(fieldDigests)
		require.NoError(t, err)
		result.FieldDigest = fmt.Sprintf("%x", sha256.Sum256(encoded))
	}
	require.Zero(t, result.Unknown)
	require.Zero(t, result.Drops)
	return result
}

func TestT06MixedTrafficContract(t *testing.T) {
	old := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(old)
	for _, p := range t06Profiles(t) {
		var expected t06Result
		for _, procs := range []int{1, 2, 4} {
			runtime.GOMAXPROCS(procs)
			for _, workers := range []int{1, 2, 4} {
				for _, mode := range []string{"full", "framed-capture", "deferred-fields", "export"} {
					t.Run(fmt.Sprintf("%s/procs%d/workers%d/%s", p.name, procs, workers, mode), func(t *testing.T) {
						got := t06Replay(t, p, workers, mode)
						require.Positive(t, got.Messages)
						if expected.Messages == 0 {
							expected = got
						} else {
							require.Equal(t, expected.Digest, got.Digest)
							require.Equal(t, expected.OrderedDigest, got.OrderedDigest, "per-flow message order changed")
							if mode != "framed-capture" {
								require.Equal(t, expected.FieldDigest, got.FieldDigest)
							}
							require.Equal(t, expected.Messages, got.Messages)
							require.Equal(t, expected.Bytes, got.Bytes)
							require.Equal(t, expected.Opaque, got.Opaque)
						}
					})
				}
			}
		}
	}
}
func t06ReleaseCycles(t testing.TB, count int) [4][2]int {
	t.Helper()
	var closed [4][2]int
	profiles := [][]sessionStep{
		{{0, []byte("CONNECT {\"headers\":true}\r\n")}, {0, []byte("HPUB sample 12 15\r\nNATS/1.0\r\n\r\nx")}},
		{{0, []byte("STOMP\naccept-version:1.2\nhost:quality.test\n\n\x00")}, {1, []byte("CONNECTED\nversion:1.2\n\n\x00")}, {0, []byte("SUBSCRIBE\nid:s\ndestination:/q\nack:client-individual\n\n\x00BEGIN\ntransaction:t\n\n\x00")}},
		uaECCReverseConnectSteps(8192),
		{{0, dohGET("quality.test", "pending.quality.test", 0)}},
	}
	for i := 0; i < count; i++ {
		session, err := NewProtocolSession(ParserBudget{MaxFrameBytes: 32768, MaxMessageBytes: 32768, MaxBufferedBytes: 65536})
		require.NoError(t, err)
		for _, step := range profiles[i%len(profiles)] {
			result := session.Feed(step.dir, time.Unix(int64(i), 0), step.wire)
			if result.Err != nil {
				require.Equal(t, ErrNeedMore, result.Err.Kind)
			}
		}
		reason := "FIN"
		if (i/len(profiles))%2 == 0 {
			reason = "idle timeout"
		}
		session.Close(reason)
		require.Zero(t, session.Stats().BufferedBytes)
		require.Empty(t, session.Close(reason))
		require.Empty(t, session.(*captureSession).events)
		closed[i%len(profiles)][(i/len(profiles))%2]++
	}
	return closed
}
func TestT06RepeatedSessionRelease(t *testing.T) { t06ReleaseCycles(t, 1000) }

func TestT06LargeBodySplitInvariance(t *testing.T) {
	body := bytes.Repeat([]byte("large body evidence\n"), 16384)
	steps := []sessionStep{{0, []byte("GET /large HTTP/1.1\r\nHost: quality.test\r\n\r\n")}, {1, append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", len(body))), body...)}}
	var baseline []string
	for _, deferred := range []bool{false, true} {
		for _, seed := range []int64{0, 1, 17, 691} {
			t.Run(fmt.Sprintf("deferred%t/seed%d", deferred, seed), func(t *testing.T) {
				session, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				session.(*captureSession).f.a.config.Deferred = deferred
				defer session.Close("FIN")
				random := rand.New(rand.NewSource(seed))
				var events []*ProtocolEvent
				for _, step := range steps {
					wire := step.wire
					for len(wire) > 0 {
						n := len(wire)
						if seed == 1 {
							n = 1
						} else if seed != 0 {
							n = min(n, 1+random.Intn(16384))
						}
						result := session.Feed(step.dir, time.Unix(1, 0), wire[:n])
						if result.Err != nil {
							require.Equal(t, ErrNeedMore, result.Err.Kind)
						}
						events = append(events, result.Events...)
						wire = wire[n:]
					}
				}
				require.Len(t, events, len(steps))
				var canonical []string
				for i, event := range events {
					require.Empty(t, event.Error)
					require.Equal(t, steps[i].dir, event.Direction)
					require.Equal(t, steps[i].wire, event.Raw)
					fields, err := event.GetFields()
					require.NoError(t, err)
					t06CheckFields(t, event, fields)
					encoded, err := json.Marshal(fields)
					require.NoError(t, err)
					canonical = append(canonical, fmt.Sprintf("%s/%x", t06RawKey(event), sha256.Sum256(encoded)))
				}
				if baseline == nil {
					baseline = canonical
				} else {
					require.Equal(t, baseline, canonical)
				}
				session.Close("FIN")
				require.Zero(t, session.Stats().BufferedBytes)
			})
		}
	}
}

// Opt-in only. Ordinary test runs never spend 30 minutes here. The caller owns
// sustained-run duration and evidence destination, independently of CI filters.
func TestT06MixedTrafficSoak(t *testing.T) {
	value := os.Getenv("YAK_T06_SOAK_DURATION")
	if value == "" {
		t.Skip("manual sustained quality tier")
	}
	duration, err := time.ParseDuration(value)
	require.NoError(t, err)
	require.GreaterOrEqual(t, duration, 30*time.Minute)
	file, err := os.Create(os.Getenv("YAK_T06_SOAK_LOG"))
	require.NoError(t, err)
	defer file.Close()
	encoder := json.NewEncoder(file)
	profiles := t06Profiles(t)
	for _, p := range profiles {
		t06Replay(t, p, 1, "full")
	}
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	initialHeap, initialGoroutines := memory.HeapAlloc, runtime.NumGoroutine()
	started := time.Now()
	deadline := started.Add(duration)
	procs := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(procs)
	process, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(t, err)
	cycles, loops := 0, 0
	var closed [4][2]int
	coverage := map[string]uint64{}
	var messages, semanticBytes, opaque uint64
	nextSample := started
	for time.Now().Before(deadline) || cycles < 1000000 {
		runtime.GOMAXPROCS([]int{1, 2, 4}[loops%3])
		workers := []int{1, 2, 4}[(loops/3)%3]
		mode := []string{"full", "deferred-fields", "export"}[(loops/9)%3]
		coverage[fmt.Sprintf("procs%d/workers%d/%s", runtime.GOMAXPROCS(0), workers, mode)]++
		for _, p := range profiles {
			result := t06Replay(t, p, workers, mode)
			messages += result.Messages
			semanticBytes += result.Bytes
			opaque += result.Opaque
			if loops == 0 {
				require.NoError(t, encoder.Encode(map[string]any{"profile": p.name, "result": result}))
			}
		}
		if cycles < 1000000 {
			batch := t06ReleaseCycles(t, 1000)
			for profile := range closed {
				for reason := range closed[profile] {
					closed[profile][reason] += batch[profile][reason]
				}
			}
			cycles += 1000
		}
		loops++
		if time.Now().After(nextSample) {
			runtime.GC()
			runtime.ReadMemStats(&memory)
			rss, err := process.MemoryInfo()
			require.NoError(t, err)
			cpu, err := process.Times()
			require.NoError(t, err)
			require.LessOrEqual(t, memory.HeapAlloc, initialHeap+32<<20, "retained heap grew after full close and GC")
			require.LessOrEqual(t, runtime.NumGoroutine(), initialGoroutines+16, "worker goroutines accumulated")
			require.NoError(t, encoder.Encode(map[string]any{"elapsed_seconds": time.Since(started).Seconds(), "loops": loops, "release_cycles": cycles, "heap_alloc": memory.HeapAlloc, "heap_sys": memory.HeapSys, "rss": rss.RSS, "goroutines": runtime.NumGoroutine(), "cpu_seconds": cpu.User + cpu.System, "gomaxprocs": runtime.GOMAXPROCS(0), "workers": workers, "mode": mode, "semantic_messages": messages, "semantic_bytes": semanticBytes, "opaque": opaque}))
			require.NoError(t, file.Sync())
			nextSample = time.Now().Add(30 * time.Second)
		}
	}
	require.Len(t, coverage, 27)
	for _, profile := range closed {
		require.Equal(t, [2]int{125000, 125000}, profile)
	}
	require.NoError(t, encoder.Encode(map[string]any{"complete": true, "elapsed_seconds": time.Since(started).Seconds(), "loops": loops, "release_cycles": cycles, "close_counts_by_profile_and_reason": closed, "coverage": coverage, "semantic_messages": messages, "semantic_bytes": semanticBytes, "opaque": opaque}))
}

// -cpu varies scheduler capacity; workers remain an independent dimension.
// Captured bytes and effective semantic bytes are separate metrics. TLS record
// envelopes/ciphertext are never counted as decoded application messages.
func BenchmarkT06Pipeline(b *testing.B) {
	for _, p := range t06Profiles(b) {
		for _, workers := range []int{1, 2, 4} {
			for _, mode := range []string{"full", "framed-capture", "deferred-fields", "export"} {
				b.Run(fmt.Sprintf("%s/workers%d/%s", p.name, workers, mode), func(b *testing.B) {
					proc, err := process.NewProcess(int32(os.Getpid()))
					require.NoError(b, err)
					before, err := proc.Times()
					require.NoError(b, err)
					samples := make([]int64, 0, 4096)
					var last t06Result
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						start := time.Now()
						last = t06Replay(b, p, workers, mode)
						if len(samples) < cap(samples) {
							samples = append(samples, time.Since(start).Nanoseconds())
						}
					}
					b.StopTimer()
					sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
					for _, q := range []int{50, 95, 99} {
						b.ReportMetric(float64(samples[(len(samples)-1)*q/100]), "p"+strconv.Itoa(q)+"-ns/batch")
					}
					after, err := proc.Times()
					require.NoError(b, err)
					rss, err := proc.MemoryInfo()
					require.NoError(b, err)
					var mem runtime.MemStats
					runtime.ReadMemStats(&mem)
					b.ReportMetric((after.User+after.System-before.User-before.System)*1e9/float64(b.N), "cpu-ns/batch")
					b.ReportMetric(float64(rss.RSS), "rss-bytes")
					b.ReportMetric(float64(mem.HeapAlloc), "heap-bytes")
					b.ReportMetric(float64(runtime.GOMAXPROCS(0)), "gomaxprocs")
					if mode == "framed-capture" {
						b.ReportMetric(float64(last.Messages), "framed-msg/batch")
						b.ReportMetric(0, "semantic-msg/batch")
					} else {
						b.ReportMetric(float64(last.Messages), "semantic-msg/batch")
					}
					if mode == "framed-capture" {
						b.ReportMetric(0, "semantic-bytes/batch")
					} else {
						b.ReportMetric(float64(last.Bytes), "semantic-bytes/batch")
					}
					b.ReportMetric(float64(len(p.raw)), "capture-bytes/batch")
					b.ReportMetric(float64(last.Opaque), "opaque/batch")
					b.ReportMetric(float64(last.Envelopes), "envelopes/batch")
					b.ReportMetric(float64(last.Unknown), "unknown/batch")
					b.ReportMetric(float64(last.Limited), "limited-bytes/batch")
					b.ReportMetric(float64(last.Drops), "rejected/batch")
					b.ReportMetric(float64(last.Peak), "peak-buffer-bytes")
					if destination := os.Getenv("YAK_T06_BENCH_EVIDENCE"); destination != "" {
						file, err := os.OpenFile(destination, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
						require.NoError(b, err)
						err = json.NewEncoder(file).Encode(map[string]any{"benchmark": b.Name(), "gomaxprocs": runtime.GOMAXPROCS(0), "iterations": b.N, "ns_per_batch": float64(b.Elapsed().Nanoseconds()) / float64(b.N), "result": last, "latencies_ns": samples, "heap_alloc": mem.HeapAlloc, "rss": rss.RSS})
						require.NoError(b, err)
						require.NoError(b, file.Close())
					}
				})
			}
		}
	}
}
func BenchmarkT06CaptureReader(b *testing.B) {
	for _, p := range t06Profiles(b) {
		b.Run(p.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(p.raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reader, err := NewCaptureReader(bytes.NewReader(p.raw))
				require.NoError(b, err)
				count := 0
				for {
					_, _, err := reader.ReadPacketData()
					if err == io.EOF {
						break
					}
					require.NoError(b, err)
					count++
				}
				if count == 0 {
					b.Fatal("empty capture")
				}
			}
		})
	}
}
