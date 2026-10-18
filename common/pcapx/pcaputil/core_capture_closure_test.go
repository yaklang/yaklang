package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// These are capture observations. A deferred/incorrect checksum is not proof
// of an invalid captured payload or of a valid transmitted IPv6 UDP packet.
func TestCoreCaptureOffloadAndHalfCloseSealed(t *testing.T) {
	raw, err := trafficfixture.ReadFile("closure-core/controls.json")
	require.NoError(t, err)
	var doc struct {
		Cases []struct {
			Name, File, SHA256 string
			Packets            int
			DecodeErrors       uint64 `json:"decode_errors"`
			Invalid            uint64 `json:"invalid_segments"`
			Messages           []struct {
				Protocol        string
				Status, Summary string
				DecodeError     string   `json:"decode_error"`
				Raw             string   `json:"raw_hex"`
				Refs            []uint64 `json:"packet_refs"`
				Direction       int
				Fields          map[string]any
			}
		}
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 7)
	for _, c := range doc.Cases {
		t.Run(c.Name, func(t *testing.T) {
			wire, err := trafficfixture.ReadFile("closure-core/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(wire)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				input := bytes.Clone(wire)
				var assembly TCPReassemblyStats
				pools := map[*TrafficPool]bool{}
				opts := []CaptureOption{WithProtocolDeferred(deferred), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s }), WithOnTrafficFlowCreated(func(f *TrafficFlow) { pools[f.pool] = true })}
				if observe {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
				}
				events, stats, err := binReplay(t, input, workers, opts...)
				require.NoError(t, err)
				clear(input)
				require.EqualValues(t, c.Packets, assembly.CapturedPackets)
				require.Equal(t, c.DecodeErrors, assembly.DecodeErrors)
				require.Equal(t, c.Invalid, assembly.InvalidSegments)
				require.Zero(t, assembly.UnreassembledBytes)
				require.Zero(t, assembly.UnreassembledSegments)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
				require.Len(t, events, len(c.Messages), "complete event set: half-close/invalid length must not add or lose messages")
				for i, e := range events {
					require.Equal(t, c.Messages[i].Protocol, e.Protocol)
					require.Equal(t, c.Messages[i].Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, c.Messages[i].Direction, e.Direction)
					fields, err := e.GetFields()
					if c.Messages[i].Status != "" {
						require.Equal(t, c.Messages[i].Status, e.Status)
						require.Equal(t, c.Messages[i].Summary, e.Summary)
						require.EqualError(t, err, c.Messages[i].DecodeError)
						require.Nil(t, fields)
						require.Nil(t, e.Structured)
						require.Zero(t, stats.Decoded+stats.Deferred)
						require.EqualValues(t, 1, stats.Incomplete)
					} else {
						require.NoError(t, err)
						require.Contains(t, []string{"decoded", "deferred"}, e.Status)
						rocEqualFields(t, c.Messages[i].Fields, fields)
					}
					var refs []uint64
					for _, r := range e.SourceBytes.PacketRefs {
						require.Equal(t, e.Domain, r.Domain)
						refs = append(refs, r.Number)
					}
					require.Equal(t, c.Messages[i].Refs, refs)
					require.Empty(t, e.Error)
					if i > 0 {
						require.Greater(t, e.ID, events[i-1].ID)
						require.Equal(t, events[0].FlowID, e.FlowID)
					}
				}
				generationPoolsReleased(t, pools)
			})
		})
	}
}
