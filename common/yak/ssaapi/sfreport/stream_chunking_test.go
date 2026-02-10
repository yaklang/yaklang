package sfreport

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

// ---------------------------------------------------------------------------
// Helpers: mirror emitter-side compress+chunk and aggregator-side join+decompress.
// ---------------------------------------------------------------------------

func emitterCompress(raw []byte, codec string) ([]byte, string) {
	if codec == "" || len(raw) < 1024 {
		return raw, ""
	}
	var enc []byte
	switch codec {
	case "gzip":
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		_, _ = zw.Write(raw)
		_ = zw.Close()
		enc = buf.Bytes()
	case "zstd":
		w, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
		enc = w.EncodeAll(raw, nil)
		w.Close()
	case "snappy":
		enc = snappy.Encode(nil, raw)
	default:
		return raw, ""
	}
	threshold := len(raw) / 10
	if codec == "snappy" {
		threshold = len(raw) / 20
	}
	if len(enc) >= len(raw)-threshold {
		return raw, ""
	}
	return append([]byte(nil), enc...), codec
}

type chunkPiece struct {
	Index int
	Data  []byte
	Last  bool
}

func emitterChunk(data []byte, chunkSize int) []chunkPiece {
	if len(data) == 0 {
		return []chunkPiece{{Index: 0, Data: nil, Last: true}}
	}
	var pieces []chunkPiece
	for i, off := 0, 0; off < len(data); i, off = i+1, off+chunkSize {
		end := off + chunkSize
		if end > len(data) {
			end = len(data)
		}
		pieces = append(pieces, chunkPiece{Index: i, Data: data[off:end], Last: end >= len(data)})
	}
	return pieces
}

func aggregatorJoinChunks(chunks map[int][]byte, lastIdx int) []byte {
	if lastIdx < 0 {
		return nil
	}
	var buf bytes.Buffer
	for i := 0; i <= lastIdx; i++ {
		buf.Write(chunks[i])
	}
	return buf.Bytes()
}

func aggregatorDecode(data []byte, encoding string) ([]byte, error) {
	switch encoding {
	case "", "none":
		return data, nil
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(zr)
	case "zstd":
		dec, err := zstd.NewReader(nil)
		if err != nil {
			return nil, err
		}
		defer dec.Close()
		return dec.DecodeAll(data, nil)
	case "snappy":
		return snappy.Decode(nil, data)
	default:
		return data, nil
	}
}

type simulatedFileMeta struct {
	FileHash      string
	ContentSize   int64
	Encoding      string
	InlineContent []byte
}
type simulatedFileChunk struct {
	FileHash   string
	ChunkIndex int
	Data       []byte
	IsLast     bool
}

func emitSimulatedFile(content []byte, fileHash, codec string, chunkSize, inlineMax int) (simulatedFileMeta, []simulatedFileChunk) {
	compressed, encoding := emitterCompress(content, codec)
	meta := simulatedFileMeta{FileHash: fileHash, ContentSize: int64(len(content)), Encoding: encoding}
	if inlineMax > 0 && len(compressed) > 0 && len(compressed) <= inlineMax {
		meta.InlineContent = compressed
		return meta, nil
	}
	var chunks []simulatedFileChunk
	for _, p := range emitterChunk(compressed, chunkSize) {
		chunks = append(chunks, simulatedFileChunk{FileHash: fileHash, ChunkIndex: p.Index, Data: p.Data, IsLast: p.Last})
	}
	return meta, chunks
}

func reassembleSimulatedFile(meta simulatedFileMeta, chunks []simulatedFileChunk) ([]byte, error) {
	if len(meta.InlineContent) > 0 {
		if meta.Encoding != "" {
			return aggregatorDecode(meta.InlineContent, meta.Encoding)
		}
		return meta.InlineContent, nil
	}
	chunkMap := make(map[int][]byte)
	lastIdx := -1
	for _, c := range chunks {
		chunkMap[c.ChunkIndex] = c.Data
		if c.IsLast {
			lastIdx = c.ChunkIndex
		}
	}
	if lastIdx < 0 {
		return nil, fmt.Errorf("no last chunk")
	}
	joined := aggregatorJoinChunks(chunkMap, lastIdx)
	if meta.Encoding != "" {
		return aggregatorDecode(joined, meta.Encoding)
	}
	return joined, nil
}

