package chunkmaker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

// TestChunkMakerWithSeparatorAsBoundary validates the "separator-as-boundary"
// mode introduced alongside the default "separator-as-trigger" mode. In the
// boundary mode the chunk maker fills up to chunkSize and prefers to cut at
// the LAST separator occurrence within the window so that pre-structured
// blocks remain intact instead of being emitted one by one.
func TestChunkMakerWithSeparatorAsBoundary(t *testing.T) {
	t.Run("small_blocks_packed_into_one_chunk", func(t *testing.T) {
		// 10 blocks of ~50 bytes each separated by "\n--- end ---\n".
		// With chunkSize=4096 in boundary mode, all of them should coalesce
		// into a single chunk at close (flush-all).
		pr, pw := utils.NewPipe()
		chunkSize := int64(4096)
		sep := "\n--- end ---\n"

		cm, err := NewTextChunkMaker(pr,
			WithChunkSize(chunkSize),
			WithSeparatorTrigger(sep),
			WithSeparatorAsBoundary(true),
		)
		assert.NoError(t, err)

		var chunks [][]byte
		done := make(chan struct{})
		go func() {
			defer close(done)
			for ch := range cm.OutputChannel() {
				cp := make([]byte, len(ch.Data()))
				copy(cp, ch.Data())
				chunks = append(chunks, cp)
			}
		}()

		var buf strings.Builder
		for i := 0; i < 10; i++ {
			buf.WriteString("--- candidate ---\npayload-")
			buf.WriteString(strings.Repeat("x", 20))
			buf.WriteString(sep)
		}
		_, err = pw.Write([]byte(buf.String()))
		assert.NoError(t, err)
		_ = pw.Close()
		<-done

		assert.Len(t, chunks, 1, "10 small blocks (~500B total) must coalesce into 1 chunk under boundary mode")
		assert.Equal(t, buf.String(), string(chunks[0]))
	})

	t.Run("fill_until_chunksize_then_cut_at_last_separator", func(t *testing.T) {
		// Write enough blocks to exceed chunkSize twice. Each block is exactly
		// 100 bytes (including separator). chunkSize=500. Boundary mode should
		// pack 5 blocks per chunk (500 bytes, cut at separator boundary).
		pr, pw := utils.NewPipe()
		chunkSize := int64(500)
		sep := "|SEP|"

		cm, err := NewTextChunkMaker(pr,
			WithChunkSize(chunkSize),
			WithSeparatorTrigger(sep),
			WithSeparatorAsBoundary(true),
		)
		assert.NoError(t, err)

		var chunks [][]byte
		done := make(chan struct{})
		go func() {
			defer close(done)
			for ch := range cm.OutputChannel() {
				cp := make([]byte, len(ch.Data()))
				copy(cp, ch.Data())
				chunks = append(chunks, cp)
			}
		}()

		// Each block = 95 bytes payload + 5 bytes separator = 100 bytes total.
		block := strings.Repeat("A", 95) + sep
		blocks := 12
		var full strings.Builder
		for i := 0; i < blocks; i++ {
			full.WriteString(block)
		}
		_, err = pw.Write([]byte(full.String()))
		assert.NoError(t, err)
		_ = pw.Close()
		<-done

		// Expected: first two full chunks of 500 bytes each, last chunk 200 bytes.
		assert.Len(t, chunks, 3)
		if len(chunks) == 3 {
			assert.Equal(t, 500, len(chunks[0]))
			assert.Equal(t, 500, len(chunks[1]))
			assert.Equal(t, 200, len(chunks[2]))
			assert.True(t, strings.HasSuffix(string(chunks[0]), sep), "chunk must end at separator boundary")
			assert.True(t, strings.HasSuffix(string(chunks[1]), sep), "chunk must end at separator boundary")
		}

		var joined strings.Builder
		for _, c := range chunks {
			joined.Write(c)
		}
		assert.Equal(t, full.String(), joined.String())
	})

	t.Run("block_larger_than_chunksize_splits_at_chunksize", func(t *testing.T) {
		// A single block of 800 bytes with no internal separator: when
		// chunkSize=300, boundary mode must fall back to hard-cut at chunkSize.
		pr, pw := utils.NewPipe()
		chunkSize := int64(300)
		sep := "|SEP|"

		cm, err := NewTextChunkMaker(pr,
			WithChunkSize(chunkSize),
			WithSeparatorTrigger(sep),
			WithSeparatorAsBoundary(true),
		)
		assert.NoError(t, err)

		var chunks [][]byte
		done := make(chan struct{})
		go func() {
			defer close(done)
			for ch := range cm.OutputChannel() {
				cp := make([]byte, len(ch.Data()))
				copy(cp, ch.Data())
				chunks = append(chunks, cp)
			}
		}()

		data := strings.Repeat("B", 800)
		_, err = pw.Write([]byte(data))
		assert.NoError(t, err)
		_ = pw.Close()
		<-done

		// Expected: 300+300+200
		assert.Len(t, chunks, 3)
		if len(chunks) == 3 {
			assert.Equal(t, 300, len(chunks[0]))
			assert.Equal(t, 300, len(chunks[1]))
			assert.Equal(t, 200, len(chunks[2]))
		}
	})

	t.Run("legacy_trigger_mode_unaffected", func(t *testing.T) {
		// Same input as block_larger_than_chunksize_splits_at_chunksize but
		// without enabling boundary mode; legacy every-separator-triggers
		// behavior must still hold.
		pr, pw := utils.NewPipe()
		chunkSize := int64(500)
		sep := "|SEP|"

		cm, err := NewTextChunkMaker(pr,
			WithChunkSize(chunkSize),
			WithSeparatorTrigger(sep),
			// WithSeparatorAsBoundary NOT set -> legacy trigger mode
		)
		assert.NoError(t, err)

		var chunks [][]byte
		done := make(chan struct{})
		go func() {
			defer close(done)
			for ch := range cm.OutputChannel() {
				cp := make([]byte, len(ch.Data()))
				copy(cp, ch.Data())
				chunks = append(chunks, cp)
			}
		}()

		// 3 blocks of 100 bytes each; legacy mode must emit one chunk per block.
		block := strings.Repeat("C", 95) + sep
		_, err = pw.Write([]byte(block + block + block))
		assert.NoError(t, err)
		_ = pw.Close()
		<-done

		assert.Len(t, chunks, 3, "legacy trigger mode must emit one chunk per separator occurrence")
		for _, c := range chunks {
			assert.Equal(t, 100, len(c))
			assert.True(t, strings.HasSuffix(string(c), sep))
		}
	})
}
