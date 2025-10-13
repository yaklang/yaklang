package aireducer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

//go:embed testdata/demo.txt.zip
var demoFileZipContent []byte

func TestAIReducer(t *testing.T) {
	zfs, err := filesys.NewZipFSFromString(string(demoFileZipContent))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := zfs.ReadFile("demo.txt")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	reducer, err := NewReducerFromString(
		string(raw),
		WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			count++
			log.Infof("Processing chunk: %d bytes", len(chunk.Data()))
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = reducer.Run()
	if err != nil {
		t.Fatal(err)
	}
	if count <= 1 {
		t.Fatal("Reducer did not process any chunks, expected more than 1")
	}
}

// Test configuration options
func TestConfigOptions(t *testing.T) {
	tests := []struct {
		name   string
		config func() *Config
		check  func(*testing.T, *Config)
	}{
		{
			name: "Default config",
			config: func() *Config {
				return NewConfig()
			},
			check: func(t *testing.T, c *Config) {
				if c.ChunkSize != 1024 {
					t.Errorf("Expected default ChunkSize 1024, got %d", c.ChunkSize)
				}
				if c.TimeTriggerInterval != 0 {
					t.Errorf("Expected default TimeTriggerInterval 0, got %v", c.TimeTriggerInterval)
				}
				if c.Memory == nil {
					t.Error("Expected Memory to be initialized")
				}
			},
		},
		{
			name: "Custom chunk size",
			config: func() *Config {
				return NewConfig(WithChunkSize(2048))
			},
			check: func(t *testing.T, c *Config) {
				if c.ChunkSize != 2048 {
					t.Errorf("Expected ChunkSize 2048, got %d", c.ChunkSize)
				}
			},
		},
		{
			name: "Time trigger interval",
			config: func() *Config {
				return NewConfig(WithTimeTriggerInterval(5 * time.Second))
			},
			check: func(t *testing.T, c *Config) {
				if c.TimeTriggerInterval != 5*time.Second {
					t.Errorf("Expected TimeTriggerInterval 5s, got %v", c.TimeTriggerInterval)
				}
			},
		},
		{
			name: "Time trigger interval seconds",
			config: func() *Config {
				return NewConfig(WithTimeTriggerIntervalSeconds(3.0))
			},
			check: func(t *testing.T, c *Config) {
				// 3.0 seconds should result in 3 * time.Second
				expected := 3 * time.Second
				if c.TimeTriggerInterval != expected {
					t.Errorf("Expected TimeTriggerInterval %v, got %v", expected, c.TimeTriggerInterval)
				}
			},
		},
		{
			name: "Separator trigger",
			config: func() *Config {
				return NewConfig(WithSeparatorTrigger("\n"))
			},
			check: func(t *testing.T, c *Config) {
				if c.SeparatorTrigger != "\n" {
					t.Errorf("Expected SeparatorTrigger '\\n', got '%s'", c.SeparatorTrigger)
				}
			},
		},
		{
			name: "Custom context",
			config: func() *Config {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return NewConfig(WithContext(ctx))
			},
			check: func(t *testing.T, c *Config) {
				if c.ctx == nil {
					t.Error("Expected context to be set")
				}
			},
		},
		{
			name: "Custom memory",
			config: func() *Config {
				memory := aid.GetDefaultMemory()
				return NewConfig(WithMemory(memory))
			},
			check: func(t *testing.T, c *Config) {
				if c.Memory == nil {
					t.Error("Expected Memory to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config()
			tt.check(t, config)
		})
	}
}

// Test different reducer creation methods
func TestReducerCreation(t *testing.T) {
	testData := "This is test data for reducer\nSecond line\nThird line"

	tests := []struct {
		name     string
		createFn func() (*Reducer, error)
	}{
		{
			name: "From string",
			createFn: func() (*Reducer, error) {
				return NewReducerFromString(testData, WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
					return nil
				}))
			},
		},
		{
			name: "From reader",
			createFn: func() (*Reducer, error) {
				return NewReducerFromReader(strings.NewReader(testData), WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
					return nil
				}))
			},
		},
		{
			name: "From file",
			createFn: func() (*Reducer, error) {
				// Create temporary file
				tmpFile, err := os.CreateTemp("", "reducer_test_*.txt")
				if err != nil {
					return nil, err
				}
				defer os.Remove(tmpFile.Name())

				if _, err := tmpFile.WriteString(testData); err != nil {
					tmpFile.Close()
					return nil, err
				}
				tmpFile.Close()

				return NewReducerFromFile(tmpFile.Name(), WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
					return nil
				}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reducer, err := tt.createFn()
			if err != nil {
				t.Fatalf("Failed to create reducer: %v", err)
			}
			if reducer == nil {
				t.Fatal("Reducer should not be nil")
			}
		})
	}
}