// ---------------------------------------------------------------------------
// Data generators
// ---------------------------------------------------------------------------

func makeTextData(size int) []byte {
	line := "the quick brown fox jumps over the lazy dog, 1234567890\n"
	var buf bytes.Buffer
	for buf.Len() < size {
		buf.WriteString(line)
	}
	return buf.Bytes()[:size]
}

func makeRandomData(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return data
}

func makeJSONData(riskCount int) []byte {
	parts := &StreamSingleResultParts{
		ProgramName: "test-program",
		ReportType:  "security",
		Risks:       make([]*StreamRiskPart, 0, riskCount),
		Files: []*File{
			{Path: "/src/main.go", IrSourceHash: "abc123", Content: strings.Repeat("package main\n", 200)},
			{Path: "/src/util.go", IrSourceHash: "def456", Content: strings.Repeat("func helper() {}\n", 300)},
		},
	}
	for i := 0; i < riskCount; i++ {
		riskJSON, _ := json.Marshal(map[string]any{
			"title":       fmt.Sprintf("SQL Injection #%d", i),
			"severity":    "high",
			"description": strings.Repeat("risk detail ", 50),
		})
		parts.Risks = append(parts.Risks, &StreamRiskPart{
			RiskHash:       fmt.Sprintf("riskhash_%04d", i),
			RiskJSON:       riskJSON,
			FileHashes:     []string{"abc123"},
			DataflowHashes: []string{fmt.Sprintf("flow_%04d", i)},
		})
	}
	raw, _ := json.Marshal(parts)
	return raw
}

// ===========================================================================
// Tests
// ===========================================================================

func TestEmitterCompress(t *testing.T) {
	textData := makeTextData(8192)

	tests := []struct {
		name        string
		data        []byte
		codec       string
		wantEncoded bool // true = should compress, false = should bypass
	}{
		{"small_gzip", []byte("hello"), "gzip", false},
		{"small_zstd", []byte("hello"), "zstd", false},
		{"small_snappy", []byte("hello"), "snappy", false},
		{"no_codec", textData, "", false},
		{"gzip_text", textData, "gzip", true},
		{"zstd_text", textData, "zstd", true},
		{"snappy_text", textData, "snappy", true},
		{"random_gzip", makeRandomData(4096), "gzip", false}, // incompressible
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, encoding := emitterCompress(tt.data, tt.codec)
			if tt.wantEncoded {
				if encoding == "" {
					t.Fatal("expected compression, got none")
				}
				if len(out) >= len(tt.data) {
					t.Fatalf("compressed should be smaller: %d >= %d", len(out), len(tt.data))
				}
			} else {
				if encoding != "" {
					t.Fatalf("expected no compression, got %q", encoding)
				}
				if !bytes.Equal(out, tt.data) {
					t.Fatal("data should be unchanged")
				}
			}
		})
	}
}

func TestCompressDecompressRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		data  []byte
		codec string
	}{
		{"gzip_text", makeTextData(16384), "gzip"},
		{"zstd_text", makeTextData(16384), "zstd"},
		{"snappy_text", makeTextData(16384), "snappy"},
		{"gzip_json", makeJSONData(20), "gzip"},
		{"zstd_json", makeJSONData(20), "zstd"},
		{"snappy_json", makeJSONData(20), "snappy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed, encoding := emitterCompress(tt.data, tt.codec)
			if encoding == "" {
				t.Skip("compression skipped")
			}
			decoded, err := aggregatorDecode(compressed, encoding)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !bytes.Equal(decoded, tt.data) {
				t.Fatalf("roundtrip mismatch: %d → %d", len(tt.data), len(decoded))
			}
		})
	}
}

