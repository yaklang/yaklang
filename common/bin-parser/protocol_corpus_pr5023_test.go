package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// Only these reviewed, byte-identical record sequences share their existing
// extraction/field/rejection contracts. File hashes and source provenance stay
// distinct. The test below proves equality of every record, not just the chosen
// representative. The three changed sequences are deliberately excluded.
var protocolCorpusPR5023IdenticalPacketIDs = []string{
	"gen-6to4",
	"gen-aoe",
	"gen-cdp",
	"gen-chap",
	"gen-cldap",
	"gen-dhcpv6",
	"gen-dhcpv6-reply",
	"gen-docker-api",
	"gen-eap",
	"gen-eapol",
	"gen-eigrp",
	"gen-etcd",
	"gen-ethernet-8023",
	"gen-ethernet-ii",
	"gen-ftp-data",
	"gen-geneve",
	"gen-icmp-ts",
	"gen-icmpv6",
	"gen-icmpv6-mld",
	"gen-icmpv6-ndp",
	"gen-ieee80211",
	"gen-ieee8021q",
	"gen-ieee8021x",
	"gen-igmp",
	"gen-ipip",
	"gen-ipv6-dstopts",
	"gen-ipv6-frag",
	"gen-ipv6-hbh",
	"gen-ipv6-ra",
	"gen-ipv6-routing",
	"gen-j1939",
	"gen-jdwp",
	"gen-jsonrpc",
	"gen-l2tp",
	"gen-lacp",
	"gen-lcp",
	"gen-ldap",
	"gen-llc",
	"gen-llmnr",
	"gen-llmnr-mdns",
	"gen-loopback",
	"gen-lpd",
	"gen-memcache-bin",
	"gen-minio-s3",
	"gen-mpls",
	"gen-msrpc-epm",
	"gen-nbns",
	"gen-nbt-dg",
	"gen-nbt-ns",
	"gen-netflow-v5",
	"gen-onc-rpc",
	"gen-ospfv3",
	"gen-pppoe-session",
	"gen-prometheus",
	"gen-qinq",
	"gen-rarp",
	"gen-redfish",
	"gen-rip",
	"gen-ripng",
	"gen-rpc",
	"gen-rsvp",
	"gen-sdp",
	"gen-snmpv3",
	"gen-socks4",
	"gen-ssl",
	"gen-udplite",
	"gen-wpad",
	"gen-wpad-proxy",
	"gen-xmlrpc",
}

func init() {
	for _, originalID := range protocolCorpusPR5023IdenticalPacketIDs {
		id := "pr5023-" + originalID
		if spec, ok := protocolCorpusCaptureParseSpecs[originalID]; ok {
			if _, duplicate := protocolCorpusCaptureParseSpecs[id]; duplicate {
				panic("duplicate PR #5023 capture contract: " + id)
			}
			protocolCorpusCaptureParseSpecs[id] = spec
		}
		if spec, ok := protocolCorpusRejectionSpecs[originalID]; ok {
			if _, duplicate := protocolCorpusRejectionSpecs[id]; duplicate {
				panic("duplicate PR #5023 rejection contract: " + id)
			}
			protocolCorpusRejectionSpecs[id] = spec
		}
		for key, values := range protocolCorpusExactValues {
			if strings.HasPrefix(key, originalID+"/") {
				protocolCorpusExactValues["pr5023-"+key] = values
			}
		}
	}
}

