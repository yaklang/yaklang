package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These original captures come from pinned, independent native stacks, with
// actual sockets/veth, endpoint success oracles and a separate TShark oracle.
// The generator configures public APIs; it does not construct protocol packets.
func TestIndustrialNativeGOOSEOPCUACaptures(t *testing.T) {
	root := "../../bin-parser/testdata/protocol-native/industrial"
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Evidence string `json:"evidence_kind"`
		Captures []struct {
			File     string          `json:"file"`
			Protocol string          `json:"protocol"`
			SHA      string          `json:"sha256"`
			Messages int             `json:"message_count"`
			Exit     int             `json:"endpoint_exit_code"`
			Drops    int             `json:"kernel_drops"`
			Reverse  bool            `json:"reverse"`
			Policy   string          `json:"policy"`
			Variant  int             `json:"variant"`
			Oracle   json.RawMessage `json:"endpoint_oracle"`
		} `json:"captures"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "independent-native-capture", manifest.Evidence)
	require.Len(t, manifest.Captures, 10)
	for _, capture := range manifest.Captures {
		t.Run(capture.File, func(t *testing.T) {
			wire, err := os.ReadFile(filepath.Join(root, capture.File))
			require.NoError(t, err)
			sum := sha256.Sum256(wire)
			require.Equal(t, capture.SHA, hex.EncodeToString(sum[:]))
			require.Zero(t, capture.Exit)
			require.Zero(t, capture.Drops)
			var gooseOracle []string
			if capture.Protocol == "goose" {
				require.NoError(t, json.Unmarshal(capture.Oracle, &gooseOracle))
				require.Len(t, gooseOracle, capture.Messages)
			}
			var baseline [32]byte
			initialized := false
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("workers-%d/deferred-%t", workers, deferred), func(t *testing.T) {
						events, stats, err := binReplay(t, wire, workers, WithProtocolDeferred(deferred))
						require.NoError(t, err)
						require.Len(t, events, capture.Messages)
						require.Zero(t, stats.BufferedBytes)
						protected, rhe, reads := 0, 0, 0
						var comparison []string
						for i, e := range events {
							require.Equal(t, capture.Protocol, e.Protocol)
							require.Empty(t, e.Error)
							status := "decoded"
							if deferred && e.Protocol == "opcua" {
								status = "deferred"
							}
							require.Equal(t, status, e.Status)
							decoded, err := e.Decode()
							require.NoError(t, err)
							require.NotEmpty(t, e.SourceBytes.PacketRefs)
							record, err := json.Marshal([]any{e.Protocol, e.Source, e.Destination, e.Raw, e.Session})
							require.NoError(t, err)
							comparison = append(comparison, string(record))
							if e.Protocol == "goose" {
								// Values are chosen by the independent libiec61850 publisher
								// and confirmed by its subscriber, not inferred from our parser.
								values, ok := findSessionField(protocolFields(decoded), "Dataset Values").([]any)
								require.True(t, ok)
								require.Len(t, values, 3)
								integer := values[0].(map[string]any)
								boolean := values[1].(map[string]any)
								visible := values[2].(map[string]any)
								require.EqualValues(t, 0x85, integer["Tag"])
								require.Equal(t, []byte{0x04, 0xd2}, integer["Encoded Value"])
								require.EqualValues(t, 0x83, boolean["Tag"])
								require.EqualValues(t, 1, boolean["Boolean"])
								require.Equal(t, []bool{true}, e.Session["Boolean"])
								require.EqualValues(t, 0x8a, visible["Tag"])
								require.Equal(t, []byte("native-libiec61850"), visible["Encoded Value"])
								// Integer and visible-string retain their BER value bytes in
								// the public field tree; do not claim typed session decoding.
								oracle := fmt.Sprintf("ORACLE valid=1 stNum=%d sqNum=%d values={%d,%t,%s}",
									e.Session["State Number"], e.Session["Sequence Number"],
									binary.BigEndian.Uint16(integer["Encoded Value"].([]byte)),
									e.Session["Boolean"].([]bool)[0], visible["Encoded Value"].([]byte))
								require.Equal(t, gooseOracle[i], oracle)
								require.Equal(t, 3, e.Session["Dataset Entry Count"])
								require.Equal(t, capture.Variant == 2, e.Session["Simulation"])
								require.Equal(t, capture.Variant == 2, e.Session["Needs Commissioning"])
								require.Equal(t, i, e.Session["Sequence Number"])
								require.Equal(t, []string{"", "/vlan:0", "/vlan:10"}[capture.Variant], e.Domain.Encapsulation)
								if capture.Variant < 2 {
									require.Equal(t, "pr5013/LLN0$GO$native", e.Session["GOOSE ID"])
								}
								continue
							}
							if e.Session["Message Type"] == "RHE" {
								rhe++
								require.True(t, capture.Reverse)
								require.Contains(t, e.Session["Endpoint URL"], "127.0.0.1:14840")
							}
							if e.Session["Service Name"] == "ReadResponse" {
								reads++
							}
							if e.Session["Semantic Status"] == "security-context-required" {
								protected++
								require.Equal(t, false, e.Session["Content Decoded"])
								require.NotContains(t, e.Session, "Service Type ID")
								require.NotContains(t, e.Session, "Service Name")
								if e.Session["Message Type"] == "OPN" {
									require.Equal(t, "http://opcfoundation.org/UA/SecurityPolicy#"+capture.Policy, e.Session["Security Policy URI"])
								}
							}
						}
						if capture.Protocol == "opcua" {
							if capture.Policy == "None" {
								require.Zero(t, protected)
								require.Positive(t, reads)
							} else {
								require.Positive(t, protected)
							}
							if capture.Reverse {
								require.Equal(t, 1, rhe)
							} else {
								require.Zero(t, rhe)
							}
						}
						sort.Strings(comparison) // Separate flows may be delivered in any worker order.
						encoded, err := json.Marshal(comparison)
						require.NoError(t, err)
						digest := sha256.Sum256(encoded)
						if !initialized {
							initialized = true
							baseline = digest
						} else {
							require.Equal(t, baseline, digest)
						}
					})
				}
			}
		})
	}
}

func TestIndustrialIndependentUpstreamGOOSEOPCUA(t *testing.T) {
	root := "../../bin-parser/testdata/protocol-corpus/captures/"
	for _, c := range []struct {
		file, sha string
		count     int
		opaque    bool
	}{
		{"iti-ics/iti-goose.pcap", "746619df546905feb6f1217ee40750004a1ca1d85fe7be1740bacdf52dca6850", 8, false},
		{"ndpi/ndpi-opcua.pcap", "46eff2793ee3105a7d478fc425a99e4cea9946a16194a2c3d95876300b2cf197", 187, false},
		{"wireshark-tests/ws-opcua-signed.pcapng", "a2aaa4a74040dd079de4358131864725f01634bf53477f1a23485f6a0acfc7d2", 51, true},
	} {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(root + c.file)
			require.NoError(t, err)
			sum := sha256.Sum256(raw)
			require.Equal(t, c.sha, hex.EncodeToString(sum[:]))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					events, stats, err := binReplay(t, raw, workers, WithProtocolDeferred(deferred))
					require.NoError(t, err)
					require.Len(t, events, c.count)
					require.Zero(t, stats.BufferedBytes)
					protected := 0
					for _, e := range events {
						require.Empty(t, e.Error)
						require.Contains(t, []string{"decoded", "deferred"}, e.Status)
						_, err = e.Decode()
						require.NoError(t, err)
						if e.Session["Semantic Status"] == "security-context-required" {
							protected++
							require.Equal(t, false, e.Session["Content Decoded"])
							require.NotContains(t, e.Session, "Service Type ID")
						}
						if strings.HasPrefix(c.file, "iti-") {
							require.Equal(t, "goose", e.Protocol)
						} else {
							require.Equal(t, "opcua", e.Protocol)
						}
					}
					if c.opaque {
						require.Positive(t, protected)
					}
				}
			}
		})
	}
}