// Test callback functionality
func TestReducerCallbacks(t *testing.T) {
	testData := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"

	t.Run("Basic callback", func(t *testing.T) {
		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithChunkSize(10), // Small chunk size to ensure multiple chunks
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		if len(chunks) == 0 {
			t.Fatal("Expected at least one chunk")
		}

		// Verify all chunks combined equal original data
		combined := strings.Join(chunks, "")
		if combined != testData {
			t.Errorf("Combined chunks don't match original data.\nExpected: %q\nGot: %q", testData, combined)
		}
	})

	t.Run("Finish callback", func(t *testing.T) {
		finishCalled := false

		reducer, err := NewReducerFromString(testData,
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				return nil
			}),
			WithFinishCallback(func(config *Config, memory *aid.PromptContextProvider) error {
				finishCalled = true
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		if !finishCalled {
			t.Error("Finish callback was not called")
		}
	})

	t.Run("Simple callback", func(t *testing.T) {
		var chunkCount int

		reducer, err := NewReducerFromString(testData,
			WithChunkSize(10),
			WithSimpleCallback(func(chunk chunkmaker.Chunk) {
				chunkCount++
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		if chunkCount == 0 {
			t.Error("Simple callback was not called")
		}
	})
}

// Test error handling
func TestErrorHandling(t *testing.T) {
	t.Run("Nil callback error", func(t *testing.T) {
		_, err := NewReducerFromString("test data")
		if err == nil {
			t.Error("Expected error for nil callback")
		}
		if !strings.Contains(err.Error(), "callback is nil") {
			t.Errorf("Expected 'callback is nil' error, got: %v", err)
		}
	})

	t.Run("Callback error propagation", func(t *testing.T) {
		expectedErr := errors.New("callback error")

		reducer, err := NewReducerFromString("test data",
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				return expectedErr
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err == nil {
			t.Error("Expected error from callback")
		}
		if !strings.Contains(err.Error(), expectedErr.Error()) {
			t.Errorf("Expected error to contain '%v', got: %v", expectedErr, err)
		}
	})

	t.Run("File not found error", func(t *testing.T) {
		_, err := NewReducerFromFile("/non/existent/file.txt",
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				return nil
			}))
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	t.Run("Simple callback panic recovery", func(t *testing.T) {
		reducer, err := NewReducerFromString("test data",
			WithSimpleCallback(func(chunk chunkmaker.Chunk) {
				panic("test panic")
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err == nil {
			t.Error("Expected error from panic recovery")
		}
	})

	t.Run("Finish callback error", func(t *testing.T) {
		expectedErr := errors.New("finish error")

		reducer, err := NewReducerFromString("test data",
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				return nil
			}),
			WithFinishCallback(func(config *Config, memory *aid.PromptContextProvider) error {
				return expectedErr
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err == nil {
			t.Error("Expected error from finish callback")
		}
		if err != expectedErr {
			t.Errorf("Expected finish callback error, got: %v", err)
		}
	})
}

// Test separator-based chunking
func TestSeparatorChunking(t *testing.T) {
	testData := "chunk1|chunk2|chunk3|chunk4"

	var chunks []string
	var mu sync.Mutex

	reducer, err := NewReducerFromString(testData,
		WithSeparatorTrigger("|"),
		WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			mu.Lock()
			chunks = append(chunks, string(chunk.Data()))
			mu.Unlock()
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}

	err = reducer.Run()
	if err != nil {
		t.Fatal(err)
	}

	// Should have chunks split by separator
	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk")
	}

	log.Infof("Separator chunks: %v", chunks)
}

// Test context cancellation
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Large data to ensure processing takes time
	largeData := strings.Repeat("This is a line of test data.\n", 1000)

	reducer, err := NewReducerFromString(largeData,
		WithContext(ctx),
		WithChunkSize(100),
		WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			// Cancel after first chunk
			cancel()
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}

	err = reducer.Run()
	// Context cancellation might cause early termination, which is expected behavior
	log.Infof("Context cancellation test completed with error: %v", err)
}

// Test edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("Empty string", func(t *testing.T) {
		callbackCalled := false

		reducer, err := NewReducerFromString("",
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				callbackCalled = true
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		// Empty string might not trigger callback, which is acceptable
		log.Infof("Empty string test: callback called = %v", callbackCalled)
	})

	t.Run("Single character", func(t *testing.T) {
		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString("a",
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		if len(chunks) > 0 {
			combined := strings.Join(chunks, "")
			if combined != "a" {
				t.Errorf("Expected 'a', got '%s'", combined)
			}
		}
	})

	t.Run("Very large chunk size", func(t *testing.T) {
		testData := "small data"
		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithChunkSize(1000000), // Much larger than data
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		if len(chunks) > 0 {
			if len(chunks) > 1 {
				t.Errorf("Expected at most 1 chunk with large chunk size, got %d", len(chunks))
			}
		}
	})

	t.Run("Zero chunk size fallback", func(t *testing.T) {
		config := NewConfig(WithChunkSize(0))
		if config.ChunkSize != 1024 {
			t.Errorf("Expected fallback to 1024 for zero chunk size, got %d", config.ChunkSize)
		}
	})

	t.Run("Negative chunk size fallback", func(t *testing.T) {
		config := NewConfig(WithChunkSize(-100))
		if config.ChunkSize != 1024 {
			t.Errorf("Expected fallback to 1024 for negative chunk size, got %d", config.ChunkSize)
		}
	})
}

// Test memory functionality
func TestMemoryIntegration(t *testing.T) {
	testData := "data line 1\ndata line 2\ndata line 3"
	memory := aid.GetDefaultMemory()

	var processedChunks int

	reducer, err := NewReducerFromString(testData,
		WithMemory(memory),
		WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			processedChunks++
			if memory == nil {
				t.Error("Memory should not be nil in callback")
			}
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}

	err = reducer.Run()
	if err != nil {
		t.Fatal(err)
	}

	if processedChunks == 0 {
		t.Error("No chunks were processed")
	}
}

// Test concurrent processing safety
func TestConcurrentSafety(t *testing.T) {
	testData := strings.Repeat("concurrent test line\n", 100)

	var wg sync.WaitGroup
	const numGoroutines = 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			var chunkCount int
			reducer, err := NewReducerFromString(testData,
				WithChunkSize(50),
				WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
					chunkCount++
					return nil
				}))
			if err != nil {
				t.Errorf("Goroutine %d: Failed to create reducer: %v", id, err)
				return
			}

			err = reducer.Run()
			if err != nil {
				t.Errorf("Goroutine %d: Failed to run reducer: %v", id, err)
				return
			}

			log.Infof("Goroutine %d processed %d chunks", id, chunkCount)
		}(i)
	}

	wg.Wait()
}

// Test time-triggered chunking
func TestTimeTriggerChunking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping time-based test in short mode")
	}

	testData := "slow data processing"
	var chunks []string
	var mu sync.Mutex
	var timestamps []time.Time

	reducer, err := NewReducerFromString(testData,
		WithTimeTriggerInterval(100*time.Millisecond), // Short interval for testing
		WithChunkSize(1000),                           // Large chunk size so time trigger takes precedence
		WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			mu.Lock()
			chunks = append(chunks, string(chunk.Data()))
			timestamps = append(timestamps, time.Now())
			mu.Unlock()
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}

	// Start the reducer and let it run for a bit
	done := make(chan error, 1)
	go func() {
		done <- reducer.Run()
	}()

	// Wait for some processing time
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		// Force early termination by canceling context
		if reducer.config.cancel != nil {
			reducer.config.cancel()
		}
		<-done // Wait for completion
	}

	mu.Lock()
	chunkCount := len(chunks)
	mu.Unlock()

	log.Infof("Time trigger test: processed %d chunks", chunkCount)
	// We should have received at least one chunk from time trigger
	if chunkCount == 0 {
		t.Error("Expected at least one chunk from time trigger")
	}
}