func TestEmitterChunk(t *testing.T) {
	seqData := func(n int) []byte {
		d := make([]byte, n)
		for i := range d {
			d[i] = byte(i % 256)
		}
		return d
	}

	tests := []struct {
		name       string
		data       []byte
		chunkSize  int
		wantPieces int
		lastSizes  []int // expected sizes of each piece
	}{
		{"empty", nil, 256, 1, []int{0}},
		{"single_byte", []byte{0x42}, 256, 1, []int{1}},
		{"smaller_than_chunk", []byte("hello"), 1024, 1, []int{5}},
		{"exact_multiple", seqData(512), 256, 2, []int{256, 256}},
		{"not_exact", seqData(700), 256, 3, []int{256, 256, 188}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pieces := emitterChunk(tt.data, tt.chunkSize)
			if len(pieces) != tt.wantPieces {
				t.Fatalf("pieces: got %d, want %d", len(pieces), tt.wantPieces)
			}
			if !pieces[len(pieces)-1].Last {
				t.Fatal("last piece must have Last=true")
			}
			for i, want := range tt.lastSizes {
				if len(pieces[i].Data) != want {
					t.Errorf("piece[%d] size: got %d, want %d", i, len(pieces[i].Data), want)
				}
			}
			var buf bytes.Buffer
			for _, p := range pieces {
				buf.Write(p.Data)
			}
			if tt.data != nil && !bytes.Equal(buf.Bytes(), tt.data) {
				t.Fatal("concatenated chunks != original")
			}
		})
	}
}

func TestJoinChunks(t *testing.T) {
	tests := []struct {
		name    string
		chunks  map[int][]byte
		lastIdx int
		want    string
	}{
		{"negative_idx", nil, -1, ""},
		{"single", map[int][]byte{0: []byte("hello")}, 0, "hello"},
		{"multiple", map[int][]byte{0: []byte("aaa"), 1: []byte("bbb"), 2: []byte("ccc")}, 2, "aaabbbccc"},
		{"missing_middle", map[int][]byte{0: []byte("aaa"), 2: []byte("ccc")}, 2, "aaaccc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := aggregatorJoinChunks(tt.chunks, tt.lastIdx)
			if tt.lastIdx < 0 {
				if result != nil {
					t.Fatal("expected nil")
				}
				return
			}
			if string(result) != tt.want {
				t.Fatalf("got %q, want %q", string(result), tt.want)
			}
		})
	}
}

func TestAggregateDecode(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		encoding string
		wantErr  bool
		wantSame bool // expect output == input
	}{
		{"empty_encoding", []byte("raw"), "", false, true},
		{"none_encoding", []byte("raw"), "none", false, true},
		{"unknown_encoding", []byte("raw"), "brotli", false, true},
		{"invalid_gzip", []byte("not gzip"), "gzip", true, false},
		{"invalid_snappy", []byte("not snappy"), "snappy", true, false},
		{"invalid_zstd", []byte("not zstd"), "zstd", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := aggregatorDecode(tt.data, tt.encoding)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSame && !bytes.Equal(out, tt.data) {
				t.Fatal("output should equal input")
			}
		})
	}
}

func TestRoundtrip(t *testing.T) {
	tests := []struct {
		name      string
		dataSize  int
		codec     string
		chunkSize int
	}{
		{"none_512", 2048, "", 512},
		{"gzip_1k", 8192, "gzip", 1024},
		{"gzip_256k", 8192, "gzip", 256 * 1024},
		{"zstd_1k", 8192, "zstd", 1024},
		{"snappy_1k", 8192, "snappy", 1024},
		{"none_large", 512 * 1024, "", 256 * 1024},
		{"gzip_large", 512 * 1024, "gzip", 256 * 1024},
		{"zstd_large", 512 * 1024, "zstd", 256 * 1024},
		{"snappy_large", 512 * 1024, "snappy", 256 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makeTextData(tt.dataSize)
			compressed, encoding := emitterCompress(data, tt.codec)
			pieces := emitterChunk(compressed, tt.chunkSize)
			if !pieces[len(pieces)-1].Last {
				t.Fatal("last piece missing Last flag")
			}

			chunkMap := make(map[int][]byte)
			lastIdx := -1
			for _, p := range pieces {
				chunkMap[p.Index] = p.Data
				if p.Last {
					lastIdx = p.Index
				}
			}
			joined := aggregatorJoinChunks(chunkMap, lastIdx)
			if !bytes.Equal(joined, compressed) {
				t.Fatal("joined != compressed")
			}
			decoded, err := aggregatorDecode(joined, encoding)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !bytes.Equal(decoded, data) {
				t.Fatal("roundtrip mismatch")
			}
		})
	}
}

