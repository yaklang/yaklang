package whisperutils

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

func TestInvokeWhisperCli(t *testing.T) {
	// This test requires a local whisper-cli setup and models.
	// 1. Download whisper-cli binary and place it in a searchable path or set YAK_WHISPER_CLI_PATH.
	// 2. Download a whisper model (e.g., ggml-medium-q5.gguf) and set YAK_WHISPER_MODEL_PATH.
	// 3. Download the silero VAD model and set YAK_WHISPER_VAD_MODEL_PATH if using VAD.
	modelPath := consts.GetWhisperModelMediumPath()
	if modelPath == "" || !utils.FileExists(modelPath) {
		t.Skip("skipping test: YAK_WHISPER_MODEL_PATH is not set or model file not found")
	}

	vadModelPath := consts.GetWhisperSileroVADPath()
	if vadModelPath == "" || !utils.FileExists(vadModelPath) {
		t.Skip("skipping test: YAK_WHISPER_VAD_MODEL_PATH is not set or VAD model file not found")
	}

	audioFile := "/Users/v1ll4n/yakit-projects/projects/libs/output.mp3"
	if !utils.FileExists(audioFile) {
		t.Fatalf("test audio file not found: %s", audioFile)
	}

	srtTargetPath := audioFile + ".srt"
	results, err := InvokeWhisperCli(audioFile, srtTargetPath,
		WithModelPath(modelPath),
		WithVAD(true),
		WithVADModelPath(vadModelPath),
		WithDebug(true),
		WithLogWriter(os.Stdout),
	)
	if err != nil {
		t.Fatalf("InvokeWhisperCli failed: %v", err)
	}

	for res := range results {
		fmt.Printf("%v [%s -> %s] %s\n", time.Now(), res.StartTime, res.EndTime, res.Text)
	}
}

func TestSRTManager(t *testing.T) {
	// Test SRT parsing and context retrieval
	srtContent := `1
00:00:01,000 --> 00:00:05,000
Hello, this is the first subtitle.

2
00:00:06,000 --> 00:00:10,000
This is the second subtitle.

3
00:00:11,000 --> 00:00:15,000
And this is the third subtitle.

4
00:00:16,000 --> 00:00:20,000
Finally, the fourth subtitle.`

	manager, err := NewSRTManagerFromContent(srtContent)
	if err != nil {
		t.Fatalf("Failed to create SRT manager: %v", err)
	}

	entries := manager.GetEntries()
	if len(entries) != 4 {
		t.Fatalf("Expected 4 entries, got %d", len(entries))
	}

	// Test context retrieval
	context := manager.GetSRTContextByOffsetSeconds(8.0, 5.0) // 8 seconds ± 5 seconds
	if context == nil {
		t.Fatal("Context should not be nil")
	}

	fmt.Printf("Context for 8 seconds ± 5 seconds:\n")
	fmt.Printf("Target Time: %v\n", context.TargetTime)
	fmt.Printf("Interval: %v\n", context.Interval)
	fmt.Printf("Context Text: %s\n", context.ContextText)
	fmt.Printf("Context Entries: %d\n", len(context.ContextEntries))

	// Should include entries 1, 2, and possibly 3
	if len(context.ContextEntries) < 2 {
		t.Fatalf("Expected at least 2 context entries, got %d", len(context.ContextEntries))
	}

	// Test that context text contains expected content
	if !strings.Contains(context.ContextText, "second subtitle") {
		t.Fatalf("Context text should contain 'second subtitle', got: %s", context.ContextText)
	}
}

