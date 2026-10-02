package bin_parser

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

const winlab5013Root = "testdata/winlab5013"

type winlab5013Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	SourcePR      string `json:"source_pr"`
	SourceCommit  string `json:"source_commit"`
	License       string `json:"license"`
	EvidenceKind  string `json:"evidence_kind"`
	Bundles       []struct {
		Name         string `json:"name"`
		Readme       string `json:"readme"`
		ReadmeSHA256 string `json:"readme_sha256"`
		Captures     []struct {
			File        string `json:"file"`
			SHA256      string `json:"sha256"`
			SizeBytes   int    `json:"size_bytes"`
			PacketCount int    `json:"packet_count"`
		} `json:"captures"`
	} `json:"bundles"`
}

// The archive's own decoder is not evidence of our automatic recognition.
// Decode each captured frame with the shipped Ethernet/transport rules and
// pin every positive application label. Unknown and fragmented bytes must
// remain bytes rather than being claimed by a permissive fallback decoder.
func TestWinlab5013FrameRecognition(t *testing.T) {
	expected := map[string]map[string]int{
		"01-socks5.pcapng":       {"SOCKS5ClientNegotiation": 1, "SOCKS5ServerNegotiation": 1, "SOCKS5Req": 1, "SOCKS5Reply": 1},
		"06-bjnp.pcapng":         {"BJNP": 8},
		"19-gearman.pcapng":      {"Gearman": 7},
		"09-turn.pcapng":         {"STUN": 8},
		"blue-01-stratum.pcapng": {"DNS": 2},
		"ctf-01-dns.pcapng":      {"DNS": 8},
		"ctf-01-ftp.pcapng":      {"FTP": 6, "FTPCommand": 6},
		"power-03-c37118.pcapng": {"C37118": 1},
	}
	root := filepath.Join(winlab5013Root, "captures")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 30)
	for _, entry := range entries {
		entry := entry
		t.Run(entry.Name(), func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, filepath.Join(root, entry.Name()))
			got := make(map[string]int)
			for i, frame := range frames {
				parsed, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame), "ethernet", "Ethernet")
				require.NoErrorf(t, err, "frame %d", i)
				eth, err := parsed.Result()
				require.NoErrorf(t, err, "frame %d", i)
				for _, transport := range []string{"TCP", "UDP"} {
					ip := eth.Child("IP")
					if ip == nil || ip.Child(transport) == nil {
						continue
					}
					for _, child := range ip.Child(transport).Children() {
						if transport == "TCP" {
							switch child.Name {
							case "Source Port", "Destination Port", "Sequence Number", "Acknowledgement Number", "Header Length", "Flags", "Window", "Checksum", "Urgent Pointer", "Options", "Remaining Payload":
								continue
							}
						} else {
							switch child.Name {
							case "Source Port", "Destination Port", "Length", "Checksum", "Remaining Payload":
								continue
							}
						}
						got[child.Name]++
					}
				}
			}
			want := expected[entry.Name()]
			if want == nil {
				want = map[string]int{}
			}
			require.Equal(t, want, got, "unsupported traffic must not be misidentified")
		})
	}
}

func TestWinlab5013SOCKS5InvalidRequestSignatures(t *testing.T) {
	for _, wire := range [][]byte{
		{5, 1, 0},                         // method negotiation, not CONNECT
		{4, 1, 0, 1, 127, 0, 0, 1, 0, 80}, // SOCKS4 version
		{5, 4, 0, 1, 127, 0, 0, 1, 0, 80}, // unsupported command
		{5, 1, 1, 1, 127, 0, 0, 1, 0, 80}, // reserved byte
		{5, 1, 0, 5, 127, 0, 0, 1, 0, 80}, // invalid address family
		{5, 1, 0, 3, 0, 0, 80},            // empty domain
	} {
		parseMustFail(t, wire, "application-layer.socks5", "Request")
	}
}

func TestWinlab5013GearmanInvalidDirectMessages(t *testing.T) {
	for _, wire := range [][]byte{
		{0, 'R', 'E', 'Q', 0, 0, 0, 1, 0, 0, 0, 0},      // CAN_DO needs a function
		{0, 'R', 'E', 'Q', 0, 0, 0, 4, 0, 0, 0, 1, 'x'}, // PRE_SLEEP has no payload
		{0, 'R', 'E', 'Q', 0, 0, 0, 7, 0, 0, 0, 1, 'x'}, // SUBMIT_JOB needs separators
		{0, 'R', 'E', 'S', 0, 0, 0, 8, 0, 0, 0, 2, 'x'}, // declared length mismatch
		{0, 'R', 'E', 'S', 0, 0, 0, 1, 0, 0, 0, 1, 'x'}, // request type in response
	} {
		parseMustFail(t, wire, "application-layer.gearman", "GearmanMessage")
	}
}

func winlab5013CriticalBlocks(t *testing.T, text string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	for _, block := range strings.Split(strings.TrimSpace(text), "\n\n") {
		first, _, _ := strings.Cut(block, "\n")
		require.Truef(t, strings.HasPrefix(first, "file="), "invalid oracle block: %q", first)
		name := strings.TrimPrefix(first, "file=")
		require.NotEmpty(t, name)
		_, duplicate := result[name]
		require.Falsef(t, duplicate, "duplicate oracle block: %s", name)
		result[name] = block + "\n"
	}
	return result
}