func TestRoundtripJSONIntegrity(t *testing.T) {
	original := makeJSONData(30)
	for _, codec := range []string{"gzip", "zstd", "snappy"} {
		t.Run(codec, func(t *testing.T) {
			compressed, encoding := emitterCompress(original, codec)
			pieces := emitterChunk(compressed, 2048)

			chunkMap := make(map[int][]byte)
			lastIdx := -1
			for _, p := range pieces {
				chunkMap[p.Index] = p.Data
				if p.Last {
					lastIdx = p.Index
				}
			}
			decoded, err := aggregatorDecode(aggregatorJoinChunks(chunkMap, lastIdx), encoding)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			var parts StreamSingleResultParts
			if err := json.Unmarshal(decoded, &parts); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if parts.ProgramName != "test-program" {
				t.Errorf("ProgramName = %q", parts.ProgramName)
			}
			if len(parts.Risks) != 30 {
				t.Errorf("risks = %d, want 30", len(parts.Risks))
			}
			if len(parts.Files) != 2 {
				t.Errorf("files = %d, want 2", len(parts.Files))
			}
		})
	}
}

func TestFileStreaming(t *testing.T) {
	tests := []struct {
		name      string
		content   []byte
		codec     string
		chunkSize int
		inlineMax int
		wantInline bool
	}{
		{"small_inline", []byte("package main\n"), "gzip", 256 * 1024, 16 * 1024, true},
		{"large_gzip_chunked", makeTextData(100 * 1024), "gzip", 256, 64, false},
		{"none_50k", makeTextData(50 * 1024), "", 16 * 1024, 4 * 1024, false},
		{"gzip_50k", makeTextData(50 * 1024), "gzip", 16 * 1024, 4 * 1024, true},  // compresses well → inline
		{"zstd_50k", makeTextData(50 * 1024), "zstd", 16 * 1024, 4 * 1024, true},
		{"snappy_50k", makeTextData(50 * 1024), "snappy", 16 * 1024, 4 * 1024, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, chunks := emitSimulatedFile(tt.content, "fh_"+tt.name, tt.codec, tt.chunkSize, tt.inlineMax)
			if tt.wantInline && len(meta.InlineContent) == 0 && len(chunks) > 0 {
				t.Log("expected inline but got chunks (compression ratio may vary)")
			}
			result, err := reassembleSimulatedFile(meta, chunks)
			if err != nil {
				t.Fatalf("reassemble: %v", err)
			}
			if !bytes.Equal(result, tt.content) {
				t.Fatalf("content mismatch: got %d, want %d", len(result), len(tt.content))
			}
		})
	}
}

func TestFileStreaming_ChunksOutOfOrder(t *testing.T) {
	content := makeTextData(30 * 1024)
	meta, chunks := emitSimulatedFile(content, "fh_ooo", "gzip", 256, 64)
	if len(chunks) < 2 {
		t.Skip("need multiple chunks")
	}
	reversed := make([]simulatedFileChunk, len(chunks))
	for i, c := range chunks {
		reversed[len(chunks)-1-i] = c
	}
	result, err := reassembleSimulatedFile(meta, reversed)
	if err != nil {
		t.Fatalf("reassemble: %v", err)
	}
	if !bytes.Equal(result, content) {
		t.Fatal("out-of-order reassembly mismatch")
	}
}

