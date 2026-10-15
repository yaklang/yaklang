package pcapdb

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type comprehensiveManifest struct {
	CompressedBytes     int    `json:"compressed_bytes"`
	ExpandedBytes       int    `json:"expanded_bytes"`
	CompressedSHA256    string `json:"compressed_sha256"`
	ExpandedSHA256      string `json:"expanded_sha256"`
	PacketPayloadSHA256 string `json:"packet_payload_sha256"`
	Packets             int64  `json:"packets"`
	Sections            int64  `json:"sections"`
	Interfaces          int    `json:"interfaces"`
	Sources             []struct {
		Source              string `json:"source"`
		Packets             int64  `json:"packets"`
		Sections            int64  `json:"sections"`
		Interfaces          int    `json:"interfaces"`
		FirstPacketID       int64  `json:"first_packet_id"`
		PacketPayloadSHA256 string `json:"packet_payload_sha256"`
	} `json:"sources"`
}

func comprehensiveCapture(t testing.TB) (string, []byte, comprehensiveManifest) {
	t.Helper()
	raw, err := os.ReadFile("testdata/comprehensive.json")
	require.NoError(t, err)
	var manifest comprehensiveManifest
	require.NoError(t, json.Unmarshal(raw, &manifest))
	compressed, err := os.ReadFile("testdata/comprehensive.pcapng.gz")
	require.NoError(t, err)
	require.Len(t, compressed, manifest.CompressedBytes)
	require.GreaterOrEqual(t, len(compressed), 190<<10)
	require.LessOrEqual(t, len(compressed), 215<<10)
	require.Equal(t, manifest.CompressedSHA256, digestBytes(compressed))
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	capture, err := io.ReadAll(io.LimitReader(reader, int64(manifest.ExpandedBytes)+1))
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Len(t, capture, manifest.ExpandedBytes)
	require.Equal(t, manifest.ExpandedSHA256, digestBytes(capture))
	path := filepath.Join(t.TempDir(), "comprehensive.pcapng")
	require.NoError(t, os.WriteFile(path, capture, 0600))
	return path, capture, manifest
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// This tests storage/replay parity and immutable packet bytes. The provenance
// manifest describes source coverage, not a golden for every parsed field.
func TestPCAPDBComprehensiveFixture(t *testing.T) {
	input, capture, manifest := comprehensiveCapture(t)
	m := testManager(t)
	db, err := m.GetOrCreate(input, WithProtocols(true), WithBatchSize(128))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, StateReady, meta.State)
	require.Equal(t, manifest.Packets, meta.PacketCount)
	require.True(t, meta.ProtocolsIndexed)
	var interfaces []PCAPCaptureInterface
	require.NoError(t, db.db.Order("section,interface_id").Find(&interfaces).Error)
	require.Len(t, interfaces, manifest.Interfaces)
	for _, iface := range interfaces {
		require.NotZero(t, iface.CreatedAt)
		require.NotZero(t, iface.UpdatedAt)
	}

	packetsHash := sha256.New()
	sourceHashes := make([][]byte, len(manifest.Sources))
	var after int64
	sourceIndex, sourceSection := 0, int64(0)
	sourceHash := sha256.New()
	for {
		packets, err := db.QueryPackets(QueryAfter(after), QueryLimit(1000))
		require.NoError(t, err)
		if len(packets) == 0 {
			break
		}
		for _, packet := range packets {
			source := manifest.Sources[sourceIndex]
			require.EqualValues(t, after+1, packet.ID)
			require.Equal(t, sourceSection, packet.Section, source.Source)
			require.NotZero(t, packet.CreatedAt)
			raw, err := db.ReadPacket(packet.ID)
			require.NoError(t, err)
			require.Len(t, raw, packet.CapturedLength)
			var lengths [8]byte
			binary.LittleEndian.PutUint32(lengths[:4], uint32(packet.CapturedLength))
			binary.LittleEndian.PutUint32(lengths[4:], uint32(packet.OriginalLength))
			packetsHash.Write(lengths[:])
			packetsHash.Write(raw)
			sourceHash.Write(lengths[:])
			sourceHash.Write(raw)
			after = int64(packet.ID)
			if after == source.FirstPacketID+source.Packets-1 {
				sourceHashes[sourceIndex] = sourceHash.Sum(nil)
				sourceHash.Reset()
				sourceIndex++
				sourceSection += source.Sections
			}
		}
	}
	require.Equal(t, manifest.Packets, after)
	require.Equal(t, manifest.PacketPayloadSHA256, hex.EncodeToString(packetsHash.Sum(nil)))
	for i, source := range manifest.Sources {
		require.Equal(t, source.PacketPayloadSHA256, hex.EncodeToString(sourceHashes[i]), source.Source)
	}

	// Compare persisted summaries and packet associations with the existing
	// replay API, independently of GORM insertion and query projection code.
	expected := make(map[string]int)
	var eventIDs []int64
	var parserRules, parserEntries []string
	var fieldTrees, sessionTrees [][]byte
	var projectionError error
	var references [][]int64
	var protocolBytes [][]byte
	require.NoError(t, pcaputil.ReplayPcapFile(input, pcaputil.WithOnProtocolMessage(func(event *pcaputil.ProtocolEvent) {
		expected[event.Protocol]++
		eventIDs = append(eventIDs, int64(event.ID))
		parserRules = append(parserRules, event.Rule)
		parserEntries = append(parserEntries, event.Entry)
		fields := event.Fields
		if fields == nil {
			fields, _ = event.GetFields() // Decode failures are persisted as null fields.
		}
		fieldsJSON, err := json.Marshal(fields)
		if err != nil {
			projectionError = err
		}
		sessionJSON, err := json.Marshal(event.Session)
		if err != nil {
			projectionError = err
		}
		fieldTrees = append(fieldTrees, fieldsJSON)
		sessionTrees = append(sessionTrees, sessionJSON)
		ids := make([]int64, 0)
		seen := make(map[uint64]bool)
		for _, packet := range event.SourceBytes.PacketRefs {
			if !seen[packet.Number] {
				ids = append(ids, int64(packet.Number))
				seen[packet.Number] = true
			}
		}
		references = append(references, ids)
		protocolBytes = append(protocolBytes, append([]byte(nil), event.Raw...))
	})))
	require.NoError(t, projectionError)
	require.EqualValues(t, len(eventIDs), meta.ProtocolCount)
	actual := make(map[string]int)
	after = 0
	var messageIndex int
	for {
		messages, err := db.QueryProtocols(QueryAfter(after), QueryLimit(100), QueryWithFields(true))
		require.NoError(t, err)
		if len(messages) == 0 {
			break
		}
		for _, message := range messages {
			actual[message.Protocol]++
			require.Equal(t, eventIDs[messageIndex], message.EventID)
			require.Equal(t, parserRules[messageIndex], message.Rule)
			require.Equal(t, parserEntries[messageIndex], message.Entry)
			var fields, session map[string]any
			require.NoError(t, decodeProtocolFields(fieldTrees[messageIndex], &fields))
			require.Equal(t, fields, message.Fields, "complete JSON field tree for message %d", message.ID)
			require.NoError(t, decodeProtocolFields(sessionTrees[messageIndex], &session))
			require.Equal(t, session, message.Session, "session snapshot for message %d", message.ID)
			ids, err := db.ProtocolPacketIDs(message.ID)
			require.NoError(t, err)
			require.ElementsMatch(t, references[messageIndex], ids)
			raw, err := db.ReadProtocol(message.ID)
			require.NoError(t, err)
			require.True(t, bytes.Equal(protocolBytes[messageIndex], raw), "protocol bytes for message %d", message.ID)
			after = int64(message.ID)
			messageIndex++
		}
	}
	require.Equal(t, expected, actual)
	for _, protocol := range []string{"dns", "http", "tls", "ssh", "mqtt", "modbus", "s7comm", "opcua"} {
		require.Positive(t, actual[protocol], protocol)
	}
	messages, err := db.QueryProtocols(QueryProtocol("dns"), QueryWithFields(true), QueryLimit(1))
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.NotEmpty(t, messages[0].Fields)
	valid, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
	output := filepath.Join(t.TempDir(), "export.pcapng")
	_, err = db.Export(output)
	require.NoError(t, err)
	exported, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, capture, exported)
	again, err := m.GetOrCreate(input, WithProtocols(true))
	require.NoError(t, err)
	require.Equal(t, db.ID, again.ID)
	count, err := m.Count()
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	t.Logf("capture: %d packets, %d messages, %d protocols, %d bytes compressed", meta.PacketCount, meta.ProtocolCount, len(actual), manifest.CompressedBytes)
}