// Test ChunkMakerOption generation
func TestChunkMakerOption(t *testing.T) {
	config := NewConfig(
		WithChunkSize(2048),
		WithTimeTriggerInterval(5*time.Second),
		WithSeparatorTrigger("\n\n"),
	)

	options := config.ChunkMakerOption()
	if len(options) == 0 {
		t.Error("Expected at least one chunk maker option")
	}

	// Options should be present but we can't easily test their values
	// since they're internal to chunkmaker package
	log.Infof("Generated %d chunk maker options", len(options))
}

// Test line-based chunking functionality
func TestLinesChunking(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		lines     int
		chunkSize int64
		expected  []string
	}{
		{
			name:      "Simple lines chunking",
			data:      "line1\nline2\nline3\nline4\nline5\n",
			lines:     2,
			chunkSize: 1024,
			expected:  []string{"line1\nline2\n", "line3\nline4\n", "line5\n"},
		},
		{
			name:      "Lines exceed chunk size",
			data:      "very long line that exceeds chunk size\nanother very long line that also exceeds the chunk size\nshort line\n",
			lines:     2,
			chunkSize: 50,  // Small chunk size to force splitting
			expected:  nil, // We'll verify splitting occurs
		},
		{
			name:      "Single line per chunk",
			data:      "line1\nline2\nline3\n",
			lines:     1,
			chunkSize: 1024,
			expected:  []string{"line1\n", "line2\n", "line3\n"},
		},
		{
			name:      "More lines than available",
			data:      "line1\nline2\n",
			lines:     5,
			chunkSize: 1024,
			expected:  []string{"line1\nline2\n"},
		},
		{
			name:      "Empty lines handling",
			data:      "line1\n\nline3\n",
			lines:     2,
			chunkSize: 1024,
			expected:  []string{"line1\n\n", "line3\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var chunks []string
			var mu sync.Mutex

			reducer, err := NewReducerFromString(tt.data,
				WithLines(tt.lines),
				WithChunkSize(tt.chunkSize),
				WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
					mu.Lock()
					chunks = append(chunks, string(chunk.Data()))
					mu.Unlock()
					return nil
				}))
			if err != nil {
				t.Fatalf("Failed to create reducer: %v", err)
			}

			err = reducer.Run()
			if err != nil {
				t.Fatalf("Failed to run reducer: %v", err)
			}

			mu.Lock()
			actualChunks := make([]string, len(chunks))
			copy(actualChunks, chunks)
			mu.Unlock()

			if tt.expected != nil {
				if len(actualChunks) != len(tt.expected) {
					t.Errorf("Expected %d chunks, got %d", len(tt.expected), len(actualChunks))
					t.Logf("Expected chunks: %v", tt.expected)
					t.Logf("Actual chunks: %v", actualChunks)
					return
				}

				for i, expected := range tt.expected {
					if i < len(actualChunks) && actualChunks[i] != expected {
						t.Errorf("Chunk %d: expected %q, got %q", i, expected, actualChunks[i])
					}
				}
			} else {
				// For cases where we expect splitting due to chunk size
				if len(actualChunks) == 0 {
					t.Error("Expected at least one chunk")
				}

				// Verify all chunks combined equal original data
				combined := strings.Join(actualChunks, "")
				if combined != tt.data {
					t.Errorf("Combined chunks don't match original data.\nExpected: %q\nGot: %q", tt.data, combined)
				}

				// Verify chunks respect chunk size limit
				for i, chunk := range actualChunks {
					if int64(len(chunk)) > tt.chunkSize {
						t.Errorf("Chunk %d exceeds size limit: %d > %d", i, len(chunk), tt.chunkSize)
					}
				}
			}

			log.Infof("Test %s: processed %d chunks", tt.name, len(actualChunks))
		})
	}
}

// Test line chunking configuration
func TestLinesConfiguration(t *testing.T) {
	t.Run("WithLines option", func(t *testing.T) {
		config := NewConfig(WithLines(10))
		if config.LineTrigger != 10 {
			t.Errorf("Expected LineTrigger 10, got %d", config.LineTrigger)
		}
	})

	t.Run("Lines with chunk size interaction", func(t *testing.T) {
		config := NewConfig(WithLines(5), WithChunkSize(100))
		if config.LineTrigger != 5 {
			t.Errorf("Expected LineTrigger 5, got %d", config.LineTrigger)
		}
		if config.ChunkSize != 100 {
			t.Errorf("Expected ChunkSize 100, got %d", config.ChunkSize)
		}
	})

	t.Run("Zero lines should disable line chunking", func(t *testing.T) {
		config := NewConfig(WithLines(0))
		if config.LineTrigger != 0 {
			t.Errorf("Expected LineTrigger 0, got %d", config.LineTrigger)
		}
	})
}

// Test line chunking error cases
func TestLinesChunkingErrors(t *testing.T) {
	t.Run("Negative lines value", func(t *testing.T) {
		// This should still work, but LineTrigger will be negative and likely ignored
		config := NewConfig(WithLines(-1))
		if config.LineTrigger != -1 {
			t.Errorf("Expected LineTrigger -1, got %d", config.LineTrigger)
		}
	})
}

// Test line chunking with different file types
func TestLinesChunkingFromFile(t *testing.T) {
	// Create a temporary file for testing
	testContent := `line 1
line 2  
line 3
line 4
line 5
line 6
line 7
line 8
line 9
line 10
line 11
line 12`

	tmpFile, err := os.CreateTemp("", "line_chunk_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(testContent); err != nil {
		tmpFile.Close()
		t.Fatal(err)
	}
	tmpFile.Close()

	var chunks []string
	var mu sync.Mutex

	reducer, err := NewReducerFromFile(tmpFile.Name(),
		WithLines(3), // 3 lines per chunk
		WithChunkSize(1024),
		WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			mu.Lock()
			chunks = append(chunks, string(chunk.Data()))
			mu.Unlock()
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}

	err = reducer.Run()
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	chunkCount := len(chunks)
	mu.Unlock()

	// Should have 4 chunks: 3 lines, 3 lines, 3 lines, 3 lines
	expectedChunks := 4
	if chunkCount != expectedChunks {
		t.Errorf("Expected %d chunks, got %d", expectedChunks, chunkCount)
	}

	// Verify chunks combined equal original content
	mu.Lock()
	combined := strings.Join(chunks, "")
	mu.Unlock()

	if combined != testContent {
		t.Errorf("Combined chunks don't match original content")
		t.Logf("Expected length: %d, got length: %d", len(testContent), len(combined))
		t.Logf("Expected: %q", testContent)
		t.Logf("Got: %q", combined)

		// Show chunk details
		for i, chunk := range chunks {
			t.Logf("Chunk %d: %q", i, chunk)
		}
	}

	log.Infof("File line chunking test: processed %d chunks", chunkCount)
}

