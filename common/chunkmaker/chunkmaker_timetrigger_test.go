package chunkmaker

import (
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func TestChunkMakerWithTimeTrigger(t *testing.T) {
	t.Run("time_trigger_before_chunk_size_met", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(100) // Large chunk size
		triggerSeconds := 0.1   // Short time trigger

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithTimeTriggerSeconds(triggerSeconds))
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
		writeData := []byte("Hello")
		_, err = pw.Write(writeData)
		assert.NoError(t, err)
		log.Debug("Wrote initial data", "data", string(writeData))

		// Wait for time trigger (triggerSeconds + a small buffer)
		time.Sleep(time.Duration(triggerSeconds*1000+50) * time.Millisecond)

		err = pw.Close() // Close the writer to signal end of input
		assert.NoError(t, err)

		<-done // Wait for the collecting goroutine to finish

		assert.Len(t, chunks, 1, "Expected one chunk due to time trigger")
		if len(chunks) == 1 {
			assert.Equal(t, writeData, chunks[0], "Chunk data mismatch")
		}
	})

	t.Run("chunk_size_met_before_time_trigger", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(10) // Small chunk size
		triggerSeconds := 1.0  // Relatively long time trigger

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithTimeTriggerSeconds(triggerSeconds))
		assert.NoError(t, err)
		assert.NotNil(t, cm)

		var chunks [][]byte
		var done = make(chan struct{})
		var firstChunkReceivedTime time.Time
		var chunkReceiveOrder []int // To track order if needed

		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				if len(chunks) == 0 {
					firstChunkReceivedTime = time.Now()
				}
				log.Debug("Received chunk by chunk size", "data_hex", spew.Sdump(chunk.Data()))
				// Make a copy of the data, as the underlying buffer might be reused by the chunk object
				chunkCopy := make([]byte, len(chunk.Data()))
				copy(chunkCopy, chunk.Data())
				chunks = append(chunks, chunkCopy)
				chunkReceiveOrder = append(chunkReceiveOrder, len(chunkCopy))
			}
		}()

		writeData := []byte("ThisIsMoreThanTenBytes")                    // "ThisIsMore" (10) + "ThanTenBytes" (12) = 22 bytes
		expectedFirstChunk := writeData[:chunkSize]                      // "ThisIsMore"
		expectedSecondChunkPortion := writeData[chunkSize : chunkSize*2] // "ThanTenByt"
		expectedThirdChunkPortion := writeData[chunkSize*2:]             // "es"

		startTime := time.Now()
		_, err = pw.Write(writeData[:chunkSize]) // Write "ThisIsMore" (exactly chunkSize)
		assert.NoError(t, err)
		log.Debug("Wrote first part to meet chunk size", "data", string(writeData[:chunkSize]))

		// Wait a bit to ensure the first chunk is processed due to size.
		// This needs to be long enough for processing but shorter than triggerSeconds.
		time.Sleep(100 * time.Millisecond)

		_, err = pw.Write(writeData[chunkSize:]) // Write "ThanTenBytes"
		assert.NoError(t, err)
		log.Debug("Wrote second part of data", "data", string(writeData[chunkSize:]))

		err = pw.Close() // Close the writer to signal end of input and flush remaining
		assert.NoError(t, err)

		<-done // Wait for the collecting goroutine to finish
		log.Debug("Collected chunks", "count", len(chunks), "order", chunkReceiveOrder)

		assert.GreaterOrEqual(t, len(chunks), 1, "Expected at least one chunk")
		if len(chunks) > 0 {
			assert.Equal(t, expectedFirstChunk, chunks[0], "First chunk data mismatch")
			// Check if the first chunk was received well before the time trigger
			durationToReceiveFirstChunk := firstChunkReceivedTime.Sub(startTime)
			timeTriggerDuration := time.Duration(triggerSeconds * float64(time.Second))
			assert.True(t, durationToReceiveFirstChunk < timeTriggerDuration,
				"First chunk received too late (took %v), possibly due to time trigger (%v) instead of chunk size. Start: %v, Received: %v",
				durationToReceiveFirstChunk, timeTriggerDuration, startTime, firstChunkReceivedTime)
		}

		// Now check the subsequent chunks based on how FlushFullChunkSizeTo and FlushAllChunkSizeTo work
		if chunkSize*2 <= int64(len(writeData)) { // If enough data for a second full chunk
			assert.Len(t, chunks, 3, "Expected three chunks in total due to partial last chunk")
			if len(chunks) == 3 {
				assert.Equal(t, expectedSecondChunkPortion, chunks[1], "Second chunk portion data mismatch")
				assert.Equal(t, expectedThirdChunkPortion, chunks[2], "Third chunk (remaining) data mismatch")
			}
		} else if int64(len(writeData)) > chunkSize { // If data for one full chunk and a remainder
			assert.Len(t, chunks, 2, "Expected two chunks in total")
			if len(chunks) == 2 {
				assert.Equal(t, writeData[chunkSize:], chunks[1], "Second chunk (remaining) data mismatch")
			}
		}
		// If len(writeData) <= chunkSize, already covered by first chunk assertion or other tests
	})

	t.Run("multiple_time_triggers_and_final_flush_on_close", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(100) // Large chunk size
		triggerSeconds := 0.1   // Short time trigger, 100ms

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithTimeTriggerSeconds(triggerSeconds))
		assert.NoError(t, err)
		assert.NotNil(t, cm)

		var chunks [][]byte
		var done = make(chan struct{})

		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				log.Debug("Received chunk in multiple trigger test", spew.Sdump(chunk.Data()))
				chunks = append(chunks, append([]byte{}, chunk.Data()...)) // Make a copy
			}
		}()

		// Write data smaller than chunkSize, wait for time trigger
		data1 := []byte("data1")
		_, err = pw.Write(data1)
		assert.NoError(t, err)
		log.Debug("Wrote data1", "data", string(data1))
		time.Sleep(time.Duration(triggerSeconds*1000+50) * time.Millisecond) // Wait for trigger + buffer

		// Write more data, still smaller than chunkSize, wait for another time trigger
		data2 := []byte("data2")
		_, err = pw.Write(data2)
		assert.NoError(t, err)
		log.Debug("Wrote data2", "data", string(data2))
		time.Sleep(time.Duration(triggerSeconds*1000+50) * time.Millisecond) // Wait for trigger + buffer

		// Write final piece of data
		data3 := []byte("data3")
		_, err = pw.Write(data3)
		assert.NoError(t, err)
		log.Debug("Wrote data3", "data", string(data3))

		// Closing the pipe should trigger a final flush of any remaining data in buffer
		err = pw.Close()
		assert.NoError(t, err)

		<-done // Wait for the collecting goroutine to finish

		assert.Len(t, chunks, 3, "Expected three chunks due to multiple time triggers and final flush")
		if len(chunks) == 3 {
			assert.Equal(t, data1, chunks[0], "Chunk 1 data mismatch")
			assert.Equal(t, data2, chunks[1], "Chunk 2 data mismatch")
			assert.Equal(t, data3, chunks[2], "Chunk 3 data mismatch on close")
		}
	})

	t.Run("error_on_zero_time_trigger_interval", func(t *testing.T) {
		pr, _ := utils.NewPipe()
		_, err := NewTextChunkMaker(pr, WithChunkSize(10), WithTimeTriggerSeconds(0))
		assert.Error(t, err, "Expected error for zero time trigger interval")
		if err != nil {
			assert.Contains(t, err.Error(), "timeTriggerInterval must be positive", "Error message mismatch")
		}
	})

	t.Run("error_on_negative_time_trigger_interval", func(t *testing.T) {
		pr, _ := utils.NewPipe()
		_, err := NewTextChunkMaker(pr, WithChunkSize(10), WithTimeTriggerSeconds(-0.5))
		assert.Error(t, err, "Expected error for negative time trigger interval")
		if err != nil {
			assert.Contains(t, err.Error(), "timeTriggerInterval must be positive", "Error message mismatch")
		}
	})

	t.Run("close_before_first_time_trigger_flushes_buffer", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(100)
		triggerSeconds := 1.0 // Long time trigger

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithTimeTriggerSeconds(triggerSeconds))
		assert.NoError(t, err)
		assert.NotNil(t, cm)

		var chunks [][]byte
		var done = make(chan struct{})

		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				chunks = append(chunks, append([]byte{}, chunk.Data()...))
			}
		}()

		writeData := []byte("partial data")
		_, err = pw.Write(writeData)
		assert.NoError(t, err)
		log.Debug("Wrote partial data", "data", string(writeData))

		// Close immediately, well before time trigger would occur
		time.Sleep(50 * time.Millisecond) // Small delay to ensure write is processed by the loop
		err = pw.Close()
		assert.NoError(t, err)

		<-done

		assert.Len(t, chunks, 1, "Expected one chunk due to flush on close")
		if len(chunks) == 1 {
			assert.Equal(t, writeData, chunks[0], "Flushed chunk data mismatch")
		}
	})

	t.Run("no_data_written_with_time_trigger_and_close", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(10)
		triggerSeconds := 0.1 // Short time trigger

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithTimeTriggerSeconds(triggerSeconds))
		assert.NoError(t, err)
		assert.NotNil(t, cm)

		var chunks [][]byte
		var done = make(chan struct{})

		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				chunks = append(chunks, chunk.Data())
			}
		}()

		// Wait for a couple of time trigger intervals
		time.Sleep(time.Duration(triggerSeconds*1000*2.5) * time.Millisecond)

		err = pw.Close()
		assert.NoError(t, err)

		<-done

		assert.Len(t, chunks, 0, "Expected no chunks when no data is written")
	})

	t.Run("rapid_writes_chunksize_dominant", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(5)
		triggerSeconds := 0.5 // Time trigger is present but should mostly be superseded by chunkSize

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithTimeTriggerSeconds(triggerSeconds))
		assert.NoError(t, err)

		var chunks [][]byte
		done := make(chan struct{})
		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				chunkCopy := make([]byte, len(chunk.Data()))
				copy(chunkCopy, chunk.Data())
				chunks = append(chunks, chunkCopy)
				log.Debug("RapidWriteTest: Received chunk", "size", len(chunkCopy), "data", string(chunkCopy))
			}
		}()

		inputData := []byte("abcdefghijklmnopqrstuvwxyz12345") // 30 bytes

		var totalWritten int
		for i := 0; i < len(inputData); i++ {
			n, _ := pw.Write(inputData[i : i+1]) // Write byte by byte
			totalWritten += n
			time.Sleep(1 * time.Millisecond) // Very short sleep to allow processing per byte
		}
		log.Debug("RapidWriteTest: Total written bytes", "count", totalWritten)

		// Allow some time for the last few bytes to be processed by the loop if they form a full chunk,
		// or be in the buffer before close. This should be less than time trigger.
		time.Sleep(50 * time.Millisecond)
		pw.Close()
		<-done

		// For this specific input (30 bytes, chunkSize 5, byte-by-byte write):
		// 6 full chunks will be sent by FlushFullChunkSizeTo after 5th, 10th, ..., 25th, 30th byte.
		// No, after the 30th byte ('4') makes a full chunk ("z1234"), this chunk is sent.
		// Then the 31st byte ('5') is written. This '5' is alone in the buffer.
		// pw.Close() will flush this '5'.
		// Total = 6 full chunks + 1 chunk of size 1 = 7 chunks.
		expectedNumChunks := 7

		assert.Len(t, chunks, expectedNumChunks, "Incorrect number of chunks for rapid writes. Input: %s, ChunkSize: %d", inputData, chunkSize)

		var reassembledData []byte
		for i, c := range chunks {
			reassembledData = append(reassembledData, c...)
			if i < expectedNumChunks-1 { // All chunks except the last one (which is the single byte '5')
				assert.Equal(t, int(chunkSize), len(c), "Chunk %d size mismatch for rapid writes. Got %d, expected %d. Chunk: %s", i+1, len(c), chunkSize, string(c))
			} else { // Last chunk
				assert.Equal(t, 1, len(c), "Last chunk size mismatch (should be 1). Got %d. Chunk: %s", len(c), string(c))
			}
		}
		assert.Equal(t, inputData, reassembledData, "Reassembled data mismatch for rapid writes")
	})

	t.Run("slow_writes_timetrigger_dominant", func(t *testing.T) {
		pr, pw := utils.NewPipe()
		chunkSize := int64(100) // Large chunk size
		triggerSeconds := 0.1   // Short time trigger (100ms)

		cm, err := NewTextChunkMaker(pr, WithChunkSize(chunkSize), WithTimeTriggerSeconds(triggerSeconds))
		assert.NoError(t, err)

		var chunks [][]byte
		done := make(chan struct{})
		go func() {
			defer close(done)
			for chunk := range cm.OutputChannel() {
				chunkCopy := make([]byte, len(chunk.Data()))
				copy(chunkCopy, chunk.Data())
				chunks = append(chunks, chunkCopy)
				log.Debug("SlowWriteTest: Received chunk", "size", len(chunkCopy), "data", string(chunkCopy))
			}
		}()

		dataSegments := [][]byte{
			[]byte("segment1"), // 8 bytes
			[]byte("data2"),    // 5 bytes
			[]byte("lastbit"),  // 7 bytes
		}

		for i, segment := range dataSegments {
			_, err := pw.Write(segment)
			assert.NoError(t, err, "Failed to write segment %d", i+1)
			log.Debug("SlowWriteTest: Wrote segment", "index", i+1, "data", string(segment))
			time.Sleep(time.Duration(triggerSeconds*1000+50) * time.Millisecond)
		}

		pw.Close()
		<-done

		assert.Len(t, chunks, len(dataSegments), "Expected one chunk per slow write segment due to time trigger")
		if len(chunks) == len(dataSegments) {
			for i, segment := range dataSegments {
				assert.Equal(t, segment, chunks[i], "Data mismatch for segment %d", i+1)
			}
		}
	})
}
