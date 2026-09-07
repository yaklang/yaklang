//go:build binparser_aid_probe

// Package aidprobe evaluates the unchanged PCAP script without loading aid,
// contacting a model, opening a live capture, or writing the script's output.
// Its opt-in contracts intentionally fail when the existing tool is incorrect.
package aidprobe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin"
)

type probeCase struct {
	name, capture, sha, filter string
	records, maxPackets        int
	invalidFile                bool
}

type probeResult struct {
	Name          string         `json:"name"`
	Capture       string         `json:"capture"`
	CaptureSHA256 string         `json:"capture_sha256"`
	ScriptSHA256  string         `json:"script_sha256"`
	Filter        string         `json:"filter"`
	MaxPackets    int            `json:"max_packets"`
	OracleRecords int            `json:"oracle_records"`
	OracleMatches int            `json:"oracle_filter_matches"`
	OracleError   string         `json:"oracle_error,omitempty"`
	Callbacks     int            `json:"reader_callbacks"`
	Transport     map[string]int `json:"callback_transport_counts"`
	OpenCalls     int            `json:"open_calls"`
	OpenError     string         `json:"open_error,omitempty"`
	EvalError     string         `json:"eval_error,omitempty"`
	Stats         map[string]int `json:"script_stats"`
	OutputBytes   int            `json:"output_bytes"`
	OutputSHA256  string         `json:"output_sha256"`
	DetailPackets int            `json:"detail_packet_headers"`
	SaveCalls     int            `json:"save_calls"`
	Completed     bool           `json:"script_reported_success"`
	Logs          []string       `json:"script_logs"`
	MarkerHits    []markerHit    `json:"source_dns_marker_hits"`
}

type markerHit struct {
	Frame      int    `json:"frame"`
	ByteOffset int    `json:"packet_byte_offset_zero_based"`
	MarkerHex  string `json:"marker_hex"`
	Transport  string `json:"transport"`
	SourcePort uint16 `json:"source_port"`
	DestPort   uint16 `json:"destination_port"`
}

// Locate the script's DNS byte markers independently, in original file order.
// A marker occurrence alone does not identify a DNS message.
func sourceMarkers(path string) ([]markerHit, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		return nil, err
	}
	hits := []markerHit{}
	for frame := 1; ; frame++ {
		data, _, err := r.ReadPacketData()
		if err == io.EOF {
			return hits, nil
		}
		if err != nil {
			return nil, err
		}
		packet := gopacket.NewPacket(data, r.LinkType(), gopacket.Default)
		for _, marker := range [][]byte{[]byte("DNS"), {1, 0, 0, 1}, {0x81, 0x80}} {
			for start := 0; start < len(data); {
				i := bytes.Index(data[start:], marker)
				if i < 0 {
					break
				}
				offset := start + i
				hit := markerHit{Frame: frame, ByteOffset: offset, MarkerHex: fmt.Sprintf("%x", marker)}
				if tcp, ok := packet.TransportLayer().(*layers.TCP); ok {
					hit.Transport, hit.SourcePort, hit.DestPort = "TCP", uint16(tcp.SrcPort), uint16(tcp.DstPort)
				} else if udp, ok := packet.TransportLayer().(*layers.UDP); ok {
					hit.Transport, hit.SourcePort, hit.DestPort = "UDP", uint16(udp.SrcPort), uint16(udp.DstPort)
				}
				hits = append(hits, hit)
				start = offset + 1
			}
		}
	}
}

// readOracle uses a separate offline reader and libpcap's BPF compiler/matcher.
// The callback being evaluated never provides its own expected packet count.
func readOracle(path, filter string) (records, matches int, readErr error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		return 0, 0, err
	}
	var bpf *pcap.BPF
	if filter != "" {
		bpf, err = pcap.NewBPF(r.LinkType(), int(r.Snaplen()), filter)
		if err != nil {
			return 0, 0, err
		}
	}
	for {
		data, ci, err := r.ReadPacketData()
		if err == io.EOF {
			return records, matches, nil
		}
		if err != nil {
			return records, matches, err
		}
		records++
		if bpf == nil || bpf.Matches(ci, data) {
			matches++
		}
	}
}