// Test line chunking with large chunk size constraint
func TestLinesWithChunkSizeConstraint(t *testing.T) {
	// Create content where multiple lines exceed chunk size
	longLine := strings.Repeat("a", 100)         // 100 character line
	testData := strings.Repeat(longLine+"\n", 5) // 5 long lines, each about 101 bytes

	var chunks []string
	var mu sync.Mutex

	reducer, err := NewReducerFromString(testData,
		WithLines(3),       // 3 lines per chunk
		WithChunkSize(150), // Should force splitting since 3*101 > 150
		WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			mu.Lock()
			chunks = append(chunks, string(chunk.Data()))
			mu.Unlock()
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}

	err = reducer.Run()
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	actualChunks := make([]string, len(chunks))
	copy(actualChunks, chunks)
	mu.Unlock()

	// Should have more than 2 chunks due to size constraint
	if len(actualChunks) < 2 {
		t.Errorf("Expected at least 2 chunks due to size constraint, got %d", len(actualChunks))
	}

	// Verify no chunk exceeds size limit
	for i, chunk := range actualChunks {
		if int64(len(chunk)) > 150 {
			t.Errorf("Chunk %d exceeds size limit: %d > 150", i, len(chunk))
		}
	}

	// Verify chunks combined equal original data
	combined := strings.Join(actualChunks, "")
	if combined != testData {
		t.Errorf("Combined chunks don't match original data")
	}

	log.Infof("Chunk size constraint test: processed %d chunks", len(actualChunks))
}

// Test comprehensive WithLines usage example
func TestWithLinesExample(t *testing.T) {
	// 创建一个具有代表性的文本示例
	exampleText := `Line 1: Introduction to the document
Line 2: This is the second line with some content
Line 3: Third line contains important information
Line 4: Fourth line has additional details
Line 5: Fifth line continues the narrative
Line 6: Sixth line provides more context
Line 7: Seventh line expands on the topic
Line 8: Eighth line offers further insights
Line 9: Ninth line concludes the section
Line 10: Final line wraps up everything`

	t.Run("WithLines(3) demonstration", func(t *testing.T) {
		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(exampleText,
			WithLines(3),        // 每3行创建一个chunk
			WithChunkSize(1024), // 足够大的chunk size，不会触发分割
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()

				// Log chunk details for documentation
				lineCount := strings.Count(string(chunk.Data()), "\n")
				if !strings.HasSuffix(string(chunk.Data()), "\n") {
					lineCount++
				}
				log.Infof("Processed chunk with %d lines, %d bytes", lineCount, len(chunk.Data()))
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		// 应该有4个chunks: 3+3+3+1行
		expectedChunks := 4
		mu.Lock()
		actualChunks := len(chunks)
		mu.Unlock()

		if actualChunks != expectedChunks {
			t.Errorf("Expected %d chunks, got %d", expectedChunks, actualChunks)
		}

		// 验证所有数据都被正确重组
		mu.Lock()
		combined := strings.Join(chunks, "")
		mu.Unlock()

		if combined != exampleText {
			t.Error("Combined chunks don't match original text")
		}

		log.Infof("WithLines(3) example completed successfully with %d chunks", actualChunks)
	})

	t.Run("WithLines(3) with ChunkSize constraint", func(t *testing.T) {
		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(exampleText,
			WithLines(3),       // 每3行创建一个chunk
			WithChunkSize(100), // 小的chunk size会强制分割
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()

				log.Infof("Chunk size constraint: %d bytes", len(chunk.Data()))
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := len(chunks)

		// 验证chunk size限制
		for i, chunk := range chunks {
			if int64(len(chunk)) > 100 {
				t.Errorf("Chunk %d exceeds size limit: %d > 100", i, len(chunk))
			}
		}

		// 验证数据完整性
		combined := strings.Join(chunks, "")
		mu.Unlock()

		if combined != exampleText {
			t.Error("Combined chunks don't match original text")
		}

		log.Infof("WithLines + ChunkSize constraint completed with %d chunks", actualChunks)
	})
}

// Test chunkSize as hard constraint override all other options
func TestChunkSizeHardConstraint(t *testing.T) {
	// 创建一个长文本来测试各种配置组合
	longLine := strings.Repeat("This is a very long line content that will exceed chunk size limits ", 10)
	testData := longLine + "\n" + longLine + "\n" + longLine + "\n"
	smallChunkSize := int64(50) // 故意设置很小的chunk size

	t.Run("ChunkSize overrides Lines configuration", func(t *testing.T) {
		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithLines(3), // 3行per chunk，但由于行很长，会被ChunkSize覆盖
			WithChunkSize(smallChunkSize),
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		defer mu.Unlock()

		// 验证所有chunks都遵守ChunkSize限制
		for i, chunk := range chunks {
			if int64(len(chunk)) > smallChunkSize {
				t.Errorf("Chunk %d exceeds ChunkSize limit: %d > %d", i, len(chunk), smallChunkSize)
			}
		}

		// 应该有多个chunks因为ChunkSize限制
		if len(chunks) < 5 {
			t.Errorf("Expected multiple chunks due to ChunkSize constraint, got %d", len(chunks))
		}

		log.Infof("ChunkSize overrides Lines: created %d chunks", len(chunks))
	})

	t.Run("ChunkSize overrides Separator configuration", func(t *testing.T) {
		separatorData := "short|medium content|" + strings.Repeat("very long content that exceeds chunk size", 5) + "|end"

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(separatorData,
			WithSeparatorTrigger("|"),     // 分隔符触发
			WithChunkSize(smallChunkSize), // 小的ChunkSize
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		defer mu.Unlock()

		// 验证ChunkSize硬限制
		for i, chunk := range chunks {
			if int64(len(chunk)) > smallChunkSize {
				t.Errorf("Chunk %d exceeds ChunkSize limit: %d > %d", i, len(chunk), smallChunkSize)
			}
		}

		log.Infof("ChunkSize overrides Separator: created %d chunks", len(chunks))
	})

	t.Run("ChunkSize with TimeTrigger combination", func(t *testing.T) {
		longData := strings.Repeat("A", int(smallChunkSize)*3) // 3倍的chunk size长度

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(longData,
			WithTimeTriggerInterval(100*time.Millisecond), // 时间触发
			WithChunkSize(smallChunkSize),                 // 小的ChunkSize
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		defer mu.Unlock()

		// 验证ChunkSize硬限制
		for i, chunk := range chunks {
			if int64(len(chunk)) > smallChunkSize {
				t.Errorf("Chunk %d exceeds ChunkSize limit: %d > %d", i, len(chunk), smallChunkSize)
			}
		}

		// 应该至少有3个chunks
		if len(chunks) < 3 {
			t.Errorf("Expected at least 3 chunks, got %d", len(chunks))
		}

		log.Infof("ChunkSize with TimeTrigger: created %d chunks", len(chunks))
	})
}

// Test NewReducerFromInputChunk function
func TestNewReducerFromInputChunk(t *testing.T) {
	// 创建输入chunk channel
	inputChan := chanx.NewUnlimitedChan[chunkmaker.Chunk](context.Background(), 10)

	// 模拟一些chunk数据
	go func() {
		defer inputChan.Close()

		inputChan.SafeFeed(chunkmaker.NewBufferChunk([]byte("chunk 1 data")))
		inputChan.SafeFeed(chunkmaker.NewBufferChunk([]byte("chunk 2 data")))
		inputChan.SafeFeed(chunkmaker.NewBufferChunk([]byte("chunk 3 data")))
	}()

	var processedChunks []string
	var mu sync.Mutex

	reducer, err := NewReducerFromInputChunk(inputChan,
		WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			mu.Lock()
			processedChunks = append(processedChunks, string(chunk.Data()))
			mu.Unlock()
			return nil
		}))

	if err != nil {
		t.Fatal(err)
	}

	err = reducer.Run()
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	chunkCount := len(processedChunks)
	mu.Unlock()

	if chunkCount == 0 {
		t.Error("Expected to process chunks from input channel")
	}

	log.Infof("NewReducerFromInputChunk processed %d chunks", chunkCount)
}

// Test NewReducerFromInputChunk error cases
func TestNewReducerFromInputChunkErrors(t *testing.T) {
	t.Run("Nil input channel", func(t *testing.T) {
		_, err := NewReducerFromInputChunk(nil,
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				return nil
			}))

		if err == nil {
			t.Error("Expected error for nil input channel")
		}
		if !strings.Contains(err.Error(), "failed to create chunk channel") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})
}

