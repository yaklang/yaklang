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
// Helpers: simulate emitter-side compress + chunk and aggregator-side reassemble + decompress.
// These mirror the algorithms in scannode/stream_emitter.go and legion/server/scanstream/aggregator.go.
// ---------------------------------------------------------------------------

func emitterCompress(raw []byte, codec string) ([]byte, string) {
	if codec == "" || len(raw) < 1024 {
		return raw, ""
	}
	var enc []byte
	switch codec {
	case "gzip":
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err != nil {
			return raw, ""
		}
		_, _ = zw.Write(raw)
		_ = zw.Close()
		enc = buf.Bytes()
	case "zstd":
		w, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
		if err != nil {
			return raw, ""
		}
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
		pieces = append(pieces, chunkPiece{
			Index: i,
			Data:  data[off:end],
			Last:  end >= len(data),
		})
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

// roundtrip: compress → chunk → reassemble → decompress → compare
func roundtripData(t *testing.T, original []byte, codec string, chunkSize int) {
	t.Helper()

	compressed, encoding := emitterCompress(original, codec)

	pieces := emitterChunk(compressed, chunkSize)
	if len(pieces) == 0 {
		t.Fatal("emitterChunk returned 0 pieces")
	}
	if !pieces[len(pieces)-1].Last {
		t.Fatal("last piece should have Last=true")
	}

	chunkMap := make(map[int][]byte)
	lastIdx := -1
	for _, p := range pieces {
		chunkMap[p.Index] = p.Data
		if p.Last {
			lastIdx = p.Index
		}
	}
	if lastIdx < 0 {
		t.Fatal("no last chunk marker")
	}
	if len(chunkMap) != lastIdx+1 {
		t.Fatalf("chunk count mismatch: got %d, want %d", len(chunkMap), lastIdx+1)
	}

	joined := aggregatorJoinChunks(chunkMap, lastIdx)
	if !bytes.Equal(joined, compressed) {
		t.Fatalf("joined data != compressed data (len %d vs %d)", len(joined), len(compressed))
	}

	decoded, err := aggregatorDecode(joined, encoding)
	if err != nil {
		t.Fatalf("decode failed (encoding=%s): %v", encoding, err)
	}
	if !bytes.Equal(decoded, original) {
		t.Fatalf("roundtrip mismatch: original len=%d, decoded len=%d", len(original), len(decoded))
	}
}

// ---------------------------------------------------------------------------
// Test data generators
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

// ---------------------------------------------------------------------------
// Tests: maybeCompress behavior
// ---------------------------------------------------------------------------

func TestEmitterCompress_SmallBypass(t *testing.T) {
	small := []byte("hello world")
	for _, codec := range []string{"gzip", "zstd", "snappy"} {
		out, encoding := emitterCompress(small, codec)
		if encoding != "" {
			t.Errorf("codec=%s: expected no compression for small data, got encoding=%s", codec, encoding)
		}
		if !bytes.Equal(out, small) {
			t.Errorf("codec=%s: data should be unchanged", codec)
		}
	}
}

func TestEmitterCompress_NoCodec(t *testing.T) {
	data := makeTextData(4096)
	out, encoding := emitterCompress(data, "")
	if encoding != "" {
		t.Errorf("expected no encoding, got %s", encoding)
	}
	if !bytes.Equal(out, data) {
		t.Error("data should be unchanged with empty codec")
	}
}

func TestEmitterCompress_GzipReducesSize(t *testing.T) {
	data := makeTextData(8192)
	out, encoding := emitterCompress(data, "gzip")
	if encoding != "gzip" {
		t.Fatalf("expected gzip encoding, got %q", encoding)
	}
	if len(out) >= len(data) {
		t.Fatalf("gzip should reduce size: %d → %d", len(data), len(out))
	}
}

func TestEmitterCompress_ZstdReducesSize(t *testing.T) {
	data := makeTextData(8192)
	out, encoding := emitterCompress(data, "zstd")
	if encoding != "zstd" {
		t.Fatalf("expected zstd encoding, got %q", encoding)
	}
	if len(out) >= len(data) {
		t.Fatalf("zstd should reduce size: %d → %d", len(data), len(out))
	}
}

func TestEmitterCompress_SnappyReducesSize(t *testing.T) {
	data := makeTextData(8192)
	out, encoding := emitterCompress(data, "snappy")
	if encoding != "snappy" {
		t.Fatalf("expected snappy encoding, got %q", encoding)
	}
	if len(out) >= len(data) {
		t.Fatalf("snappy should reduce size: %d → %d", len(data), len(out))
	}
}

func TestEmitterCompress_RandomDataMaySkip(t *testing.T) {
	data := makeRandomData(4096)
	_, encoding := emitterCompress(data, "gzip")
	// Random data is nearly incompressible; compression may be skipped.
	// Either outcome is acceptable, just verify no panic.
	_ = encoding
}

// ---------------------------------------------------------------------------
// Tests: compress → decompress roundtrip
// ---------------------------------------------------------------------------

func TestCompressDecompressRoundtrip_AllCodecs(t *testing.T) {
	data := makeTextData(16384)
	for _, codec := range []string{"gzip", "zstd", "snappy"} {
		t.Run(codec, func(t *testing.T) {
			compressed, encoding := emitterCompress(data, codec)
			if encoding == "" {
				t.Skip("compression skipped (not enough gain)")
			}
			decoded, err := aggregatorDecode(compressed, encoding)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if !bytes.Equal(decoded, data) {
				t.Fatalf("roundtrip mismatch: len %d → %d", len(data), len(decoded))
			}
		})
	}
}

func TestCompressDecompressRoundtrip_JSONPayload(t *testing.T) {
	data := makeJSONData(20)
	for _, codec := range []string{"gzip", "zstd", "snappy"} {
		t.Run(codec, func(t *testing.T) {
			compressed, encoding := emitterCompress(data, codec)
			if encoding == "" {
				t.Skip("compression skipped")
			}
			decoded, err := aggregatorDecode(compressed, encoding)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if !bytes.Equal(decoded, data) {
				t.Fatal("roundtrip mismatch")
			}
			var parts StreamSingleResultParts
			if err := json.Unmarshal(decoded, &parts); err != nil {
				t.Fatalf("JSON unmarshal failed after roundtrip: %v", err)
			}
			if parts.ProgramName != "test-program" {
				t.Fatalf("ProgramName mismatch: %q", parts.ProgramName)
			}
			if len(parts.Risks) != 20 {
				t.Fatalf("risk count mismatch: %d", len(parts.Risks))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: emitterChunk
// ---------------------------------------------------------------------------

func TestEmitterChunk_Empty(t *testing.T) {
	pieces := emitterChunk(nil, 256)
	if len(pieces) != 1 {
		t.Fatalf("expected 1 piece for empty data, got %d", len(pieces))
	}
	if !pieces[0].Last || pieces[0].Index != 0 {
		t.Fatal("single piece should be index=0 and Last=true")
	}
}

func TestEmitterChunk_SmallerThanChunkSize(t *testing.T) {
	data := []byte("small payload")
	pieces := emitterChunk(data, 1024)
	if len(pieces) != 1 {
		t.Fatalf("expected 1 piece, got %d", len(pieces))
	}
	if !bytes.Equal(pieces[0].Data, data) || !pieces[0].Last {
		t.Fatal("single piece should contain all data and be last")
	}
}

func TestEmitterChunk_ExactMultiple(t *testing.T) {
	data := make([]byte, 512)
	for i := range data {
		data[i] = byte(i % 256)
	}
	pieces := emitterChunk(data, 256)
	if len(pieces) != 2 {
		t.Fatalf("expected 2 pieces for 512/256, got %d", len(pieces))
	}
	if pieces[0].Last {
		t.Fatal("first piece should not be last")
	}
	if !pieces[1].Last {
		t.Fatal("second piece should be last")
	}
	if !bytes.Equal(append(pieces[0].Data, pieces[1].Data...), data) {
		t.Fatal("concatenated chunks should equal original")
	}
}

func TestEmitterChunk_NotExactMultiple(t *testing.T) {
	data := make([]byte, 700)
	for i := range data {
		data[i] = byte(i % 256)
	}
	pieces := emitterChunk(data, 256)
	if len(pieces) != 3 {
		t.Fatalf("expected 3 pieces for 700/256, got %d", len(pieces))
	}
	if len(pieces[0].Data) != 256 || len(pieces[1].Data) != 256 || len(pieces[2].Data) != 188 {
		t.Fatalf("chunk sizes: %d, %d, %d", len(pieces[0].Data), len(pieces[1].Data), len(pieces[2].Data))
	}
	if !pieces[2].Last {
		t.Fatal("last piece should have Last=true")
	}
	var buf bytes.Buffer
	for _, p := range pieces {
		buf.Write(p.Data)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatal("concatenated chunks should equal original")
	}
}

func TestEmitterChunk_SingleByte(t *testing.T) {
	pieces := emitterChunk([]byte{0x42}, 256)
	if len(pieces) != 1 {
		t.Fatalf("expected 1 piece, got %d", len(pieces))
	}
	if !bytes.Equal(pieces[0].Data, []byte{0x42}) || !pieces[0].Last {
		t.Fatal("single byte chunk mismatch")
	}
}

// ---------------------------------------------------------------------------
// Tests: aggregatorJoinChunks
// ---------------------------------------------------------------------------

func TestJoinChunks_NegativeLastIdx(t *testing.T) {
	result := aggregatorJoinChunks(nil, -1)
	if result != nil {
		t.Fatal("expected nil for negative lastIdx")
	}
}

func TestJoinChunks_SingleChunk(t *testing.T) {
	chunks := map[int][]byte{0: []byte("hello")}
	result := aggregatorJoinChunks(chunks, 0)
	if string(result) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(result))
	}
}

func TestJoinChunks_MultipleChunks(t *testing.T) {
	chunks := map[int][]byte{
		0: []byte("aaa"),
		1: []byte("bbb"),
		2: []byte("ccc"),
	}
	result := aggregatorJoinChunks(chunks, 2)
	if string(result) != "aaabbbccc" {
		t.Fatalf("expected 'aaabbbccc', got %q", string(result))
	}
}

func TestJoinChunks_MissingMiddleChunk(t *testing.T) {
	chunks := map[int][]byte{
		0: []byte("aaa"),
		// 1 is missing
		2: []byte("ccc"),
	}
	result := aggregatorJoinChunks(chunks, 2)
	// Missing chunk produces empty bytes for that slot.
	if string(result) != "aaaccc" {
		t.Fatalf("expected 'aaaccc', got %q", string(result))
	}
}

// ---------------------------------------------------------------------------
// Tests: aggregatorDecode edge cases
// ---------------------------------------------------------------------------

func TestAggregateDecode_EmptyEncoding(t *testing.T) {
	data := []byte("raw bytes")
	out, err := aggregatorDecode(data, "")
	if err != nil || !bytes.Equal(out, data) {
		t.Fatal("empty encoding should return data as-is")
	}
}

func TestAggregateDecode_NoneEncoding(t *testing.T) {
	data := []byte("raw bytes")
	out, err := aggregatorDecode(data, "none")
	if err != nil || !bytes.Equal(out, data) {
		t.Fatal("'none' encoding should return data as-is")
	}
}

func TestAggregateDecode_UnknownEncoding(t *testing.T) {
	data := []byte("raw bytes")
	out, err := aggregatorDecode(data, "brotli")
	if err != nil || !bytes.Equal(out, data) {
		t.Fatal("unknown encoding should return data as-is")
	}
}

func TestAggregateDecode_InvalidGzip(t *testing.T) {
	_, err := aggregatorDecode([]byte("not gzip"), "gzip")
	if err == nil {
		t.Fatal("expected error for invalid gzip data")
	}
}

func TestAggregateDecode_InvalidSnappy(t *testing.T) {
	_, err := aggregatorDecode([]byte("not snappy"), "snappy")
	if err == nil {
		t.Fatal("expected error for invalid snappy data")
	}
}

func TestAggregateDecode_InvalidZstd(t *testing.T) {
	_, err := aggregatorDecode([]byte("not zstd"), "zstd")
	if err == nil {
		t.Fatal("expected error for invalid zstd data")
	}
}

// ---------------------------------------------------------------------------
// Tests: full roundtrip (compress → chunk → join → decompress)
// ---------------------------------------------------------------------------

func TestRoundtrip_NoCompression(t *testing.T) {
	data := makeTextData(2048)
	roundtripData(t, data, "", 512)
}

func TestRoundtrip_GzipSmallChunk(t *testing.T) {
	data := makeTextData(8192)
	roundtripData(t, data, "gzip", 1024)
}

func TestRoundtrip_GzipLargeChunk(t *testing.T) {
	data := makeTextData(8192)
	roundtripData(t, data, "gzip", 256*1024)
}

func TestRoundtrip_ZstdSmallChunk(t *testing.T) {
	data := makeTextData(8192)
	roundtripData(t, data, "zstd", 1024)
}

func TestRoundtrip_SnappySmallChunk(t *testing.T) {
	data := makeTextData(8192)
	roundtripData(t, data, "snappy", 1024)
}

func TestRoundtrip_LargePayload_AllCodecs(t *testing.T) {
	data := makeTextData(512 * 1024) // 512KB
	for _, codec := range []string{"", "gzip", "zstd", "snappy"} {
		name := codec
		if name == "" {
			name = "none"
		}
		t.Run(name, func(t *testing.T) {
			roundtripData(t, data, codec, 256*1024)
		})
	}
}

func TestRoundtrip_JSONPayloadIntegrity(t *testing.T) {
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
			joined := aggregatorJoinChunks(chunkMap, lastIdx)
			decoded, err := aggregatorDecode(joined, encoding)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			var parts StreamSingleResultParts
			if err := json.Unmarshal(decoded, &parts); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if parts.ProgramName != "test-program" {
				t.Errorf("ProgramName = %q", parts.ProgramName)
			}
			if len(parts.Risks) != 30 {
				t.Errorf("risk count = %d, want 30", len(parts.Risks))
			}
			if len(parts.Files) != 2 {
				t.Errorf("file count = %d, want 2", len(parts.Files))
			}
			for i, r := range parts.Risks {
				expected := fmt.Sprintf("riskhash_%04d", i)
				if r.RiskHash != expected {
					t.Errorf("risk[%d] hash = %q, want %q", i, r.RiskHash, expected)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: simulate full file streaming pipeline (meta → inline / chunks)
// ---------------------------------------------------------------------------

type simulatedFileMeta struct {
	FileHash      string `json:"file_hash"`
	ContentSize   int64  `json:"content_size"`
	Encoding      string `json:"encoding,omitempty"`
	InlineContent []byte `json:"inline_content,omitempty"`
}

type simulatedFileChunk struct {
	FileHash   string `json:"file_hash"`
	ChunkIndex int    `json:"chunk_index"`
	Data       []byte `json:"data"`
	IsLast     bool   `json:"is_last"`
}

// emitSimulatedFile mimics StreamEmitter.emitFile: compress → decide inline vs chunk.
func emitSimulatedFile(content []byte, fileHash, codec string, chunkSize, inlineMax int) (meta simulatedFileMeta, chunks []simulatedFileChunk) {
	compressed, encoding := emitterCompress(content, codec)
	meta = simulatedFileMeta{
		FileHash:    fileHash,
		ContentSize: int64(len(content)),
		Encoding:    encoding,
	}
	if inlineMax > 0 && len(compressed) > 0 && len(compressed) <= inlineMax {
		meta.InlineContent = compressed
		return meta, nil
	}
	pieces := emitterChunk(compressed, chunkSize)
	for _, p := range pieces {
		chunks = append(chunks, simulatedFileChunk{
			FileHash:   fileHash,
			ChunkIndex: p.Index,
			Data:       p.Data,
			IsLast:     p.Last,
		})
	}
	return meta, chunks
}

// reassembleSimulatedFile mimics Aggregator's setFileMeta + addFileChunk.
func reassembleSimulatedFile(meta simulatedFileMeta, chunks []simulatedFileChunk) ([]byte, error) {
	if len(meta.InlineContent) > 0 {
		content := meta.InlineContent
		if meta.Encoding != "" {
			decoded, err := aggregatorDecode(content, meta.Encoding)
			if err != nil {
				return nil, fmt.Errorf("decode inline: %w", err)
			}
			return decoded, nil
		}
		return content, nil
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
	if len(chunkMap) != lastIdx+1 {
		return nil, fmt.Errorf("missing chunks: have %d, need %d", len(chunkMap), lastIdx+1)
	}
	joined := aggregatorJoinChunks(chunkMap, lastIdx)
	if meta.Encoding != "" {
		return aggregatorDecode(joined, meta.Encoding)
	}
	return joined, nil
}

func TestFileStreaming_SmallInline(t *testing.T) {
	content := []byte("package main\nfunc main() {}\n")
	meta, chunks := emitSimulatedFile(content, "hash1", "gzip", 256*1024, 16*1024)

	// Small file: should be inlined (below compression threshold, so no compression either)
	if len(chunks) != 0 {
		// Small data < 1024 bytes won't be compressed, so inlined as raw
	}
	result, err := reassembleSimulatedFile(meta, chunks)
	if err != nil {
		t.Fatalf("reassemble failed: %v", err)
	}
	if !bytes.Equal(result, content) {
		t.Fatalf("content mismatch: got %d bytes, want %d", len(result), len(content))
	}
}

func TestFileStreaming_LargeGzipChunked(t *testing.T) {
	content := makeTextData(100 * 1024) // 100KB
	// Use very small inlineMax to force chunking even after gzip compression.
	meta, chunks := emitSimulatedFile(content, "hash_large", "gzip", 256, 64)

	if meta.Encoding != "gzip" {
		t.Fatalf("expected gzip encoding, got %q", meta.Encoding)
	}
	if len(meta.InlineContent) != 0 {
		t.Fatal("large file should not be inlined")
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	result, err := reassembleSimulatedFile(meta, chunks)
	if err != nil {
		t.Fatalf("reassemble failed: %v", err)
	}
	if !bytes.Equal(result, content) {
		t.Fatalf("content mismatch: got %d bytes, want %d", len(result), len(content))
	}
}

func TestFileStreaming_AllCodecs(t *testing.T) {
	content := makeTextData(50 * 1024)
	for _, codec := range []string{"", "gzip", "zstd", "snappy"} {
		name := codec
		if name == "" {
			name = "none"
		}
		t.Run(name, func(t *testing.T) {
			meta, chunks := emitSimulatedFile(content, "fh_"+name, codec, 16*1024, 4*1024)
			result, err := reassembleSimulatedFile(meta, chunks)
			if err != nil {
				t.Fatalf("reassemble failed: %v", err)
			}
			if !bytes.Equal(result, content) {
				t.Fatalf("roundtrip mismatch")
			}
		})
	}
}

func TestFileStreaming_ChunksOutOfOrder(t *testing.T) {
	content := makeTextData(30 * 1024)
	meta, chunks := emitSimulatedFile(content, "fh_ooo", "gzip", 4*1024, 1*1024)

	if len(chunks) < 2 {
		t.Skip("need multiple chunks for out-of-order test")
	}

	// Reverse chunk order to simulate out-of-order delivery
	reversed := make([]simulatedFileChunk, len(chunks))
	for i, c := range chunks {
		reversed[len(chunks)-1-i] = c
	}

	result, err := reassembleSimulatedFile(meta, reversed)
	if err != nil {
		t.Fatalf("reassemble failed: %v", err)
	}
	if !bytes.Equal(result, content) {
		t.Fatalf("out-of-order reassembly mismatch")
	}
}

// ---------------------------------------------------------------------------
// Tests: simulate full dataflow streaming pipeline
// ---------------------------------------------------------------------------

func TestDataflowStreaming_Roundtrip(t *testing.T) {
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
	payload, err := json.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}
	// Pad payload to exceed 1024 to trigger compression
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
			joined := aggregatorJoinChunks(chunkMap, lastIdx)
			decoded, err := aggregatorDecode(joined, encoding)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			var result StreamMinimalDataFlowPath
			trimmed := bytes.TrimRight(decoded, " ")
			if err := json.Unmarshal(trimmed, &result); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if result.Description != flow.Description {
				t.Errorf("description mismatch")
			}
			if len(result.Nodes) != 2 || len(result.Edges) != 1 {
				t.Errorf("node/edge count mismatch")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: StreamSingleResultParts full pipeline simulation
// ---------------------------------------------------------------------------

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

	codec := "gzip"
	chunkSize := 1024
	inlineMax := 512

	// Simulate emitter: files
	fileMeta, fileChunks := emitSimulatedFile(
		[]byte(parts.Files[0].Content), parts.Files[0].IrSourceHash,
		codec, chunkSize, inlineMax,
	)

	// Simulate emitter: dataflows
	flowPayload := []byte(parts.Dataflows[0].Payload)
	// Dataflow payload is small, will not be compressed (< 1024)
	flowCompressed, flowEncoding := emitterCompress(flowPayload, codec)
	flowMeta := simulatedFileMeta{
		FileHash:    parts.Dataflows[0].DataflowHash,
		ContentSize: int64(len(flowPayload)),
		Encoding:    flowEncoding,
	}
	var flowChunks []simulatedFileChunk
	if inlineMax > 0 && len(flowCompressed) <= inlineMax {
		flowMeta.InlineContent = flowCompressed
	} else {
		for _, p := range emitterChunk(flowCompressed, chunkSize) {
			flowChunks = append(flowChunks, simulatedFileChunk{
				FileHash:   parts.Dataflows[0].DataflowHash,
				ChunkIndex: p.Index,
				Data:       p.Data,
				IsLast:     p.Last,
			})
		}
	}

	// Simulate aggregator: reassemble file
	fileContent, err := reassembleSimulatedFile(fileMeta, fileChunks)
	if err != nil {
		t.Fatalf("file reassemble failed: %v", err)
	}
	if string(fileContent) != parts.Files[0].Content {
		t.Fatalf("file content mismatch: %d vs %d", len(fileContent), len(parts.Files[0].Content))
	}

	// Simulate aggregator: reassemble dataflow
	flowContent, err := reassembleSimulatedFile(flowMeta, flowChunks)
	if err != nil {
		t.Fatalf("flow reassemble failed: %v", err)
	}
	if !bytes.Equal(flowContent, flowPayload) {
		t.Fatalf("flow content mismatch")
	}

	// Verify risk references are intact
	risk := parts.Risks[0]
	if risk.FileHashes[0] != "fh1" || risk.DataflowHashes[0] != "dh1" {
		t.Fatal("risk references corrupted")
	}

	// Verify full JSON reconstruction
	reconstructed := &StreamSingleResultParts{
		ProgramName: parts.ProgramName,
		ReportType:  parts.ReportType,
		Files:       []*File{{Path: "/a.go", IrSourceHash: "fh1", Content: string(fileContent)}},
		Dataflows:   []*StreamDataflowPart{{DataflowHash: "dh1", Payload: flowContent}},
		Risks:       parts.Risks,
	}
	if reconstructed.ProgramName != "test-proj" {
		t.Fatal("program name mismatch")
	}
	if reconstructed.Files[0].Content != parts.Files[0].Content {
		t.Fatal("reconstructed file content mismatch")
	}
}
