package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcapgo"
)

type protocolCorpusManifest struct {
	SchemaVersion int                        `json:"schema_version"`
	RoadmapTotal  int                        `json:"roadmap_total"`
	Repositories  []protocolCorpusRepository `json:"repositories"`
	Captures      []protocolCorpusCapture    `json:"captures"`
}

type protocolCorpusRepository struct {
	ID            string `json:"id"`
	Repository    string `json:"repository"`
	Commit        string `json:"commit"`
	License       string `json:"license"`
	Homepage      string `json:"homepage"`
	LicenseFile   string `json:"license_file"`
	LicenseSHA256 string `json:"license_sha256"`
}

type protocolCorpusCapture struct {
	ID                  string               `json:"id"`
	RepositoryID        string               `json:"repository_id"`
	Protocol            string               `json:"protocol"`
	RoadmapName         *string              `json:"roadmap_name"`
	RoadmapFamily       *string              `json:"roadmap_family"`
	RoadmapPriority     *string              `json:"roadmap_priority"`
	RoadmapStatus       *string              `json:"roadmap_status"`
	CaptureFile         string               `json:"capture_file"`
	SourceURL           string               `json:"source_url"`
	UpstreamPath        string               `json:"upstream_path"`
	SHA256              string               `json:"sha256"`
	SizeBytes           int64                `json:"size_bytes"`
	PacketCount         int                  `json:"packet_count"`
	LinkType            string               `json:"link_type"`
	DisplayFilter       string               `json:"display_filter"`
	EvidenceKind        string               `json:"evidence_kind"`
	Notes               string               `json:"notes"`
	RepresentativeFrame *protocolCorpusFrame `json:"representative_frame"`
	FrameProtocols      []string             `json:"frame_protocols"`
}

type protocolCorpusFrame struct {
	Number      int    `json:"number"`
	LengthBytes int    `json:"length_bytes"`
	HexFile     string `json:"hex_file"`
}

type protocolCorpusSourceSpec struct {
	Schema        string            `json:"$schema"`
	SchemaVersion int               `json:"schema_version"`
	Repositories  []json.RawMessage `json:"repositories"`
	Captures      []struct {
		ID            string  `json:"id"`
		RepositoryID  string  `json:"repository_id"`
		UpstreamPath  string  `json:"upstream_path"`
		Protocol      string  `json:"protocol"`
		RoadmapName   *string `json:"roadmap_name"`
		DisplayFilter string  `json:"display_filter"`
		EvidenceKind  string  `json:"evidence_kind"`
		Notes         string  `json:"notes"`
		AllowEmpty    bool    `json:"allow_empty"`
	} `json:"captures"`
}

type protocolCorpusPacketReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
}

