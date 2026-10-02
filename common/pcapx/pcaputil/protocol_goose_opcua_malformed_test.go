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
	"strings"
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

// Synthetic negative state transitions; native positive coverage is separate.
func TestOPCUAReverseContextObserved(t *testing.T) {
	for _, kind := range []string{"same-direction-HEL", "wrong-endpoint-HEL", "wrong-direction-ACK"} {
		t.Run(kind, func(t *testing.T) {
			steps := uaECCReverseConnectSteps(8192)
			s := &binOPCUA{}
			_, err := s.consume(0, steps[0].wire, 64, 1024*1024)
			require.NoError(t, err)
			dir, wire := 1, steps[1].wire
			if kind == "same-direction-HEL" {
				dir = 0
			}
			if kind == "wrong-endpoint-HEL" {
				wire[len(wire)-1] = '1'
			}
			if kind == "wrong-direction-ACK" {
				wire = steps[2].wire
			}
			_, err = s.consume(dir, wire, 64, 1024*1024)
			require.Error(t, err)
		})
	}
}

func uaRHEString(value []byte) []byte {
	wire := make([]byte, 4)
	if value == nil {
		binary.LittleEndian.PutUint32(wire, ^uint32(0))
	} else {
		binary.LittleEndian.PutUint32(wire, uint32(len(value)))
	}
	return append(wire, value...)
}

func TestOPCUAObservedHandshakeCannotRestart(t *testing.T) {
	// Part 6 7.1.3: RHE starts a server-initiated transport; HEL and ACK
	// are each sent once. These are observed-state negatives, not a demand
	// that a partial capture contain the beginning of a connection.
	// https://reference.opcfoundation.org/specs/OPC-10000-6/7.1.3
	steps := uaECCReverseConnectSteps(8192)
	rhe, hello, ack := steps[0], steps[1], steps[2]
	changedRHE := sessionStep{rhe.dir, bytes.Clone(rhe.wire)}
	changedRHE.wire[len(changedRHE.wire)-1] ^= 1
	opn := append(make([]byte, 4), uaRHEString([]byte("http://opcfoundation.org/UA/SecurityPolicy#Basic256Sha256"))...)
	opn = append(opn, uaRHEString(nil)...)
	opn = append(opn, uaRHEString(nil)...)
	for _, tc := range []struct {
		name   string
		prefix []sessionStep
		bad    sessionStep
	}{
		{"RHE-twice", []sessionStep{rhe}, rhe},
		{"RHE-changed-endpoint", []sessionStep{rhe}, changedRHE},
		{"RHE-changed-direction", []sessionStep{rhe}, sessionStep{1, rhe.wire}},
		{"HEL-twice", []sessionStep{hello}, hello},
		{"HEL-other-direction", []sessionStep{hello}, sessionStep{0, hello.wire}},
		{"ACK-twice", []sessionStep{ack}, ack},
		{"ACK-other-direction", []sessionStep{ack}, sessionStep{1, ack.wire}},
		{"ACK-same-direction-as-HEL", []sessionStep{hello}, sessionStep{1, ack.wire}},
		{"RHE-after-HEL", []sessionStep{hello}, rhe},
		{"RHE-after-ACK", []sessionStep{ack}, rhe},
		{"RHE-after-OPN", []sessionStep{{1, uaTestEnvelope("OPN", opn)}}, rhe},
		{"RHE-after-MSG", []sessionStep{{1, uaTestEnvelope("MSG", make([]byte, 8))}}, rhe},
		{"RHE-after-CLO", []sessionStep{{1, uaTestEnvelope("CLO", make([]byte, 8))}}, rhe},
		{"RHE-after-ERR", []sessionStep{{1, uaTestEnvelope("ERR", make([]byte, 8))}}, rhe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &binOPCUA{}
			for _, step := range tc.prefix {
				_, err := s.consume(step.dir, step.wire, 64, 1<<20)
				require.NoError(t, err)
			}
			peers, reverseDir, reverseEndpoint := s.peers, s.reverseDir, s.reverseEndpoint
			_, err := s.consume(tc.bad.dir, tc.bad.wire, 64, 1<<20)
			require.Error(t, err)
			require.Equal(t, peers, s.peers, "rejected handshake must not overwrite negotiated peers")
			require.Equal(t, reverseDir, s.reverseDir)
			require.Equal(t, reverseEndpoint, s.reverseEndpoint)
		})
	}
	for _, step := range []sessionStep{rhe, hello, ack, {1, uaTestEnvelope("OPN", opn)}} {
		_, err := (&binOPCUA{}).consume(step.dir, step.wire, 64, 1<<20)
		require.NoError(t, err, "first observed %s remains supported", step.wire[:3])
	}
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, duplicate := range []sessionStep{rhe, hello, ack} {
				raw := sessionTestPCAP(t, append(append([]sessionStep{}, steps...), duplicate), 4840, 7, false, false)
				events, stats, err := binReplay(t, raw, workers, WithBinParserDeferred(deferred))
				require.NoError(t, err)
				require.Len(t, events, 4)
				for _, event := range events[:3] {
					require.Empty(t, event.Error)
				}
				require.Equal(t, "malformed", events[3].Status)
				require.NotEmpty(t, events[3].Error)
				require.Zero(t, stats.BufferedBytes)
			}
		}
	}
}

func TestOPCUAReverseHelloStringBounds(t *testing.T) {
	// Part 6 7.1.2.6 limits each encoded string value to less than 4096
	// bytes. The four-byte length prefix is not string value content.
	// https://reference.opcfoundation.org/specs/OPC-10000-6/7.1.2.6
	for _, field := range []string{"Server URI", "Endpoint URL"} {
		for _, size := range []int{-1, 0, 4095, 4096, 5000} {
			t.Run(fmt.Sprintf("%s/%d", field, size), func(t *testing.T) {
				value := []byte(nil)
				if size >= 0 {
					value = []byte(strings.Repeat("x", size))
				}
				uri, url := []byte("urn:example:server"), []byte("opc.tcp://server.example:4840")
				if field == "Server URI" {
					uri = value
				} else {
					url = value
				}
				wire := uaTestEnvelope("RHE", append(uaRHEString(uri), uaRHEString(url)...))
				s := &binOPCUA{}
				out, err := s.consume(0, wire, 64, 1<<20)
				if size < 4096 {
					require.NoError(t, err)
					require.Equal(t, string(value), out[field])
				} else {
					require.ErrorContains(t, err, "ReverseHello string length")
					require.False(t, s.reverseSeen)
					require.Empty(t, s.reverseEndpoint)
				}
			})
		}
	}
}