func TestSRTManagerOperations(t *testing.T) {
	manager := NewSRTManager()

	// Test adding entries
	manager.AddEntry(1*time.Second, 5*time.Second, "First entry")
	manager.AddEntry(6*time.Second, 10*time.Second, "Second entry")
	manager.AddEntry(11*time.Second, 15*time.Second, "Third entry")

	entries := manager.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// Test updating entry
	err := manager.UpdateEntry(2, "Updated second entry")
	if err != nil {
		t.Fatalf("Failed to update entry: %v", err)
	}

	// Verify update
	entries = manager.GetEntries()
	if entries[1].Text != "Updated second entry" {
		t.Fatalf("Entry was not updated correctly, got: %s", entries[1].Text)
	}

	// Test removing entry
	err = manager.RemoveEntry(2)
	if err != nil {
		t.Fatalf("Failed to remove entry: %v", err)
	}

	entries = manager.GetEntries()
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries after removal, got %d", len(entries))
	}

	// Test SRT output
	srtOutput := manager.ToSRT()
	if !strings.Contains(srtOutput, "First entry") {
		t.Fatal("SRT output should contain 'First entry'")
	}
	if !strings.Contains(srtOutput, "Third entry") {
		t.Fatal("SRT output should contain 'Third entry'")
	}
	if strings.Contains(srtOutput, "Updated second entry") {
		t.Fatal("SRT output should not contain removed entry")
	}
}

func TestSRTContextByOffsetSeconds(t *testing.T) {
	manager := NewSRTManager()

	// Add some test entries
	manager.AddEntry(0*time.Second, 3*time.Second, "开始")
	manager.AddEntry(4*time.Second, 7*time.Second, "中间部分")
	manager.AddEntry(8*time.Second, 11*time.Second, "结束部分")
	manager.AddEntry(15*time.Second, 18*time.Second, "最后一段")

	// Test GetSRTContextByOffsetSeconds - should get context around 6 seconds
	context := manager.GetSRTContextByOffsetSeconds(6.0, 3.0)

	if context.TargetTime != 6*time.Second {
		t.Fatalf("Expected target time 6s, got %v", context.TargetTime)
	}

	if context.Interval != 3*time.Second {
		t.Fatalf("Expected interval 3s, got %v", context.Interval)
	}

	// Should include entries that overlap with [3s, 9s] window
	// Entry 1: [4s, 7s] - overlaps
	// Entry 2: [8s, 11s] - overlaps
	expectedMinEntries := 2
	if len(context.ContextEntries) < expectedMinEntries {
		t.Fatalf("Expected at least %d context entries, got %d", expectedMinEntries, len(context.ContextEntries))
	}

	// Check that context text contains expected content
	if !strings.Contains(context.ContextText, "中间部分") {
		t.Fatalf("Context text should contain '中间部分', got: %s", context.ContextText)
	}

	fmt.Printf("Context for 6 seconds ± 3 seconds:\n")
	fmt.Printf("Target Time: %v\n", context.TargetTime)
	fmt.Printf("Context Text: %s\n", context.ContextText)
	fmt.Printf("New String() format:\n%s\n", context.String())
	for i, entry := range context.ContextEntries {
		fmt.Printf("Entry %d: [%v -> %v] %s\n", i+1, entry.StartTime, entry.EndTime, entry.Text)
	}
}

func TestSRTManagerFromFile(t *testing.T) {
	// Create a temporary SRT file for testing
	srtContent := `1
00:00:01,000 --> 00:00:05,000
测试字幕第一条

2
00:00:06,000 --> 00:00:10,000
测试字幕第二条`

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test_*.srt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write SRT content to file
	_, err = tmpFile.WriteString(srtContent)
	if err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test loading from file
	manager, err := NewSRTManagerFromFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create SRT manager from file: %v", err)
	}

	entries := manager.GetEntries()
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].Text != "测试字幕第一条" {
		t.Fatalf("Expected '测试字幕第一条', got '%s'", entries[0].Text)
	}

	if entries[1].Text != "测试字幕第二条" {
		t.Fatalf("Expected '测试字幕第二条', got '%s'", entries[1].Text)
	}

	fmt.Printf("Successfully loaded %d entries from SRT file\n", len(entries))
}