func winlab5013ReadmeCritical(t *testing.T, readme string) string {
	t.Helper()
	const begin, end = "BEGIN CRITICAL\n", "END CRITICAL"
	_, tail, found := strings.Cut(readme, begin)
	require.True(t, found, "README has no critical contract")
	critical, _, found := strings.Cut(tail, end)
	require.True(t, found, "README has no critical terminator")
	return critical
}

func winlab5013Go(t *testing.T, args ...string) []byte {
	t.Helper()
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBin += ".exe"
	}
	cmd := exec.Command(goBin, append([]string{"run", "."}, args...)...)
	cmd.Dir = winlab5013Root
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "winlab verifier: %s", out)
	return out
}

// PR #5174 used six opaque zip containers in pcapx. This retained, unpacked
// corpus checks every application-field oracle against the shipped pcapngs.
// It is generated lab evidence, not an independent real-capture benchmark or
// proof that every protocol is already supported by parser.ParseBinary.
func TestWinlab5013Corpus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(winlab5013Root, "manifest.json"))
	require.NoError(t, err)
	var manifest winlab5013Manifest
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, 1, manifest.SchemaVersion)
	require.Equal(t, "https://github.com/yaklang/yaklang/pull/5174", manifest.SourcePR)
	require.Equal(t, "305f34eaa7eb54e59dd59cd1c9bc4a3cb6afc068", manifest.SourceCommit)
	require.Equal(t, "CC0-1.0", manifest.License)
	require.Equal(t, "generated-lab", manifest.EvidenceKind)
	require.Len(t, manifest.Bundles, 6)

	gotBlocks := winlab5013CriticalBlocks(t, string(winlab5013Go(t, "captures")))
	wantBlocks := make(map[string]string)
	wantFiles := make(map[string]bool)
	totalPackets := 0
	for _, bundle := range manifest.Bundles {
		bundle := bundle
		t.Run(bundle.Name, func(t *testing.T) {
			require.NotEmpty(t, bundle.Captures)
			readmePath := filepath.Join(winlab5013Root, bundle.Readme)
			readme, err := os.ReadFile(readmePath)
			require.NoError(t, err)
			require.Equal(t, bundle.ReadmeSHA256, fmt.Sprintf("%x", sha256.Sum256(readme)))
			oracle := winlab5013CriticalBlocks(t, winlab5013ReadmeCritical(t, string(readme)))
			require.Len(t, oracle, len(bundle.Captures))
			for _, capture := range bundle.Captures {
				name := capture.File
				require.Falsef(t, wantFiles[name], "duplicate capture %s", name)
				wantFiles[name] = true
				path := filepath.Join(winlab5013Root, "captures", name)
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, capture.SizeBytes, len(data), name)
				require.Equal(t, capture.SHA256, fmt.Sprintf("%x", sha256.Sum256(data)), name)
				packets := protocolCorpusAuditPackets(t, path)
				require.Equal(t, capture.PacketCount, len(packets), name)
				require.Positive(t, len(packets), name)
				totalPackets += len(packets)
				want, ok := oracle[name]
				require.Truef(t, ok, "missing oracle for %s", name)
				wantBlocks[name] = want
				require.Equal(t, want, gotBlocks[name], name)
			}
		})
	}
	require.Len(t, wantFiles, 30)
	require.Len(t, gotBlocks, 30)
	require.Equal(t, 692, totalPackets)
	entries, err := os.ReadDir(filepath.Join(winlab5013Root, "captures"))
	require.NoError(t, err)
	require.Len(t, entries, len(wantFiles), "untracked or missing capture")
	for _, entry := range entries {
		require.False(t, entry.IsDir())
		require.True(t, wantFiles[entry.Name()], "untracked capture: "+entry.Name())
	}

	// Rebuild the source PR's six archives only in a temporary directory. The
	// checked-in corpus contains the unpacked files, without duplicate zips.
	generated := t.TempDir()
	winlab5013Go(t, "-pack", generated)
	var generatedNames []string
	for _, bundle := range manifest.Bundles {
		archive := filepath.Join(generated, bundle.Name+".zip")
		zr, err := zip.OpenReader(archive)
		require.NoError(t, err)
		generatedNames = append(generatedNames, bundle.Name+".zip")
		seen := make(map[string]bool)
		for _, f := range zr.File {
			if f.Name != "README.md" && !strings.HasPrefix(f.Name, "captures/") {
				continue // original archive also included the source verifier
			}
			r, err := f.Open()
			require.NoError(t, err)
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			require.NoError(t, r.Close())
			path := bundle.Readme
			if f.Name != "README.md" {
				path = f.Name
			}
			want, err := os.ReadFile(filepath.Join(winlab5013Root, path))
			require.NoError(t, err)
			require.True(t, bytes.Equal(want, data), f.Name)
			seen[f.Name] = true
		}
		require.Len(t, seen, len(bundle.Captures)+1)
		require.NoError(t, zr.Close())
	}
	entries, err = os.ReadDir(generated)
	require.NoError(t, err)
	var actualNames []string
	for _, entry := range entries {
		actualNames = append(actualNames, entry.Name())
	}
	sort.Strings(actualNames)
	sort.Strings(generatedNames)
	require.Equal(t, generatedNames, actualNames)
}
