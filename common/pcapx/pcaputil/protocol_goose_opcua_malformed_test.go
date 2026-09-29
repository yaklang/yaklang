package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func gooseTimestampWidthFixture(t *testing.T, width int) []byte {
	t.Helper()
	wire := goosePDU()
	_, body, _, err := berRead(wire, 8)
	require.NoError(t, err)
	var changed []byte
	for at := 0; at < len(body); {
		tag, val, next, err := berRead(body, at)
		require.NoError(t, err)
		if tag == 0x84 {
			val = make([]byte, width)
		}
		changed = append(changed, berPut(tag, val)...)
		at = next
	}
	result := append(bytes.Clone(wire[:8]), berPut(0x61, changed)...)
	binary.BigEndian.PutUint16(result[2:4], uint16(len(result)))
	return result
}
func TestGOOSERejectInvalidTimestampWidth(t *testing.T) {
	// IEC 61850 UtcTime is eight octets: seconds, fraction, and time quality.
	// https://github.com/wireshark/wireshark/blob/4f63ea0eae68cf6facea31604994f1a339e43640/epan/dissectors/asn1/goose/goose.cnf#L30-L44
	for _, width := range []int{0, 1, 7, 8, 9} {
		wire := gooseTimestampWidthFixture(t, width)
		events := replayGOOSECapture(t, oneFrameEthernetCapture(t, 0x88b8, wire))
		require.Len(t, events, 1)
		status := "malformed"
		if width == 8 {
			status = "decoded"
		}
		require.Equal(t, status, events[0].Status, "timestamp width %d: %s", width, events[0].Error)
	}
}
func TestOPCUAAckRejectsVersionUpgrade(t *testing.T) {
	// Part 6 7.1.2.4 requires the ACK version <= the requested HEL version.
	// https://reference.opcfoundation.org/specs/OPC-10000-6/7.1.2.4
	steps := uaECCReverseConnectSteps(8192)
	binary.LittleEndian.PutUint32(steps[2].wire[8:12], 1)
	s := &binOPCUA{}
	_, err := s.consume(1, steps[1].wire, 4, 1024)
	require.NoError(t, err)
	_, err = s.consume(0, steps[2].wire, 4, 1024)
	require.ErrorContains(t, err, "ACK version exceeds HEL version")
}

func TestOPCUAAckVersionCompatibility(t *testing.T) {
	for _, versions := range [][2]uint32{{0, 0}, {1, 0}, {1, 1}} {
		steps := uaECCReverseConnectSteps(8192)
		binary.LittleEndian.PutUint32(steps[1].wire[8:12], versions[0])
		binary.LittleEndian.PutUint32(steps[2].wire[8:12], versions[1])
		s := &binOPCUA{}
		_, err := s.consume(1, steps[1].wire, 4, 1024)
		require.NoError(t, err)
		_, err = s.consume(0, steps[2].wire, 4, 1024)
		require.NoError(t, err)
	}
	// A capture beginning at ACK has no observed HEL version to compare.
	steps := uaECCReverseConnectSteps(8192)
	binary.LittleEndian.PutUint32(steps[2].wire[8:12], 1)
	_, err := (&binOPCUA{}).consume(0, steps[2].wire, 4, 1024)
	require.NoError(t, err)
}

func TestGOOSEOPCUAMalformedCaptures(t *testing.T) {
	root := "../../bin-parser/testdata/protocol-recognition-regressions/goose-opcua"
	raw, err := os.ReadFile(filepath.Join(root, "malformed-manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		EvidenceKind string `json:"evidence_kind"`
		Captures     []struct {
			File     string   `json:"file"`
			SHA256   string   `json:"sha256"`
			Statuses []string `json:"event_statuses"`
			Sources  []string `json:"sources"`
		} `json:"captures"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "synthetic-negative", manifest.EvidenceKind)
	require.Len(t, manifest.Captures, 2)
	for _, capture := range manifest.Captures {
		wire, err := os.ReadFile(filepath.Join(root, capture.File))
		require.NoError(t, err)
		sum := sha256.Sum256(wire)
		require.Equal(t, capture.SHA256, hex.EncodeToString(sum[:]))
		require.NotEmpty(t, capture.Sources)
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/workers-%d/deferred-%t", capture.File, workers, deferred), func(t *testing.T) {
					events := replayGOOSECapture(t, wire, WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred))
					require.Len(t, events, len(capture.Statuses))
					for i, event := range events {
						status := capture.Statuses[i]
						if deferred && event.Protocol == "opcua" && status == "decoded" {
							status = "deferred"
						}
						require.Equal(t, status, event.Status, event.Error)
					}
				})
			}
		}
	}
}