func TestProtocolCorpusPR5023EveryRepeatedPacket(t *testing.T) {
	const dir = "testdata/protocol-corpus"
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(dir, "manifest.json"), &manifest)
	captures := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		captures[capture.ID] = capture
	}
	identical := make(map[string]bool)
	for _, originalID := range protocolCorpusPR5023IdenticalPacketIDs {
		require.False(t, identical[originalID], "duplicate identical sequence")
		identical[originalID] = true
	}
	changed := map[string]bool{"gen-sntp": true, "gen-smb2": true, "gen-mariadb": true}
	var sameCount, changedCount, novelCount, records, identicalRecords int
	for _, capture := range manifest.Captures {
		if capture.RepositoryID != "generated-pr5023" {
			continue
		}
		records += capture.PacketCount
		originalID := strings.TrimPrefix(capture.ID, "pr5023-")
		original, exists := captures[originalID]
		if !exists {
			novelCount++
			require.False(t, identical[originalID])
			require.False(t, changed[originalID])
			continue
		}
		t.Run(capture.ID, func(t *testing.T) {
			require.NotEqual(t, original.CaptureFile, capture.CaptureFile)
			require.Equal(t, original.LinkType, capture.LinkType)
			require.Equal(t, original.PacketCount, capture.PacketCount)
			first := protocolCorpusAuditPackets(t, filepath.Join(dir, original.CaptureFile))
			second := protocolCorpusAuditPackets(t, filepath.Join(dir, capture.CaptureFile))
			require.Len(t, second, len(first))
			allEqual := true
			for index := range first {
				equal := bytes.Equal(first[index], second[index])
				if identical[originalID] {
					require.Truef(t, equal, "record %d differs; sharing a contract is no longer justified", index+1)
				}
				allEqual = allEqual && equal
			}
			if identical[originalID] {
				sameCount++
				identicalRecords += len(first)
				require.Equal(t, original.EvidenceKind, capture.EvidenceKind)
				require.Equal(t, original.RoadmapName, capture.RoadmapName)
				require.Equal(t, original.RepresentativeFrame.Number, capture.RepresentativeFrame.Number)
			} else {
				require.True(t, changed[originalID], "new unreviewed regenerated sequence")
				require.False(t, allEqual, "changed sequence became identical; review the provenance ledger")
				changedCount++
			}
		})
	}
	require.Equal(t, 69, sameCount)
	require.Equal(t, 3, changedCount)
	require.Equal(t, 96, novelCount)
	require.Equal(t, 311, records)
	t.Logf("PR #5023: 168 captures / %d records; 69 sequences / %d records byte-identical, 3 changed, 96 new", records, identicalRecords)
}

func TestProtocolCorpusPRMergeOriginalDigestsRetained(t *testing.T) {
	const dir = "testdata/protocol-corpus"
	var audit struct {
		PriorCaptureDigests map[string]string `json:"prior_capture_digests"`
		AddedCaptures       []struct {
			ID      string `json:"retained_id"`
			SHA256  string `json:"sha256"`
			Records int    `json:"records"`
		} `json:"added_captures"`
	}
	// This test reads the immutable retention section, not the report's
	// evolving validation/provenance annotations. Manifests remain strict.
	require.NoError(t, json.Unmarshal(readProtocolCorpusFile(t, dir, "reports/PR_MERGE_AUDIT.json"), &audit))
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(dir, "manifest.json"), &manifest)
	captures := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		captures[capture.ID] = capture
	}
	require.Len(t, audit.PriorCaptureDigests, 311)
	require.Len(t, audit.AddedCaptures, 172)
	check := func(id, digest string) {
		t.Helper()
		capture, exists := captures[id]
		require.True(t, exists, "original capture disappeared: %s", id)
		actual := fmt.Sprintf("%x", sha256.Sum256(readProtocolCorpusFile(t, dir, capture.CaptureFile)))
		require.Equal(t, digest, actual, "original bytes changed: %s", id)
		require.Equal(t, digest, capture.SHA256, "manifest no longer identifies original: %s", id)
	}
	for id, digest := range audit.PriorCaptureDigests {
		check(id, digest)
	}
	seen := make(map[string]bool)
	for _, added := range audit.AddedCaptures {
		require.False(t, seen[added.ID], "duplicate incoming ID")
		seen[added.ID] = true
		_, wasPrior := audit.PriorCaptureDigests[added.ID]
		require.False(t, wasPrior, "incoming artifact overwrote a prior ID")
		check(added.ID, added.SHA256)
		require.Equal(t, added.Records, captures[added.ID].PacketCount)
	}
}