func TestProtocolCorpusIntegrity(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"

	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	var sourceSpec protocolCorpusSourceSpec
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "sources.json"), &sourceSpec)
	for _, schemaName := range []string{"source-spec.schema.json", "manifest.schema.json"} {
		var schema map[string]any
		readProtocolCorpusJSON(t, filepath.Join(corpusDir, schemaName), &schema)
		if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("%s does not declare JSON Schema 2020-12", schemaName)
		}
	}

	if manifest.SchemaVersion != 1 || sourceSpec.SchemaVersion != 1 {
		t.Fatalf("unexpected corpus schema versions: manifest=%d sources=%d", manifest.SchemaVersion, sourceSpec.SchemaVersion)
	}
	if manifest.RoadmapTotal != len(ProtocolRoadmap) || manifest.RoadmapTotal < 600 {
		t.Fatalf("manifest roadmap total %d does not match %d in source", manifest.RoadmapTotal, len(ProtocolRoadmap))
	}
	if len(manifest.Repositories) != len(sourceSpec.Repositories) || len(manifest.Repositories) < 4 {
		t.Fatalf("manifest has %d source repositories, source spec has %d", len(manifest.Repositories), len(sourceSpec.Repositories))
	}
	if len(manifest.Captures) != len(sourceSpec.Captures) {
		t.Fatalf("manifest has %d captures but source spec has %d", len(manifest.Captures), len(sourceSpec.Captures))
	}

	roadmap := make(map[string]RoadmapItem, len(ProtocolRoadmap))
	for _, item := range ProtocolRoadmap {
		if _, exists := roadmap[item.Name]; exists {
			t.Fatalf("duplicate protocol roadmap name %q", item.Name)
		}
		roadmap[item.Name] = item
	}

	repositories := make(map[string]protocolCorpusRepository, len(manifest.Repositories))
	for _, repository := range manifest.Repositories {
		if _, exists := repositories[repository.ID]; exists {
			t.Fatalf("duplicate corpus repository %q", repository.ID)
		}
		if len(repository.Commit) != 40 {
			t.Fatalf("repository %q does not use a full commit hash", repository.ID)
		}
		licenseData := readProtocolCorpusFile(t, corpusDir, repository.LicenseFile)
		assertProtocolCorpusHash(t, repository.LicenseFile, licenseData, repository.LicenseSHA256)
		repositories[repository.ID] = repository
	}

	sourceIDs := make(map[string]struct{}, len(sourceSpec.Captures))
	for _, capture := range sourceSpec.Captures {
		if _, exists := sourceIDs[capture.ID]; exists {
			t.Fatalf("duplicate source capture id %q", capture.ID)
		}
		sourceIDs[capture.ID] = struct{}{}
	}

	manifestIDs := make(map[string]struct{}, len(manifest.Captures))
	mappedProtocols := make(map[string]struct{})
	evidenceCounts := make(map[string]int)
	totalPackets := 0
	for _, capture := range manifest.Captures {
		capture := capture
		t.Run(capture.ID, func(t *testing.T) {
			if _, exists := manifestIDs[capture.ID]; exists {
				t.Fatalf("duplicate manifest capture id %q", capture.ID)
			}
			manifestIDs[capture.ID] = struct{}{}
			if _, exists := sourceIDs[capture.ID]; !exists {
				t.Fatalf("capture is absent from sources.json")
			}
			repository, exists := repositories[capture.RepositoryID]
			if !exists {
				t.Fatalf("unknown repository %q", capture.RepositoryID)
			}
			if !strings.Contains(capture.SourceURL, "/"+repository.Commit+"/") && !strings.Contains(capture.SourceURL, repository.Commit+"/") {
				t.Fatalf("source URL is not pinned to repository commit %s", repository.Commit)
			}
			switch capture.EvidenceKind {
			case "upstream-positive", "upstream-negative", "educational-challenge", "generated-positive":
			default:
				t.Fatalf("unknown evidence kind %q", capture.EvidenceKind)
			}

			if capture.RoadmapName != nil {
				item, exists := roadmap[*capture.RoadmapName]
				if !exists {
					t.Fatalf("unknown roadmap mapping %q", *capture.RoadmapName)
				}
				if capture.RoadmapFamily == nil || capture.RoadmapPriority == nil || capture.RoadmapStatus == nil {
					t.Fatalf("mapped capture has incomplete roadmap metadata")
				}
				if *capture.RoadmapFamily != item.Family || *capture.RoadmapPriority != item.Priority || *capture.RoadmapStatus != item.Status {
					t.Fatalf("stale roadmap metadata: got %s/%s/%s, want %s/%s/%s", *capture.RoadmapFamily, *capture.RoadmapPriority, *capture.RoadmapStatus, item.Family, item.Priority, item.Status)
				}
				mappedProtocols[*capture.RoadmapName] = struct{}{}
			} else if capture.RoadmapFamily != nil || capture.RoadmapPriority != nil || capture.RoadmapStatus != nil {
				t.Fatalf("outside-roadmap capture unexpectedly has roadmap metadata")
			}

			data := readProtocolCorpusFile(t, corpusDir, capture.CaptureFile)
			if int64(len(data)) != capture.SizeBytes {
				t.Fatalf("size is %d bytes, manifest says %d", len(data), capture.SizeBytes)
			}
			assertProtocolCorpusHash(t, capture.CaptureFile, data, capture.SHA256)
			packetCount, linkType, frame := inspectProtocolCorpusCapture(t, data, capture.RepresentativeFrame)
			if packetCount != capture.PacketCount || linkType != capture.LinkType {
				t.Fatalf("capture metadata is count=%d link=%q, manifest says count=%d link=%q", packetCount, linkType, capture.PacketCount, capture.LinkType)
			}
			if capture.RepresentativeFrame == nil {
				if capture.PacketCount != 0 {
					t.Fatalf("non-empty capture has no representative frame")
				}
				return
			}
			if len(capture.FrameProtocols) == 0 {
				t.Fatalf("representative frame has no recorded protocol hierarchy")
			}
			hexText := strings.TrimSpace(string(readProtocolCorpusFile(t, corpusDir, capture.RepresentativeFrame.HexFile)))
			hexFrame, err := hex.DecodeString(hexText)
			if err != nil {
				t.Fatalf("decode representative hex: %v", err)
			}
			if len(hexFrame) != capture.RepresentativeFrame.LengthBytes || !bytes.Equal(hexFrame, frame) {
				t.Fatalf("representative frame bytes do not match packet %d", capture.RepresentativeFrame.Number)
			}
		})
		evidenceCounts[capture.EvidenceKind]++
		totalPackets += capture.PacketCount
	}

	for id := range sourceIDs {
		if _, exists := manifestIDs[id]; !exists {
			t.Fatalf("source capture %q is absent from manifest", id)
		}
	}
	if len(manifest.Captures) < 384 || totalPackets < 58000 || len(mappedProtocols) < 337 {
		t.Fatalf("corpus unexpectedly shrank: captures=%d packets=%d mapped_protocols=%d", len(manifest.Captures), totalPackets, len(mappedProtocols))
	}
	if evidenceCounts["upstream-positive"] < 155 || evidenceCounts["upstream-negative"] < 16 || evidenceCounts["educational-challenge"] != 3 || evidenceCounts["generated-positive"] < 150 {
		t.Fatalf("corpus evidence classes unexpectedly shrank: %+v", evidenceCounts)
	}

	reportFile, err := os.Open(filepath.Join(corpusDir, "reports", "roadmap-material-coverage.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer reportFile.Close()
	records, err := csv.NewReader(reportFile).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != len(ProtocolRoadmap)+1 {
		t.Fatalf("coverage CSV has %d data rows, want %d", len(records)-1, len(ProtocolRoadmap))
	}
}

func readProtocolCorpusJSON(t *testing.T, fileName string, destination any) {
	t.Helper()
	data, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("decode %s: %v", fileName, err)
	}
}