func BenchmarkPCAPDBComprehensiveImport(b *testing.B) {
	input, capture, _ := comprehensiveCapture(b)
	b.SetBytes(int64(len(capture)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		m := testManager(b)
		b.StartTimer()
		db, err := m.GetOrCreate(input, WithProtocols(true))
		require.NoError(b, err)
		require.NoError(b, db.Close())
	}
}

func TestPCAPDBIncompleteReplayFailureAndPacketOnlyRetry(t *testing.T) {
	input, err := trafficfixture.Materialize("../../bin-parser/testdata/protocol-corpus/captures/ndpi/ndpi-tls.pcap", t.TempDir())
	require.NoError(t, err)
	m := testManager(t)
	_, err = m.GetOrCreate(input, WithProtocols(true), WithBatchSize(1))
	require.ErrorContains(t, err, "unfilled sequence gap")
	entries, err := m.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	meta := entries[0]
	require.Equal(t, StateFailed, meta.State)
	require.EqualValues(t, 120, meta.PacketCount)
	require.False(t, meta.ProtocolsIndexed)
	var messages int64
	reader, err := openIndex(meta.DatabasePath, false, false)
	require.NoError(t, err)
	require.NoError(t, reader.Model(&PCAPProtocolMessage{}).Count(&messages).Error)
	require.NoError(t, reader.Close())
	require.Equal(t, meta.ProtocolCount, messages, "failed replay must report committed protocol rows only")
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	require.Equal(t, meta.DatasetID, db.ID)
	recovered, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, StateReady, recovered.State)
	require.EqualValues(t, 120, recovered.PacketCount)
	require.Zero(t, recovered.ProtocolCount)
	require.False(t, recovered.ProtocolsIndexed)
	valid, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
}