func TestAIDPCAPProbe(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate probe source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "../../../.."))
	scriptPath := filepath.Join(root, "common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai/pcap/analyze_pcap.yak")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	const memcached = "captures/ndpi/ndpi-memcached.cap"
	const memcachedSHA = "3a74dd7c9f97e5a7ff75d201014accb76b673d5e13579ddf5faa8bf3844d17bd"
	cases := []probeCase{
		{name: "cassandra", capture: "captures/ndpi/ndpi-cassandra.pcap", sha: "5992c6bbbd1f84fafb84052520e710adec4144fbf5f71b75a8a6d74bad57d155", records: 20, maxPackets: 50000},
		{name: "memcached", capture: memcached, sha: memcachedSHA, records: 10, maxPackets: 50000},
		{name: "binary-original", capture: "captures/generated-local/gen-memcache-bin.pcap", sha: "8ef5179f84123ec6a6fe435fd11a5b3291b5c6f60c8c441edf7980bead85e514", records: 4, maxPackets: 50000},
		{name: "binary-pr5023", capture: "captures/generated-pr5023/pr5023-gen-memcache-bin.pcap", sha: "a08ee0943de03aad2624f017cf876a2091ea07e776c67f90dba70184042479e2", records: 4, maxPackets: 50000},
		{name: "limit-one", capture: memcached, sha: memcachedSHA, records: 10, maxPackets: 1},
		{name: "filter-reject-all", capture: memcached, sha: memcachedSHA, filter: "tcp port 1", records: 10, maxPackets: 50000},
		{name: "invalid-filter", capture: memcached, sha: memcachedSHA, filter: "tcp and (", records: 10, maxPackets: 50000},
		{name: "invalid-file", invalidFile: true, maxPackets: 50000},
	}
	t.Logf("environment: %s %s/%s; libpcap: %s", runtime.Version(), runtime.GOOS, runtime.GOARCH, pcap.Version())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "common/bin-parser/testdata/protocol-corpus", tc.capture)
			if tc.invalidFile {
				path = filepath.Join(t.TempDir(), "invalid.pcap")
				if err := os.WriteFile(path, []byte("not-a-capture\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			gotSHA := fmt.Sprintf("%x", sha256.Sum256(data))
			if !tc.invalidFile && gotSHA != tc.sha {
				t.Fatalf("capture identity changed: got %s, want %s", gotSHA, tc.sha)
			}
			// Always verify capture record count independently of filter compilation.
			allRecords, _, allErr := readOracle(path, "")
			if !tc.invalidFile && (allErr != nil || allRecords != tc.records) {
				t.Fatalf("capture records: got %d, want %d, err %v", allRecords, tc.records, allErr)
			}
			_, matches, oracleErr := readOracle(path, tc.filter)
			result := probeResult{
				Name: tc.name, Capture: tc.capture, CaptureSHA256: gotSHA,
				ScriptSHA256: fmt.Sprintf("%x", sha256.Sum256(script)),
				Filter:       tc.filter, MaxPackets: tc.maxPackets,
				OracleRecords: allRecords, OracleMatches: matches,
				Transport: make(map[string]int),
			}
			if oracleErr != nil {
				result.OracleError = oracleErr.Error()
			}
			if !tc.invalidFile {
				result.MarkerHits, err = sourceMarkers(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			engine := antlr4yak.New()
			engine.SetSourceFilePath(scriptPath)
			var output string
			logf := func(level string) func(string, ...any) {
				return func(format string, args ...any) {
					result.Logs = append(result.Logs, level+": "+fmt.Sprintf(format, args...))
				}
			}
			engine.ImportLibs(map[string]any{
				"append": builtin.Append, "len": builtin.Len, "sprintf": fmt.Sprintf,
				"time": map[string]any{"Now": time.Now, "Since": time.Since},
				// These adapters isolate CLI, UI/webhook logging, and file persistence.
				// They do not replace packet decoding, assembly, filtering, or statistics.
				"cli": map[string]any{
					"String": func(name string, _ ...any) string {
						switch name {
						case "file":
							return path
						case "filter":
							return tc.filter
						default:
							panic("unexpected CLI string: " + name)
						}
					},
					"Int": func(name string, _ ...any) int {
						if name != "max-packets" {
							panic("unexpected CLI integer: " + name)
						}
						return tc.maxPackets
					},
					"setRequired": func(any) any { return nil },
					"setHelp":     func(any) any { return nil },
					"setDefault":  func(any) any { return nil },
					"check":       func() {},
				},
				"yakit": map[string]any{
					"AutoInitYakit": func() {}, "Info": logf("INFO"), "Warn": logf("WARN"), "Error": logf("ERROR"),
				},
				"file": map[string]any{
					"IsExisted": func(name string) bool { _, err := os.Stat(name); return name == path && err == nil },
					"Save": func(name, contents string) error {
						if !strings.HasPrefix(name, "/tmp/pcap_analysis_") || !strings.HasSuffix(name, ".txt") {
							return fmt.Errorf("unexpected output path: %s", name)
						}
						result.SaveCalls++
						output = contents
						return nil
					},
				},
				"str": map[string]any{"Join": func(parts []any, sep string) string {
					ss := make([]string, len(parts))
					for i, part := range parts {
						ss[i] = part.(string) // This script only appends strings.
					}
					return strings.Join(ss, sep)
				}},
				"pcapx": map[string]any{
					"pcap_bpfFilter":   pcaputil.WithBPFFilter,
					"pcap_everyPacket": pcaputil.WithEveryPacket,
					"OpenPcapFile": func(name string, options ...pcaputil.CaptureOption) error {
						if name != path {
							return fmt.Errorf("probe only permits its selected offline capture")
						}
						result.OpenCalls++
						options = append(options, pcaputil.WithContext(ctx), pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
							result.Callbacks++
							if packet.Layer(layers.LayerTypeTCP) != nil {
								result.Transport["tcp"]++
							} else if packet.Layer(layers.LayerTypeUDP) != nil {
								result.Transport["udp"]++
							} else {
								result.Transport["other"]++
							}
						}))
						err := pcaputil.OpenPcapFile(name, options...)
						if err != nil {
							result.OpenError = err.Error()
						}
						return err
					},
				},
			})
			if err := engine.SafeEvalWithoutCache(ctx, string(script)); err != nil {
				result.EvalError = err.Error()
			}
			statsJSON, err := json.Marshal(engine.Var("stats"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(statsJSON, &result.Stats); err != nil {
				t.Fatalf("decode script stats %s: %v", statsJSON, err)
			}
			result.OutputBytes = len(output)
			result.OutputSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(output)))
			result.DetailPackets = strings.Count(output, "=== Packet ")
			for _, line := range result.Logs {
				if line == "INFO: PCAP analysis completed successfully" {
					result.Completed = true
				}
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("PROBE_JSON %s", encoded)
			if ctx.Err() != nil || result.EvalError != "" || result.OpenCalls != 1 || result.Stats == nil {
				t.Fatalf("probe execution incomplete: context=%v eval=%s open_calls=%d stats=%v", ctx.Err(), result.EvalError, result.OpenCalls, result.Stats)
			}
			// Desired contracts, not assertions that bless the current defects.
			if oracleErr != nil {
				if result.OpenError == "" || result.Completed {
					t.Error("contract: invalid capture/filter must return an error and must not report successful analysis")
				}
				return
			}
			want := min(matches, tc.maxPackets)
			if result.OpenError != "" || !result.Completed {
				t.Error("contract: valid offline analysis must finish successfully")
			}
			if result.Stats["total_packets"] != want || result.DetailPackets != want {
				t.Errorf("contract: processed/detail counts should be %d, got %d/%d", want, result.Stats["total_packets"], result.DetailPackets)
			}
			if result.Callbacks > want {
				t.Errorf("resource contract: filter/limit should bound delivered callbacks to %d, got %d (not a hard read limit)", want, result.Callbacks)
			}
			// These pinned captures carry Memcached/Cassandra, not HTTP or DNS.
			for _, key := range []string{"http_requests", "http_responses", "dns_queries", "dns_responses"} {
				if result.Stats[key] != 0 {
					t.Errorf("classification contract: %s should be zero for this capture, got %d", key, result.Stats[key])
				}
			}
		})
	}
}
