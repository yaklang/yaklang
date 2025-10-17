package chunkmaker

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestChunkMakerWithSepTrigger(t *testing.T) {
	t.Run("sep_trigger_before_chunk_size_met", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(100) // Large chunk size

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithSeparatorTrigger("\n"))
		assert.NoError(t, err)
		assert.NotNil(t, cm)

		var chunks [][]byte
		var done = make(chan struct{})

		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				log.Debug("Received chunk by time trigger", spew.Sdump(chunk.Data()))
				chunks = append(chunks, chunk.Data())
			}
		}()

		// Write some data, less than chunkSize
		writeData := []byte("Hello\n")
		_, err = pw.Write(writeData)
		assert.NoError(t, err)
		log.Debug("Wrote initial data", "data", string(writeData))

		err = pw.Close() // Close the writer to signal end of input
		assert.NoError(t, err)

		<-done // Wait for the collecting goroutine to finish

		assert.Len(t, chunks, 1, "Expected one chunk due to time trigger")
		if len(chunks) == 1 {
			assert.Equal(t, writeData, chunks[0], "Chunk data mismatch")
		}
	})

	t.Run("sep_trigger_and_chunk_size_met", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(5)

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithSeparatorTrigger("\n"))
		assert.NoError(t, err)
		assert.NotNil(t, cm)

		var chunks [][]byte
		var done = make(chan struct{})

		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				log.Debug("Received chunk by time trigger", spew.Sdump(chunk.Data()))
				chunks = append(chunks, chunk.Data())
			}
		}()

		// Write some data, less than chunkSize

		writeData1 := []byte("Hello")
		writeData2 := []byte(",\n")
		writeData3 := []byte("World")
		_, err = pw.Write(append(writeData1, append(writeData2, writeData3...)...))
		assert.NoError(t, err)
		log.Debug("Wrote initial data", "data", string(writeData1))
		err = pw.Close() // Close the writer to signal end of input
		assert.NoError(t, err)

		<-done // Wait for the collecting goroutine to finish

		assert.Len(t, chunks, 3, "Expected one chunk due to time trigger")
		if len(chunks) == 3 {
			assert.Equal(t, writeData1, chunks[0], "Chunk data mismatch")
			assert.Equal(t, writeData2, chunks[1], "Chunk data mismatch")
			assert.Equal(t, writeData3, chunks[2], "Chunk data mismatch")
		}

	})

	t.Run("long_sep_trigger", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(100)

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithSeparatorTrigger("\n\n"))
		assert.NoError(t, err)
		assert.NotNil(t, cm)

		var chunks [][]byte
		var done = make(chan struct{})

		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				log.Debug("Received chunk by time trigger", spew.Sdump(chunk.Data()))
				chunks = append(chunks, chunk.Data())
			}
		}()

		// Write some data, less than chunkSize

		writeData1 := []byte("Hello\nWorld\n\n")
		writeData2 := []byte("Hello again\n")
		writeData3 := []byte("World\n\n")
		_, err = pw.Write(append(writeData1, append(writeData2, writeData3...)...))
		assert.NoError(t, err)
		log.Debug("Wrote initial data", "data", string(writeData1))
		err = pw.Close() // Close the writer to signal end of input
		assert.NoError(t, err)

		<-done // Wait for the collecting goroutine to finish

		assert.Len(t, chunks, 2, "Expected one chunk due to time trigger")
		if len(chunks) == 2 {
			assert.Equal(t, writeData1, chunks[0], "Chunk data mismatch")
			assert.Equal(t, append(writeData2, writeData3...), chunks[1], "Chunk data mismatch")
		}

	})

}