// Test LineChunkMaker error cases and edge scenarios
func TestLineChunkMakerEdgeCases(t *testing.T) {
	t.Run("Invalid LineTrigger value", func(t *testing.T) {
		config := NewConfig(WithLines(-1), WithChunkSize(100))
		_, err := NewLineChunkMaker(strings.NewReader("test"), config)

		if err == nil {
			t.Error("Expected error for negative LineTrigger")
		}
		if !strings.Contains(err.Error(), "LineTrigger must be positive") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Invalid ChunkSize value", func(t *testing.T) {
		// 直接创建config绕过NewConfig的默认值设置
		config := &Config{
			LineTrigger: 1,
			ChunkSize:   -1,
		}
		_, err := NewLineChunkMaker(strings.NewReader("test"), config)

		if err == nil {
			t.Error("Expected error for negative ChunkSize")
		}
		if !strings.Contains(err.Error(), "ChunkSize must be positive") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Reader with IO error", func(t *testing.T) {
		// 创建一个会产生读取错误的Reader
		errorReader := &erroringReader{}

		config := NewConfig(WithLines(1), WithChunkSize(100))
		lm, err := NewLineChunkMaker(errorReader, config)
		if err != nil {
			t.Fatal(err)
		}

		// 尝试从输出通道读取，应该会关闭但不会panic
		ch := lm.OutputChannel()
		select {
		case _, ok := <-ch:
			if ok {
				t.Error("Expected channel to be closed due to read error")
			}
		case <-time.After(100 * time.Millisecond):
			// 超时也是可以接受的
		}

		// 清理
		lm.Close()
	})

	t.Run("Context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		config := NewConfig(WithLines(1), WithChunkSize(100), WithContext(ctx))

		longData := strings.Repeat("line content\n", 1000)
		lm, err := NewLineChunkMaker(strings.NewReader(longData), config)
		if err != nil {
			t.Fatal(err)
		}

		// 立即取消context
		cancel()

		// 应该能正常关闭
		lm.Close()

		log.Info("Context cancellation test completed")
	})
}

// Helper struct for testing IO errors
type erroringReader struct{}

