package whisperutils

import (
	"fmt"
	"time"
)

// ExampleUsage demonstrates how to use the whisperutils package
func ExampleUsage() {
	// Example 1: Using WhisperCli to transcribe audio
	fmt.Println("=== Example 1: Using WhisperCli ===")

	// This would require actual audio file and model paths
	// audioFile := "/path/to/your/audio.mp3"
	// modelPath := "/path/to/your/model.gguf"
	// vadModelPath := "/path/to/your/vad_model.bin"

	// srtTargetPath := audioFile + ".srt"
	// results, err := InvokeWhisperCli(audioFile, srtTargetPath,
	// 	WithModelPath(modelPath),
	// 	WithVAD(true),
	// 	WithVADModelPath(vadModelPath),
	// 	WithDebug(false),
	// )
	// if err != nil {
	// 	fmt.Printf("Error: %v\n", err)
	// 	return
	// }

	// for result := range results {
	// 	fmt.Printf("[%s -> %s] %s\n", result.StartTime, result.EndTime, result.Text)
	// }

	// Example 2: Working with SRT files
	fmt.Println("=== Example 2: Working with SRT files ===")

	// Sample SRT content
	srtContent := `1
00:00:01,000 --> 00:00:05,000
Hello, this is the first subtitle.

2
00:00:06,000 --> 00:00:10,000
This is the second subtitle.

3
00:00:11,000 --> 00:00:15,000
And this is the third subtitle.`

	// Create SRT manager from content
	manager, err := NewSRTManagerFromContent(srtContent)
	if err != nil {
		fmt.Printf("Error creating SRT manager: %v\n", err)
		return
	}

	// Get all entries
	entries := manager.GetEntries()
	fmt.Printf("Total entries: %d\n", len(entries))

	// Example 3: Getting context around a specific time
	fmt.Println("\n=== Example 3: Getting context around time ===")

	// Get context around 8 seconds with ±3 seconds interval
	context := manager.GetSRTContextByOffsetSeconds(8.0, 3.0)
	fmt.Printf("Context for 8 seconds ± 3 seconds:\n")
	fmt.Printf("Target Time: %v\n", context.TargetTime)
	fmt.Printf("Interval: %v\n", context.Interval)
	fmt.Printf("Context Text: %s\n", context.ContextText)
	fmt.Printf("Number of context entries: %d\n", len(context.ContextEntries))

	for i, entry := range context.ContextEntries {
		fmt.Printf("  Entry %d: [%v -> %v] %s\n", i+1, entry.StartTime, entry.EndTime, entry.Text)
	}

	// Example 4: Modifying SRT entries
	fmt.Println("\n=== Example 4: Modifying SRT entries ===")

	// Add a new entry
	manager.AddEntry(16*time.Second, 20*time.Second, "This is a new subtitle entry.")

	// Update an existing entry
	err = manager.UpdateEntry(2, "This is the updated second subtitle.")
	if err != nil {
		fmt.Printf("Error updating entry: %v\n", err)
	}

	// Get entries in a specific time range
	rangeEntries := manager.GetEntriesInTimeRange(5*time.Second, 15*time.Second)
	fmt.Printf("Entries between 5-15 seconds: %d\n", len(rangeEntries))

	for _, entry := range rangeEntries {
		fmt.Printf("  [%v -> %v] %s\n", entry.StartTime, entry.EndTime, entry.Text)
	}

	// Example 5: Export back to SRT format
	fmt.Println("\n=== Example 5: Export to SRT format ===")

	srtOutput := manager.ToSRT()
	fmt.Printf("Generated SRT:\n%s\n", srtOutput)

	// Example 6: Loading SRT from file
	fmt.Println("\n=== Example 6: Loading SRT from file ===")

	// Load SRT manager from file
	// manager2, err := NewSRTManagerFromFile("/path/to/your/subtitle.srt")
	// if err != nil {
	// 	fmt.Printf("Error loading SRT file: %v\n", err)
	// 	return
	// }
	//
	// context2 := manager2.GetSRTContextByOffsetSeconds(30.0, 5*time.Second)
	// fmt.Printf("Context from file around 30 seconds: %s\n", context2.ContextText)
}
