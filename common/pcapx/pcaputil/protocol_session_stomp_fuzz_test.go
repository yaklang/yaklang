package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Seeds retain the native TCP payload bytes and directions, with a small
// harness-only length envelope. Mutations are never presented as native PCAP.
func FuzzSTOMPStateSequence(f *testing.F) {
	for _, version := range []string{"1.0", "1.1", "1.2"} {
		raw, err := os.ReadFile(filepath.Join(stompNativeRoot, "rabbitmq-4.1.0-stomp-"+version+"-tcp-oracle.tsv"))
		require.NoError(f, err)
		reader := csv.NewReader(bytes.NewReader(raw))
		reader.Comma = '\t'
		rows, err := reader.ReadAll()
		require.NoError(f, err)
		var sequence []byte
		for _, row := range rows[1:] {
			if row[12] == "" {
				continue
			}
			wire, err := hex.DecodeString(strings.ReplaceAll(row[12], ":", ""))
			require.NoError(f, err)
			dir := byte(0)
			if row[4] == "46164" {
				dir = 1
			}
			sequence = append(sequence, dir, byte(len(wire)>>8), byte(len(wire)))
			sequence = append(sequence, wire...)
		}
		for chunk := byte(0); chunk < 4; chunk++ {
			f.Add(append([]byte{chunk}, sequence...))
		}
		// Harness-only close actions preserve the original native payloads:
		// FIN after one feed, or idle timeout after five 64-byte feeds.
		for _, control := range []byte{0x80, 0xd3} {
			f.Add(append([]byte{control}, sequence...))
		}
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) == 0 || len(input) > 16<<10 {
			return
		}
		budget := DefaultParserBudget()
		budget.MaxMessageBytes = 8192
		budget.MaxFrameBytes = 8192
		budget.MaxBufferedBytes = 128 << 10
		budget.MaxCollectionElements = 64
		session, err := NewProtocolSession(budget)
		require.NoError(t, err)
		s := session.(*captureSession)
		type snapshot struct {
			event *ProtocolEvent
			raw   []byte
		}
		var snapshots []snapshot
		marshal := func(e *ProtocolEvent) []byte {
			raw, err := json.Marshal(map[string]any{"session": e.Session, "fields": e.Fields, "raw": e.Raw})
			require.NoError(t, err)
			return raw
		}
		closeAt, feeds := 0, 0
		if input[0]&0x80 != 0 {
			closeAt = 1 + int(input[0]>>2&15)
		}
		closeReason := string(TrafficFlowCloseReason_FIN)
		if input[0]&0x40 != 0 {
			closeReason = string(TrafficFlowCloseReason_INACTIVE)
		}
		closedByInput := false
		closeFromInput := func() {
			for _, event := range s.Close(closeReason) {
				snapshots = append(snapshots, snapshot{event, marshal(event)})
			}
			closedByInput = true
			require.Zero(t, s.Stats().BufferedBytes)
		}
		feed := func(dir int, wire []byte) {
			result := s.Feed(dir, time.Unix(1, 0), wire)
			if closedByInput {
				require.Equal(t, "closed", result.State)
				require.Empty(t, result.Events)
				require.Zero(t, result.Consumed)
				require.Zero(t, s.Stats().BufferedBytes)
			}
			for _, event := range result.Events {
				snapshots = append(snapshots, snapshot{event, marshal(event)})
			}
			require.LessOrEqual(t, s.f.a.peak.Load(), int64(budget.MaxBufferedBytes))
			feeds++
			if closeAt > 0 && feeds == closeAt {
				closeFromInput()
			}
		}
		chunk := []int{0, 1, 7, 64}[input[0]&3]
		for rest := input[1:]; len(rest) >= 3; {
			dir := int(rest[0] & 1)
			size := min(int(binary.BigEndian.Uint16(rest[1:3])), len(rest)-3)
			wire := rest[3 : 3+size]
			rest = rest[3+size:]
			for len(wire) > 0 {
				n := len(wire)
				if chunk > 0 {
					n = min(n, chunk)
				}
				feed(dir, wire[:n])
				wire = wire[n:]
			}
		}
		if closeAt > 0 && !closedByInput {
			closeFromInput()
		}
		// STOMP errors terminate a connection. A subsequent valid frame must be
		// safe even when it cannot be decoded after a fatal earlier frame.
		feed(0, []byte("SEND\n\nbad\x00"))
		feed(0, []byte("SEND\ndestination:/queue/after\n\ngood\x00"))
		s.Close("FIN")
		require.Empty(t, s.Close("FIN"))
		require.Zero(t, s.f.a.buffered.Load())
		for _, snapshot := range snapshots {
			require.Equal(t, snapshot.raw, marshal(snapshot.event), "event snapshot changed after later feed/close")
		}
		clean, err := NewProtocolSession(budget)
		require.NoError(t, err)
		for _, step := range []sessionStep{{0, []byte(stomp12Connect)}, {1, []byte("CONNECTED\nversion:1.2\n\n\x00")}, {0, []byte("SEND\ndestination:/queue/after\nreceipt:r\n\ngood\x00")}, {1, []byte("RECEIPT\nreceipt-id:r\n\n\x00")}} {
			result := clean.Feed(step.dir, time.Unix(1, 0), step.wire)
			require.Nil(t, result.Err)
			require.NotEmpty(t, result.Events)
		}
		clean.Close("FIN")
		require.Empty(t, clean.Close("FIN"))
		require.Zero(t, clean.(*captureSession).f.a.buffered.Load())
	})
}