func (er *erroringReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

// Test Close method coverage
func TestLineChunkMakerClose(t *testing.T) {
	config := NewConfig(WithLines(1), WithChunkSize(100))
	lm, err := NewLineChunkMaker(strings.NewReader("test data"), config)
	if err != nil {
		t.Fatal(err)
	}

	// 测试Close方法
	err = lm.Close()
	if err != nil {
		t.Errorf("Expected Close to return nil, got: %v", err)
	}

	// 多次调用Close应该安全
	err = lm.Close()
	if err != nil {
		t.Errorf("Multiple Close calls should be safe, got: %v", err)
	}

	log.Info("Close method test completed")
}

// Test splitAndEmitChunk method via integration test
func TestSplitAndEmitChunk(t *testing.T) {
	// 测试splitAndEmitChunk功能通过集成测试
	// 创建一个包含长行的数据，这会触发splitAndEmitChunk
	longLine := strings.Repeat("This is a very long line that exceeds chunk size ", 10)
	testData := longLine + "\n" + "short line"

	var chunks []string
	var mu sync.Mutex

	config := NewConfig(WithLines(1), WithChunkSize(50))
	lm, err := NewLineChunkMaker(strings.NewReader(testData), config)
	if err != nil {
		t.Fatal(err)
	}
	defer lm.Close()

	// 从输出通道读取chunks
	timeout := time.After(500 * time.Millisecond)

	for {
		select {
		case chunk, ok := <-lm.OutputChannel():
			if !ok {
				goto done
			}
			mu.Lock()
			chunks = append(chunks, string(chunk.Data()))
			mu.Unlock()
		case <-timeout:
			goto done
		}
	}

done:
	mu.Lock()
	actualChunks := make([]string, len(chunks))
	copy(actualChunks, chunks)
	mu.Unlock()

	if len(actualChunks) == 0 {
		t.Error("Expected splitAndEmitChunk to produce chunks")
	}

	// 验证chunks不超过指定大小
	for i, chunk := range actualChunks {
		if int64(len(chunk)) > 50 {
			t.Errorf("Chunk %d exceeds size limit: %d > 50", i, len(chunk))
		}
	}

	// 验证数据完整性
	combined := strings.Join(actualChunks, "")
	if combined != testData {
		t.Error("Combined chunks don't match original data")
	}

	log.Infof("splitAndEmitChunk test: produced %d chunks", len(actualChunks))
}

// Test Run method edge cases
func TestRunEdgeCases(t *testing.T) {
	t.Run("Run without callback (default behavior)", func(t *testing.T) {
		// 创建没有callback的reducer来测试默认行为
		reducer := &Reducer{
			config: NewConfig(),
			input:  createMockChunkMaker([]string{"test chunk"}),
		}

		// 应该执行默认的spew.Dump行为而不报错
		err := reducer.Run()
		if err != nil {
			t.Errorf("Run without callback should work with default behavior, got: %v", err)
		}

		log.Info("Run without callback test completed")
	})

	t.Run("Run with nil memory", func(t *testing.T) {
		var chunkReceived bool

		reducer := &Reducer{
			config: &Config{
				Memory: nil, // 故意设置为nil
				callback: func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
					chunkReceived = true
					if memory == nil {
						t.Error("Memory should be initialized automatically")
					}
					return nil
				},
			},
			input: createMockChunkMaker([]string{"test chunk"}),
		}

		err := reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		if !chunkReceived {
			t.Error("Chunk should have been received")
		}

		log.Info("Run with nil memory test completed")
	})
}

// Mock ChunkMaker for testing
type mockChunkMaker struct {
	chunks []string
	ch     chan chunkmaker.Chunk
	closed bool
}

func createMockChunkMaker(chunks []string) *mockChunkMaker {
	ch := make(chan chunkmaker.Chunk, len(chunks)+1)

	// 发送所有chunks
	for _, chunkData := range chunks {
		ch <- chunkmaker.NewBufferChunk([]byte(chunkData))
	}
	close(ch)

	return &mockChunkMaker{
		chunks: chunks,
		ch:     ch,
		closed: true,
	}
}

func (m *mockChunkMaker) Close() error {
	if !m.closed {
		close(m.ch)
		m.closed = true
	}
	return nil
}

func (m *mockChunkMaker) OutputChannel() <-chan chunkmaker.Chunk {
	return m.ch
}

// Test complex chunkSize constraint scenarios
func TestComplexChunkSizeConstraints(t *testing.T) {
	t.Run("Multiple configuration options with ChunkSize override", func(t *testing.T) {
		// 创建复杂的测试数据
		complexData := `Line 1 with some content
Line 2 with different content  
Line 3 has more information here
Line 4 contains additional data points
Line 5 continues the narrative flow
Line 6 provides contextual information
Line 7 expands on previous topics
Line 8 offers new perspectives
Line 9 concludes this section
Line 10 wraps everything up nicely`

		var chunks []string
		var mu sync.Mutex

		// 使用非常小的chunk size来强制分割
		reducer, err := NewReducerFromString(complexData,
			WithLines(5),                                 // 5行per chunk
			WithSeparatorTrigger("\n"),                   // 换行符触发
			WithTimeTriggerInterval(50*time.Millisecond), // 时间触发
			WithChunkSize(40),                            // 很小的chunk size，应该覆盖所有其他选项
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]string, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		// 验证ChunkSize硬约束
		for i, chunk := range actualChunks {
			if int64(len(chunk)) > 40 {
				t.Errorf("Chunk %d exceeds ChunkSize constraint: %d > 40", i, len(chunk))
				t.Logf("Chunk content: %q", chunk)
			}
		}

		// 应该有很多chunks由于size限制
		if len(actualChunks) < 8 {
			t.Errorf("Expected many chunks due to small ChunkSize, got %d", len(actualChunks))
		}

		// 验证数据完整性
		combined := strings.Join(actualChunks, "")
		if combined != complexData {
			t.Error("Combined chunks don't match original data")
		}

		log.Infof("Complex ChunkSize constraints: created %d chunks with size limit 40", len(actualChunks))
	})

	t.Run("ChunkSize constraint with binary-like data", func(t *testing.T) {
		// 创建包含特殊字符的数据
		binaryLikeData := strings.Repeat("ABC\x00\x01\x02DEF\n", 20)

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(binaryLikeData,
			WithLines(3),
			WithChunkSize(25), // 小chunk size
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]string, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		// 验证ChunkSize约束
		for i, chunk := range actualChunks {
			if int64(len(chunk)) > 25 {
				t.Errorf("Chunk %d exceeds ChunkSize: %d > 25", i, len(chunk))
			}
		}

		// 验证数据完整性
		combined := strings.Join(actualChunks, "")
		if combined != binaryLikeData {
			t.Error("Combined chunks don't match original binary-like data")
		}

		log.Infof("Binary-like data ChunkSize test: created %d chunks", len(actualChunks))
	})
}