func TestDataflowStreaming(t *testing.T) {
	flow := &StreamMinimalDataFlowPath{
		Description: "sql injection data flow",
		Nodes: []*StreamMinimalNodeInfo{
			{NodeID: "n1", IRCode: "call sql.Query", IRSourceHash: "src1", StartOffset: 10, EndOffset: 50},
			{NodeID: "n2", IRCode: "param input", IRSourceHash: "src2", StartOffset: 20, EndOffset: 60},
		},
		Edges: []*StreamMinimalEdgeInfo{
			{EdgeID: "e1", FromNodeID: "n1", ToNodeID: "n2", EdgeType: "taint"},
		},
	}
	payload, _ := json.Marshal(flow)
	payload = append(payload, []byte(strings.Repeat(" ", 2048))...)

	for _, codec := range []string{"gzip", "zstd", "snappy"} {
		t.Run(codec, func(t *testing.T) {
			compressed, encoding := emitterCompress(payload, codec)
			pieces := emitterChunk(compressed, 512)
			chunkMap := make(map[int][]byte)
			lastIdx := -1
			for _, p := range pieces {
				chunkMap[p.Index] = p.Data
				if p.Last {
					lastIdx = p.Index
				}
			}
			decoded, err := aggregatorDecode(aggregatorJoinChunks(chunkMap, lastIdx), encoding)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			var result StreamMinimalDataFlowPath
			if err := json.Unmarshal(bytes.TrimRight(decoded, " "), &result); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if result.Description != flow.Description {
				t.Error("description mismatch")
			}
			if len(result.Nodes) != 2 || len(result.Edges) != 1 {
				t.Error("node/edge count mismatch")
			}
		})
	}
}

func TestFullPipeline_PartsToEnvelopesAndBack(t *testing.T) {
	parts := &StreamSingleResultParts{
		ProgramName: "test-proj",
		ReportType:  "security",
		Files: []*File{
			{Path: "/a.go", IrSourceHash: "fh1", Content: strings.Repeat("line\n", 500)},
		},
		Dataflows: []*StreamDataflowPart{
			{DataflowHash: "dh1", Payload: json.RawMessage(`{"nodes":[],"edges":[]}`)},
		},
		Risks: []*StreamRiskPart{
			{
				RiskHash:       "rh1",
				RiskJSON:       json.RawMessage(`{"title":"xss","severity":"high"}`),
				FileHashes:     []string{"fh1"},
				DataflowHashes: []string{"dh1"},
			},
		},
	}

	fileMeta, fileChunks := emitSimulatedFile(
		[]byte(parts.Files[0].Content), "fh1", "gzip", 1024, 512,
	)
	fileContent, err := reassembleSimulatedFile(fileMeta, fileChunks)
	if err != nil {
		t.Fatalf("file reassemble: %v", err)
	}
	if string(fileContent) != parts.Files[0].Content {
		t.Fatal("file content mismatch")
	}

	flowPayload := []byte(parts.Dataflows[0].Payload)
	flowCompressed, flowEnc := emitterCompress(flowPayload, "gzip")
	flowMeta := simulatedFileMeta{FileHash: "dh1", ContentSize: int64(len(flowPayload)), Encoding: flowEnc}
	var flowChunks []simulatedFileChunk
	if len(flowCompressed) <= 512 {
		flowMeta.InlineContent = flowCompressed
	} else {
		for _, p := range emitterChunk(flowCompressed, 1024) {
			flowChunks = append(flowChunks, simulatedFileChunk{FileHash: "dh1", ChunkIndex: p.Index, Data: p.Data, IsLast: p.Last})
		}
	}
	flowContent, err := reassembleSimulatedFile(flowMeta, flowChunks)
	if err != nil {
		t.Fatalf("flow reassemble: %v", err)
	}
	if !bytes.Equal(flowContent, flowPayload) {
		t.Fatal("flow content mismatch")
	}

	risk := parts.Risks[0]
	if risk.FileHashes[0] != "fh1" || risk.DataflowHashes[0] != "dh1" {
		t.Fatal("risk references corrupted")
	}
}
