package chunkmaker_test

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils"
)

// ExampleNewChunkMaker_basic demonstrates basic usage of ChunkMaker with a specific chunk size.
func ExampleNewChunkMaker_basic() {
	inputData := "This is a test string for basic chunking."
	chunkSize := int64(10)

	pr, pw := utils.NewPipe()

	cm, err := chunkmaker.NewTextChunkMaker(pr, chunkmaker.WithChunkSize(chunkSize))
	if err != nil {
		log.Fatalf("Failed to create ChunkMaker: %v", err)
	}

	go func() {
		defer pw.Close()
		_, err := pw.Write([]byte(inputData))
		if err != nil {
			log.Printf("Error writing to pipe: %v", err)
		}
	}()

	fmt.Printf("Input: %s\nChunkSize: %d\n", inputData, chunkSize)
	fmt.Println("Output Chunks:")
	for chunk := range cm.OutputChannel() {
		fmt.Printf(" - [%s] (Size: %d)\n", string(chunk.Data()), len(chunk.Data()))
	}

	// Output:
	// Input: This is a test string for basic chunking.
	// ChunkSize: 10
	// Output Chunks:
	//  - [This is a ] (Size: 10)
	//  - [test strin] (Size: 10)
	//  - [g for basi] (Size: 10)
	//  - [c chunking] (Size: 10)
	//  - [.] (Size: 1)
}

// ExampleNewChunkMaker_withTimeTrigger demonstrates using ChunkMaker with a time trigger.
func ExampleNewChunkMaker_withTimeTrigger() {
	chunkSize := int64(100) // Large chunk size, so time trigger should activate first
	triggerSeconds := 0.1   // 100ms time trigger
	sampleData := "Short data"

	pr, pw := utils.NewPipe()

	cm, err := chunkmaker.NewTextChunkMaker(pr,
		chunkmaker.WithChunkSize(chunkSize),
		chunkmaker.WithTimeTriggerSeconds(triggerSeconds),
	)
	if err != nil {
		log.Fatalf("Failed to create ChunkMaker: %v", err)
	}

	var receivedChunks []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		for chunk := range cm.OutputChannel() {
			receivedChunks = append(receivedChunks, string(chunk.Data()))
		}
	}()

	fmt.Printf("Writing data: '%s'\n", sampleData)
	_, err = pw.Write([]byte(sampleData))
	if err != nil {
		log.Printf("Error writing to pipe: %v", err)
	}

	// Wait long enough for the time trigger to activate
	time.Sleep(time.Duration(triggerSeconds*1000+50) * time.Millisecond)

	// Write more data, this should also be flushed by time trigger or close
	nextSampleData := "More data"
	fmt.Printf("Writing more data: '%s'\n", nextSampleData)
	_, err = pw.Write([]byte(nextSampleData))
	if err != nil {
		log.Printf("Error writing to pipe: %v", err)
	}
	// Wait for the next time trigger
	time.Sleep(time.Duration(triggerSeconds*1000+50) * time.Millisecond)

	pw.Close() // Close the pipe to flush any remaining data and stop the collector
	<-done     // Wait for collector goroutine to finish

	fmt.Println("Received Chunks by Time Trigger:")
	for i, s := range receivedChunks {
		fmt.Printf(" - Chunk %d: [%s]\n", i+1, s)
	}

	// Output:
	// Writing data: 'Short data'
	// Writing more data: 'More data'
	// Received Chunks by Time Trigger:
	//  - Chunk 1: [Short data]
	//  - Chunk 2: [More data]
}

// ExampleNewChunkMaker_utf8 demonstrates ChunkMaker's handling of UTF-8 characters when ChunkSize is based on rune count.
func ExampleNewChunkMaker_utf8() {
	inputData := "你好世界123"    // 4 runes (你好世界) + 3 runes (123) = 7 runes
	chunkRuneSize := int64(2) // Each chunk should have 2 runes

	pr, pw := utils.NewPipe()

	// Note: By default, if input is valid UTF-8, ChunkSize is interpreted as rune count.
	cm, err := chunkmaker.NewTextChunkMaker(pr, chunkmaker.WithChunkSize(chunkRuneSize))
	if err != nil {
		log.Fatalf("Failed to create ChunkMaker: %v", err)
	}

	go func() {
		defer pw.Close()
		_, err := pw.Write([]byte(inputData))
		if err != nil {
			log.Printf("Error writing to pipe: %v", err)
		}
	}()

	fmt.Printf("Input (UTF-8): %s\nChunkRuneSize: %d\n", inputData, chunkRuneSize)
	fmt.Println("Output Chunks (UTF-8):")
	var outputOrder []string
	for chunk := range cm.OutputChannel() {
		outputOrder = append(outputOrder, fmt.Sprintf(" - [%s] (Runes: %d, Bytes: %d)", string(chunk.Data()), chunk.RunesSize(), chunk.BytesSize()))
	}
	// Sort to ensure consistent output for example testing, as map iteration order is not guaranteed if there were internal maps
	// For ChunkMaker, output order is generally preserved from input.
	// sort.Strings(outputOrder)
	for _, s := range outputOrder {
		fmt.Println(s)
	}

	// Output:
	// Input (UTF-8): 你好世界123
	// ChunkRuneSize: 2
	// Output Chunks (UTF-8):
	//  - [你好] (Runes: 2, Bytes: 6)
	//  - [世界] (Runes: 2, Bytes: 6)
	//  - [12] (Runes: 2, Bytes: 2)
	//  - [3] (Runes: 1, Bytes: 1)
}