// Test EnableLineNumber functionality
func TestEnableLineNumber(t *testing.T) {
	t.Run("Basic line number prefixing", func(t *testing.T) {
		testData := "line1\nline2\nline3"

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithEnableLineNumber(true),
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]string, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		if len(actualChunks) == 0 {
			t.Error("Expected at least one chunk")
			return
		}

		// 验证全局行号连续性
		combined := strings.Join(actualChunks, "")
		expectedLines := []string{"1 | line1", "2 | line2", "3 | line3"}
		for _, expectedLine := range expectedLines {
			if !strings.Contains(combined, expectedLine) {
				t.Errorf("Expected global line number %q in output, got: %q", expectedLine, combined)
			}
		}

		log.Infof("Line number prefixing test: processed %d chunks", len(actualChunks))
	})

	t.Run("Line numbers with ChunkSize constraint", func(t *testing.T) {
		testData := "line1\nline2\nline3\nline4\nline5"

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithEnableLineNumber(true),
			WithChunkSize(20), // 很小的chunk size，会强制分割
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]string, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		// 验证ChunkSize约束
		for i, chunk := range actualChunks {
			if int64(len(chunk)) > 20 {
				t.Errorf("Chunk %d exceeds ChunkSize: %d > 20", i, len(chunk))
			}
		}

		// 验证全局行号连续性，即使在多个chunks中也应该保持连续
		combined := strings.Join(actualChunks, "")
		expectedLines := []string{"1 | line1", "2 | line2", "3 | line3", "4 | line4", "5 | line5"}
		for _, expectedLine := range expectedLines {
			if !strings.Contains(combined, expectedLine) {
				t.Errorf("Expected global line number %q in combined output, got: %q", expectedLine, combined)
			}
		}

		log.Infof("Line numbers with ChunkSize constraint: created %d chunks", len(actualChunks))
	})

	t.Run("Line numbers with Lines trigger", func(t *testing.T) {
		testData := "line1\nline2\nline3\nline4\nline5\nline6"

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithEnableLineNumber(true),
			WithLines(2), // 2行per chunk
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]string, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		// 应该有3个chunks (6行 / 2行per chunk)
		if len(actualChunks) != 3 {
			t.Errorf("Expected 3 chunks, got %d", len(actualChunks))
		}

		// 验证全局行号连续性跨越chunks
		combined := strings.Join(actualChunks, "")
		expectedLines := []string{"1 | line1", "2 | line2", "3 | line3", "4 | line4", "5 | line5", "6 | line6"}
		for _, expectedLine := range expectedLines {
			if !strings.Contains(combined, expectedLine) {
				t.Errorf("Expected global line number %q across chunks, got: %q", expectedLine, combined)
			}
		}

		// 验证行号不是从每个chunk重新开始
		if strings.Count(combined, "1 | ") > 1 {
			t.Errorf("Line number '1 |' appears multiple times, indicating non-global numbering")
		}

		log.Infof("Line numbers with Lines trigger: created %d chunks", len(actualChunks))
	})

	t.Run("Line numbers with Lines and ChunkSize constraints", func(t *testing.T) {
		// 创建很长的行来测试ChunkSize优先级
		longLine := strings.Repeat("This is a very long line that will exceed chunk size when numbered ", 3)
		testData := longLine + "\n" + longLine + "\n" + "short line"

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithEnableLineNumber(true),
			WithLines(2),      // 2行per chunk
			WithChunkSize(50), // 小的chunk size，应该会分割长行
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]string, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		// 验证ChunkSize硬约束
		for i, chunk := range actualChunks {
			if int64(len(chunk)) > 50 {
				t.Errorf("Chunk %d exceeds ChunkSize: %d > 50", i, len(chunk))
			}
		}

		// 验证有行号
		combined := strings.Join(actualChunks, "")
		if !strings.Contains(combined, "1 | ") {
			t.Errorf("Expected line numbers in output")
		}

		// 由于ChunkSize限制，应该有多个chunks
		if len(actualChunks) < 3 {
			t.Errorf("Expected multiple chunks due to ChunkSize constraint, got %d", len(actualChunks))
		}

		log.Infof("Line numbers with Lines and ChunkSize: created %d chunks", len(actualChunks))
	})

	t.Run("Line numbers with file input", func(t *testing.T) {
		// 创建临时文件
		tmpFile, err := os.CreateTemp("", "test_line_numbers_*.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())

		testContent := "first line\nsecond line\nthird line\nfourth line"
		_, err = tmpFile.WriteString(testContent)
		if err != nil {
			t.Fatal(err)
		}
		tmpFile.Close()

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromFile(tmpFile.Name(),
			WithEnableLineNumber(true),
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]string, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		if len(actualChunks) == 0 {
			t.Error("Expected at least one chunk")
			return
		}

		// 验证行号
		combined := strings.Join(actualChunks, "")
		expectedLines := []string{"1 | first line", "2 | second line", "3 | third line", "4 | fourth line"}
		for _, expectedLine := range expectedLines {
			if !strings.Contains(combined, expectedLine) {
				t.Errorf("Expected %q in output, got: %q", expectedLine, combined)
			}
		}

		log.Infof("File line numbers test: processed %d chunks", len(actualChunks))
	})

	t.Run("Global line numbering consistency across different chunking methods", func(t *testing.T) {
		testContent := "Line One\nLine Two\nLine Three\nLine Four\nLine Five\nLine Six\nLine Seven\nLine Eight\n"

		// Test with different chunking methods to ensure global line numbering is consistent
		tests := []struct {
			name string
			opts []Option
		}{
			{
				name: "Standard chunking with small size",
				opts: []Option{WithEnableLineNumber(true), WithChunkSize(25)},
			},
			{
				name: "Line-based chunking",
				opts: []Option{WithEnableLineNumber(true), WithLines(2)},
			},
			{
				name: "Mixed line and size constraints",
				opts: []Option{WithEnableLineNumber(true), WithLines(3), WithChunkSize(30)},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var chunks []string
				var mu sync.Mutex

				reducer, err := NewReducerFromString(testContent, append(tt.opts,
					WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
						mu.Lock()
						chunks = append(chunks, string(chunk.Data()))
						mu.Unlock()
						return nil
					}))...)

				if err != nil {
					t.Fatal(err)
				}

				err = reducer.Run()
				if err != nil {
					t.Fatal(err)
				}

				mu.Lock()
				combined := strings.Join(chunks, "")
				chunkCount := len(chunks)
				mu.Unlock()

				// 验证所有8行都有正确的全局行号
				expectedLines := []string{
					"1 | Line One", "2 | Line Two", "3 | Line Three", "4 | Line Four",
					"5 | Line Five", "6 | Line Six", "7 | Line Seven", "8 | Line Eight",
				}
				for _, expectedLine := range expectedLines {
					if !strings.Contains(combined, expectedLine) {
						t.Errorf("Missing expected global line number %q in %s", expectedLine, tt.name)
					}
				}

				// 验证行号的唯一性（每个行号只出现一次）
				for i := 1; i <= 8; i++ {
					lineMarker := fmt.Sprintf("%d | ", i)
					count := strings.Count(combined, lineMarker)
					if count != 1 {
						t.Errorf("Line number %d appears %d times in %s, expected exactly 1", i, count, tt.name)
					}
				}

				log.Infof("%s: processed %d chunks with global line numbering", tt.name, chunkCount)
			})
		}
	})

	t.Run("Disable line numbers (default behavior)", func(t *testing.T) {
		testData := "line1\nline2\nline3"

		var chunks []string
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithEnableLineNumber(false), // 显式禁用
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, string(chunk.Data()))
				mu.Unlock()
				return nil
			}))

		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]string, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		if len(actualChunks) == 0 {
			t.Error("Expected at least one chunk")
			return
		}

		// 验证没有行号
		combined := strings.Join(actualChunks, "")
		if strings.Contains(combined, " | ") {
			t.Errorf("Expected no line numbers, but found them in: %q", combined)
		}

		// 应该包含原始内容
		if combined != testData {
			t.Errorf("Expected original content %q, got %q", testData, combined)
		}

		log.Infof("Disabled line numbers test: processed %d chunks", len(actualChunks))
	})
}

