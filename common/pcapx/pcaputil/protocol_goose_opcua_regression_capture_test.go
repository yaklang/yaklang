package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"io"

	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestGOOSEOPCUARegressionCaptures(t *testing.T) {
	root := "../../bin-parser/testdata/protocol-recognition-regressions/goose-opcua"
	raw, err := trafficfixture.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		EvidenceKind string `json:"evidence_kind"`
		Captures     []struct {
			File         string   `json:"file"`
			Protocol     string   `json:"protocol"`
			SHA256       string   `json:"sha256"`
			Size         int      `json:"size_bytes"`
			Packets      int      `json:"packet_count"`
			Events       int      `json:"event_count"`
			EvidenceKind string   `json:"evidence_kind"`
			Sources      []string `json:"sources"`
		} `json:"captures"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "synthetic-positive", manifest.EvidenceKind)
	require.Len(t, manifest.Captures, 3)
	for _, capture := range manifest.Captures {
		t.Run(capture.File, func(t *testing.T) {
			require.Equal(t, "synthetic-positive", capture.EvidenceKind)
			require.NotEmpty(t, capture.Sources)
			for _, source := range capture.Sources {
				require.Contains(t, source, "https://")
			}
			wire, err := trafficfixture.ReadFile(filepath.Join(root, capture.File))
			require.NoError(t, err)
			sum := sha256.Sum256(wire)
			require.Equal(t, capture.SHA256, hex.EncodeToString(sum[:]))
			require.Len(t, wire, capture.Size)
			reader, err := pcapgo.NewReader(bytes.NewReader(wire))
			require.NoError(t, err)
			packets := 0
			for {
				_, _, err := reader.ReadPacketData()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				packets++
			}
			require.Equal(t, capture.Packets, packets)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("workers-%d/deferred-%t", workers, deferred), func(t *testing.T) {
						var events []*ProtocolEvent
						err := ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
						require.NoError(t, err)
						require.Len(t, events, capture.Events)
						for i, event := range events {
							require.Equal(t, capture.Protocol, event.Protocol)
							status := "decoded"
							if deferred && capture.Protocol == "opcua" {
								status = "deferred"
							}
							require.Equal(t, status, event.Status, event.Error)
							_, err := event.Decode()
							require.NoError(t, err)
							require.NotEmpty(t, event.SourceBytes.PacketRefs)
							switch capture.File {
							case "goose-vlan.pcap":
								require.Equal(t, []string{"", "/vlan:0", "/vlan:10", "/vlan:100/vlan:10"}[i], event.Domain.Encapsulation)
								require.Equal(t, goosePDU(), event.Raw)
							case "goose-optional-fields.pcap":
								require.Equal(t, gooseOptionalFieldsFixture(t, byte(i)), event.Raw)
								require.Equal(t, false, event.Session["Simulation"])
								require.Equal(t, false, event.Session["Needs Commissioning"])
								if i&1 != 0 {
									require.NotContains(t, event.Session, "GOOSE ID")
								}
							case "opcua-ecc-reverse-connect.pcap":
								require.Equal(t, uaECCReverseConnectSteps(1024)[i].wire, event.Raw)
								require.Equal(t, []string{"RHE", "HEL", "ACK"}[i], event.Session["Message Type"])
								if i == 1 {
									require.Contains(t, event.Source, ":4840")
								} else {
									require.Contains(t, event.Destination, ":4840")
								}
								if i > 0 {
									require.Equal(t, uint32(1024), event.Session["Receive Buffer Size"])
									require.Equal(t, uint32(1024), event.Session["Send Buffer Size"])
								}
							}
						}
					})
				}
			}
		})
	}
}