func readProtocolCorpusFile(t *testing.T, root, relativeName string) []byte {
	t.Helper()
	clean := filepath.Clean(relativeName)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		t.Fatalf("unsafe corpus path %q", relativeName)
	}
	data, err := os.ReadFile(filepath.Join(root, clean))
	if err != nil {
		t.Fatalf("read %s: %v", relativeName, err)
	}
	return data
}

func assertProtocolCorpusHash(t *testing.T, name string, data []byte, want string) {
	t.Helper()
	got := fmt.Sprintf("%x", sha256.Sum256(data))
	if got != want {
		t.Fatalf("%s SHA-256 is %s, want %s", name, got, want)
	}
}

func inspectProtocolCorpusCapture(t *testing.T, data []byte, representative *protocolCorpusFrame) (int, string, []byte) {
	t.Helper()
	if len(data) < 4 {
		t.Fatalf("capture has only %d bytes", len(data))
	}
	reader := bytes.NewReader(data)
	var packetReader protocolCorpusPacketReader
	var linkType string
	if bytes.Equal(data[:4], []byte{'\x0a', '\x0d', '\x0d', '\x0a'}) {
		ngReader, err := pcapgo.NewNgReader(reader, pcapgo.NgReaderOptions{SkipUnknownVersion: true})
		if err != nil {
			t.Fatalf("open pcapng: %v", err)
		}
		packetReader = ngReader
		linkType = ngReader.LinkType().String()
	} else {
		pcapReader, err := pcapgo.NewReader(reader)
		if err != nil {
			normalized, ok := normalizeProtocolCorpusLegacyPcapHeader(data)
			if !ok {
				t.Fatalf("open pcap: %v", err)
			}
			pcapReader, err = pcapgo.NewReader(bytes.NewReader(normalized))
			if err != nil {
				t.Fatalf("open legacy pcap: %v", err)
			}
		}
		packetReader = pcapReader
		linkType = pcapReader.LinkType().String()
	}

	wantedFrame := 0
	if representative != nil {
		wantedFrame = representative.Number
	}
	count := 0
	var selected []byte
	for {
		packet, _, err := packetReader.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read packet %d: %v", count+1, err)
		}
		count++
		if count == wantedFrame {
			selected = append([]byte(nil), packet...)
		}
	}
	if wantedFrame > count {
		t.Fatalf("representative frame %d exceeds packet count %d", wantedFrame, count)
	}
	return count, linkType, selected
}

func normalizeProtocolCorpusLegacyPcapHeader(data []byte) ([]byte, bool) {
	if len(data) < 24 {
		return nil, false
	}
	bigEndian := bytes.Equal(data[:4], []byte{0xa1, 0xb2, 0xc3, 0xd4}) || bytes.Equal(data[:4], []byte{0xa1, 0xb2, 0x3c, 0x4d})
	littleEndian := bytes.Equal(data[:4], []byte{0xd4, 0xc3, 0xb2, 0xa1}) || bytes.Equal(data[:4], []byte{0x4d, 0x3c, 0xb2, 0xa1})
	normalized := append([]byte(nil), data...)
	switch {
	case bigEndian && data[4] == 0 && data[5] == 2 && data[6] == 0 && data[7] < 4:
		normalized[7] = 4
	case littleEndian && data[4] == 2 && data[5] == 0 && data[6] < 4 && data[7] == 0:
		normalized[6] = 4
	default:
		return nil, false
	}
	return normalized, true
}