// Test DumpWithOverlap functionality when chunks have overlap information
func TestDumpWithOverlap(t *testing.T) {
	testData := strings.Repeat("This is a test line that will be split across chunks.\n", 20)

	t.Run("DumpWithOverlap with small chunk size", func(t *testing.T) {
		var chunks []chunkmaker.Chunk
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithChunkSize(100), // Small chunk size to force splitting
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, chunk)
				mu.Unlock()
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]chunkmaker.Chunk, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		if len(actualChunks) < 2 {
			t.Errorf("Expected at least 2 chunks for overlap testing, got %d", len(actualChunks))
			return
		}

		log.Infof("Testing DumpWithOverlap with %d chunks", len(actualChunks))

		// Test DumpWithOverlap functionality
		for i, chunk := range actualChunks {
			if i == 0 {
				// First chunk should not have overlap
				normalDump := chunk.Dump()
				overlapDump := chunk.DumpWithOverlap(50)
				if normalDump != overlapDump {
					t.Errorf("First chunk should have same output for Dump() and DumpWithOverlap(), got different")
				}
				log.Infof("Chunk %d (first): no overlap as expected", i)
			} else {
				// Subsequent chunks should have overlap if requested
				if chunk.HaveLastChunk() {
					overlapDump := chunk.DumpWithOverlap(50)
					if !strings.Contains(overlapDump, "<|OVERLAP[") {
						t.Errorf("Chunk %d should contain overlap markers when DumpWithOverlap(50) is called", i)
					}
					if !strings.Contains(overlapDump, "]|>") {
						t.Errorf("Chunk %d should contain overlap end markers when DumpWithOverlap(50) is called", i)
					}
					if !strings.Contains(overlapDump, "<|OVERLAP_END[") {
						t.Errorf("Chunk %d should contain overlap end markers when DumpWithOverlap(50) is called", i)
					}
					log.Infof("Chunk %d: contains overlap markers as expected", i)
				} else {
					t.Errorf("Chunk %d should have previous chunk reference", i)
				}
			}
		}
	})

	t.Run("DumpWithOverlap with zero overlap request", func(t *testing.T) {
		var chunks []chunkmaker.Chunk
		var mu sync.Mutex

		reducer, err := NewReducerFromString(testData,
			WithChunkSize(100),
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, chunk)
				mu.Unlock()
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]chunkmaker.Chunk, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		// When overlap is 0, DumpWithOverlap should behave like Dump
		for i, chunk := range actualChunks {
			normalDump := chunk.Dump()
			zeroOverlapDump := chunk.DumpWithOverlap(0)
			if normalDump != zeroOverlapDump {
				t.Errorf("Chunk %d: DumpWithOverlap(0) should equal Dump(), but they differ", i)
			}
		}

		log.Infof("Zero overlap test: all chunks behave correctly")
	})

	t.Run("DumpWithOverlap with line numbering", func(t *testing.T) {
		var chunks []chunkmaker.Chunk
		var mu sync.Mutex

		// Test with line numbers enabled to ensure overlap works with numbered content
		reducer, err := NewReducerFromString("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\n",
			WithEnableLineNumber(true),
			WithChunkSize(30), // Small enough to split numbered lines
			WithReducerCallback(func(config *Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
				mu.Lock()
				chunks = append(chunks, chunk)
				mu.Unlock()
				return nil
			}))
		if err != nil {
			t.Fatal(err)
		}

		err = reducer.Run()
		if err != nil {
			t.Fatal(err)
		}

		mu.Lock()
		actualChunks := make([]chunkmaker.Chunk, len(chunks))
		copy(actualChunks, chunks)
		mu.Unlock()

		if len(actualChunks) < 2 {
			t.Errorf("Expected at least 2 chunks for line number overlap testing, got %d", len(actualChunks))
			return
		}

		// Verify overlap contains line numbers
		for i, chunk := range actualChunks {
			if i > 0 && chunk.HaveLastChunk() {
				overlapDump := chunk.DumpWithOverlap(20)
				if !strings.Contains(overlapDump, " | ") {
					t.Errorf("Chunk %d overlap should contain line numbers (format 'N | ')", i)
				}
				log.Infof("Chunk %d: overlap contains line numbers correctly", i)
			}
		}
	})
}

// Test configuration option for line numbers
func TestWithEnableLineNumberOption(t *testing.T) {
	t.Run("WithEnableLineNumber true", func(t *testing.T) {
		config := NewConfig(WithEnableLineNumber(true))
		if !config.EnableLineNumber {
			t.Error("Expected EnableLineNumber to be true")
		}
	})

	t.Run("WithEnableLineNumber false", func(t *testing.T) {
		config := NewConfig(WithEnableLineNumber(false))
		if config.EnableLineNumber {
			t.Error("Expected EnableLineNumber to be false")
		}
	})

	t.Run("Default EnableLineNumber value", func(t *testing.T) {
		config := NewConfig()
		if config.EnableLineNumber {
			t.Error("Expected default EnableLineNumber to be false")
		}
	})
}