// The three non-identical regenerated sequences do not inherit a contract
// merely because their filenames match. Compare every byte of both versions,
// independently validate IPv4/transport checksums, and parse both full frames.
func TestProtocolCorpusPR5023ChangedSequencesEveryRecord(t *testing.T) {
	for _, name := range []string{"sntp", "smb2", "mariadb"} {
		t.Run(name, func(t *testing.T) {
			const dir = "testdata/protocol-corpus/captures/"
			old := protocolCorpusAuditPackets(t, dir+"generated-local/gen-"+name+".pcap")
			added := protocolCorpusAuditPackets(t, dir+"generated-pr5023/pr5023-gen-"+name+".pcap")
			count := 4
			if name == "sntp" {
				count = 1
			}
			require.Len(t, old, count)
			require.Len(t, added, count)
			for i := range old {
				t.Run(fmt.Sprintf("frame-%d", i+1), func(t *testing.T) {
					require.Len(t, added[i], len(old[i]))
					var changed, want []int
					for offset, value := range old[i] {
						if value != added[i][offset] {
							changed = append(changed, offset)
						}
					}
					switch {
					case name == "sntp":
						want = []int{40, 41, 68, 69, 70, 71, 72, 84, 85, 86, 87, 88}
					case i == 1 || (name == "mariadb" && i == 3):
						want = []int{5, 11} // Only Ethernet address bytes, not TCP/application bytes.
						require.Equal(t, old[i][:6], added[i][6:12])
						require.Equal(t, old[i][6:12], added[i][:6])
					}
					require.Equal(t, want, changed, "unreviewed changes in a regenerated record")
					for version, frame := range [][]byte{old[i], added[i]} {
						t.Run(fmt.Sprintf("version-%d", version), func(t *testing.T) {
							protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
							require.Equal(t, []byte{8, 0}, frame[12:14])
							ip := frame[14:]
							require.Equal(t, byte(0x45), ip[0])
							require.EqualValues(t, len(ip), binary.BigEndian.Uint16(ip[2:4]))
							require.Zero(t, protocolCorpusOnesComplement(ip[:20]), "IPv4 checksum")
							transport := ip[20:]
							pseudo := make([]byte, 12)
							copy(pseudo, ip[12:20])
							pseudo[9] = ip[9]
							binary.BigEndian.PutUint16(pseudo[10:], uint16(len(transport)))
							require.Zero(t, protocolCorpusOnesComplement(append(pseudo, transport...)), "transport pseudo-header checksum")
							if name != "sntp" {
								require.Equal(t, byte(6), ip[9])
								return
							}
							require.Equal(t, byte(17), ip[9])
							require.EqualValues(t, len(transport), binary.BigEndian.Uint16(transport[4:6]))
							wire := transport[8:]
							require.Len(t, wire, 48)
							node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.ntp", "NTP")
							for field, expected := range map[string]uint64{"Leap Indicator": uint64(wire[0] >> 6), "Version": uint64(wire[0] >> 3 & 7), "Mode": uint64(wire[0] & 7), "Stratum": uint64(wire[1]), "Poll": uint64(wire[2]), "Precision": uint64(wire[3])} {
								protocolCorpusRequireValue(t, node, field, expected)
							}
							for field, offset := range map[string]int{"Root Delay": 4, "Root Dispersion": 8} {
								protocolCorpusRequireValue(t, node, field, uint64(binary.BigEndian.Uint32(wire[offset:])))
							}
							protocolCorpusRequireValue(t, node, "Reference ID", wire[12:16])
							for field, offset := range map[string]int{"Reference Timestamp": 16, "Origin Timestamp": 24, "Receive Timestamp": 32, "Transmit Timestamp": 40} {
								protocolCorpusRequireValue(t, node, field, wire[offset:offset+8])
							}
							for cut := 0; cut < len(wire); cut++ {
								_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.ntp", "NTP")
								require.Error(t, err, "truncated SNTP record at byte %d", cut)
							}
						})
					}
				})
			}
		})
	}
}