// ExampleNewChunkMaker_invalidChunkSize demonstrates error handling for invalid ChunkSize.
func ExampleNewChunkMaker_invalidChunkSize() {
	pr, _ := utils.NewPipe()
	_, err := chunkmaker.NewTextChunkMaker(pr, chunkmaker.WithChunkSize(0))
	if err != nil {
		fmt.Printf("Error: %s\n", strings.Split(err.Error(), ": ")[1]) // Simplify error for consistent example output
	}

	_, err = chunkmaker.NewTextChunkMaker(pr, chunkmaker.WithChunkSize(-5))
	if err != nil {
		fmt.Printf("Error: %s\n", strings.Split(err.Error(), ": ")[1])
	}
	// Output:
	// Error: ChunkSize must be positive, got 0
	// Error: ChunkSize must be positive, got -5
}

// ExampleNewChunkMaker_invalidTimeTrigger demonstrates error handling for invalid timeTriggerInterval.
func ExampleNewChunkMaker_invalidTimeTrigger() {
	pr, _ := utils.NewPipe()
	_, err := chunkmaker.NewTextChunkMaker(pr, chunkmaker.WithChunkSize(10), chunkmaker.WithTimeTriggerSeconds(0))
	if err != nil {
		fmt.Printf("Error: %s\n", strings.Split(err.Error(), ": ")[1]) // Simplify error for consistent example output
	}

	_, err = chunkmaker.NewTextChunkMaker(pr, chunkmaker.WithChunkSize(10), chunkmaker.WithTimeTriggerSeconds(-0.1))
	if err != nil {
		fmt.Printf("Error: %s\n", strings.Split(err.Error(), ": ")[1])
	}
	// Output:
	// Error: timeTriggerInterval must be positive when time trigger is enabled, got 0s
	// Error: timeTriggerInterval must be positive when time trigger is enabled, got -100ms
}

// ExampleNewChunkMaker_withPrevNBytes demonstrates using PrevNBytes with linked chunks.
func ExampleNewChunkMaker_withPrevNBytes() {
	inputData := "abcdefghijklmnopqrs"
	chunkSize := int64(5)

	pr, pw := utils.NewPipe() // Use a pipe to simulate io.Reader input

	cm, err := chunkmaker.NewTextChunkMaker(pr, chunkmaker.WithChunkSize(chunkSize))
	if err != nil {
		log.Fatalf("Failed to create ChunkMaker: %v", err)
	}

	// Write data in a separate goroutine
	go func() {
		defer pw.Close()                           // Close the writer when done
		_, errWrite := pw.Write([]byte(inputData)) // Write all data at once.
		// Temporarily simplifying the error check to isolate the linter issue
		if errWrite != nil {
			// log.Printf("Error writing to pipe: %v", errWrite) // Temporarily commented out to see if this is the source of the error
		}
	}()

	fmt.Printf("Input data: '%s'\nChunkSize: %d\n\n", inputData, chunkSize)

	var collectedChunks []chunkmaker.Chunk
	for chunk := range cm.OutputChannel() {
		collectedChunks = append(collectedChunks, chunk)
	}

	// Iterate through collected chunks to show linking and PrevNBytes
	for i, currentChunk := range collectedChunks {
		fmt.Printf("Chunk %d: [\"%s\"]\n", i, string(currentChunk.Data()))

		if currentChunk.HaveLastChunk() {
			lastChunk := currentChunk.LastChunk()
			fmt.Printf("  - Has Prev Chunk: true (Data: [\"%s\"])\n", string(lastChunk.Data()))
		} else {
			fmt.Printf("  - Has Prev Chunk: false\n")
		}

		// Demonstrate PrevNBytes
		// Get previous 3 bytes (if available)
		prev3Bytes := currentChunk.PrevNBytes(3)
		if len(prev3Bytes) > 0 {
			fmt.Printf("  - PrevNBytes(3): [\"%s\"]\n", string(prev3Bytes))
		} else {
			fmt.Printf("  - PrevNBytes(3): [] (no previous data or not enough)\n")
		}

		// Get previous 7 bytes (if available)
		prev7Bytes := currentChunk.PrevNBytes(7)
		if len(prev7Bytes) > 0 {
			fmt.Printf("  - PrevNBytes(7): [\"%s\"]\n", string(prev7Bytes))
		} else {
			fmt.Printf("  - PrevNBytes(7): [] (no previous data or not enough)\n")
		}
		fmt.Println()
	}

	// Expected output based on chunkSize=5 and input "abcdefghijklmnopqrs"
	// Chunks: "abcde", "fghij", "klmno", "pqrs"
	// Output:
	// Input data: 'abcdefghijklmnopqrs'
	// ChunkSize: 5
	//
	// Chunk 0: ["abcde"]
	//   - Has Prev Chunk: false
	//   - PrevNBytes(3): [] (no previous data or not enough)
	//   - PrevNBytes(7): [] (no previous data or not enough)
	//
	// Chunk 1: ["fghij"]
	//   - Has Prev Chunk: true (Data: ["abcde"])
	//   - PrevNBytes(3): ["cde"]
	//   - PrevNBytes(7): ["abcde"]
	//
	// Chunk 2: ["klmno"]
	//   - Has Prev Chunk: true (Data: ["fghij"])
	//   - PrevNBytes(3): ["hij"]
	//   - PrevNBytes(7): ["defghij"]
	//
	// Chunk 3: ["pqrs"]
	//   - Has Prev Chunk: true (Data: ["klmno"])
	//   - PrevNBytes(3): ["mno"]
	//   - PrevNBytes(7): ["hijklmno"]
}
