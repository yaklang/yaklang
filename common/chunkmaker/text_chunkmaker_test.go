package chunkmaker

import (
	"testing"

	"unicode/utf8"

	"bytes"

	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func TestChunkerWithChunkSize_Basic(t *testing.T) {
	// 定义测试用例
	testCases := []struct {
		name           string
		chunkSize      int64
		inputData      string
		expectedChunks []string
	}{
		{
			name:      "small chunk size",
			chunkSize: 5,
			inputData: "HelloWorld",
			expectedChunks: []string{
				"Hello",
				"World",
			},
		},
		{
			name:      "medium chunk size",
			chunkSize: 10,
			inputData: "HelloWorldHelloWorld",
			expectedChunks: []string{
				"HelloWorld",
				"HelloWorld",
			},
		},
		{
			name:      "large chunk size",
			chunkSize: 20,
			inputData: "HelloWorldHelloWorld",
			expectedChunks: []string{
				"HelloWorldHelloWorld",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建管道
			pr, pw := utils.NewPipe()

			// 写入测试数据
			go func() {
				pw.Write([]byte(tc.inputData))
				pw.Close()
			}()

			// 创建 ChunkMaker
			cm, err := NewTextChunkMaker(pr, WithChunkSize(tc.chunkSize))
			if err != nil {
				t.Fatalf("Failed to create ChunkMaker: %v", err)
			}

			// 收集输出
			var chunks []string
			for chunk := range cm.OutputChannel() {
				chunks = append(chunks, string(chunk.Data()))
			}
			// 验证结果
			if len(chunks) != len(tc.expectedChunks) {
				t.Errorf("Expected %d chunks, got %d", len(tc.expectedChunks), len(chunks))
			}

			for i, chunk := range chunks {
				if i >= len(tc.expectedChunks) {
					break
				}
				if chunk != tc.expectedChunks[i] {
					t.Errorf("Chunk %d: expected %q, got %q", i, tc.expectedChunks[i], chunk)
				}
			}
		})
	}
}

func TestChunkerWithChunkSize_Advanced(t *testing.T) {
	// 定义测试用例
	testCases := []struct {
		name           string
		chunkSize      int64
		inputData      string
		expectedChunks []string
		expectError    bool
	}{
		{
			name:           "error on chunk size zero",
			chunkSize:      0,
			inputData:      "abc",
			expectedChunks: nil,
			expectError:    true,
		},
		{
			name:           "error on chunk size negative",
			chunkSize:      -5,
			inputData:      "abc",
			expectedChunks: nil,
			expectError:    true,
		},
		{
			name:           "empty input data",
			chunkSize:      5,
			inputData:      "",
			expectedChunks: []string{},
			expectError:    false,
		},
		{
			name:           "input smaller than chunkSize",
			chunkSize:      10,
			inputData:      "Hello", // 5 runes
			expectedChunks: []string{"Hello"},
			expectError:    false,
		},
		{
			name:           "chunkSize is 1 (ASCII)",
			chunkSize:      1,
			inputData:      "abc",
			expectedChunks: []string{"a", "b", "c"},
			expectError:    false,
		},
		{
			name:           "UTF-8 multi-byte characters, standard chunking",
			chunkSize:      2,      // runes
			inputData:      "你好世界", // 4 runes
			expectedChunks: []string{"你好", "世界"},
			expectError:    false,
		},
		{
			name:           "UTF-8 multi-byte characters, chunkSize 1",
			chunkSize:      1,    // rune
			inputData:      "你好", // 2 runes
			expectedChunks: []string{"你", "好"},
			expectError:    false,
		},
		{
			name:           "UTF-8 multi-byte characters, input smaller than chunkSize",
			chunkSize:      3,    // runes
			inputData:      "你好", // 2 runes
			expectedChunks: []string{"你好"},
			expectError:    false,
		},
		{
			name:      "Non-UTF-8 bytes",
			chunkSize: 2,                                                  // This will be interpreted as bytes due to invalid UTF-8 sequence
			inputData: string([]byte{0xff, 0xfe, 0xfd, 0xfc, 0xfa, 0xf9}), // 6 bytes
			expectedChunks: []string{
				string([]byte{0xff, 0xfe}),
				string([]byte{0xfd, 0xfc}),
				string([]byte{0xfa, 0xf9}),
			},
			expectError: false,
		},
		{
			name:      "Non-UTF-8 bytes, input smaller than chunkSize",
			chunkSize: 4,                                // bytes
			inputData: string([]byte{0xff, 0xfe, 0xfd}), // 3 bytes
			expectedChunks: []string{
				string([]byte{0xff, 0xfe, 0xfd}),
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建管道
			pr, pw := utils.NewPipe()

			// 写入测试数据
			go func() {
				_, errWrite := pw.Write([]byte(tc.inputData))
				if errWrite != nil {
					// 在测试 goroutine 中报告错误，如果写入失败
					t.Errorf("Failed to write input data for test case %s: %v", tc.name, errWrite)
				}
				pw.Close()
			}()

			// 创建 ChunkMaker
			cm, err := NewTextChunkMaker(pr, WithChunkSize(tc.chunkSize))

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected an error for chunkSize %d, but got nil", tc.chunkSize)
				}
				// 如果预期有错误且确实收到了错误，则测试通过，可以提前返回或继续（取决于是否还要检查 cm 的状态）
				return
			}

			if err != nil {
				t.Fatalf("Failed to create ChunkMaker for chunkSize %d: %v", tc.chunkSize, err)
			}

			// 收集输出
			var chunks []string
			outputChan := cm.OutputChannel()
			for chunk := range outputChan {
				chunks = append(chunks, string(chunk.Data()))
			}
			// 验证结果
			if len(chunks) != len(tc.expectedChunks) {
				t.Errorf("Expected %d chunks, got %d. Chunks: %v", len(tc.expectedChunks), len(chunks), chunks)
			}

			for i, chunkStr := range chunks {
				if i >= len(tc.expectedChunks) {
					// 如果实际 chunks 比预期多，上面的长度检查会失败。这里防止越界。
					break
				}
				if chunkStr != tc.expectedChunks[i] {
					// 对于非 UTF-8 数据，使用 %x 或 []byte 格式化可能更清晰
					if !utf8.ValidString(tc.expectedChunks[i]) || !utf8.ValidString(chunkStr) {
						t.Errorf("Chunk %d: expected %x, got %x", i, []byte(tc.expectedChunks[i]), []byte(chunkStr))
					} else {
						t.Errorf("Chunk %d: expected %q, got %q", i, tc.expectedChunks[i], chunkStr)
					}
				}
			}
			// 检查预期的 chunks 是否都已收到
			if len(tc.expectedChunks) > len(chunks) {
				t.Errorf("Missing expected chunks. Expected: %v, Got: %v", tc.expectedChunks, chunks)
			}
		})
	}
}

func TestChunkMaker_ChunkLinkingAndPrevNBytes(t *testing.T) {
	testCases := []struct {
		name               string
		chunkSize          int64
		inputData          string
		expectedChunkDatas []string // Data content of each chunk
		prevNChecks        []struct {
			chunkIndex int // Index of the chunk in expectedChunkDatas to call PrevNBytes on
			n          int
			expected   []byte
		}
	}{
		{
			name:               "Simple linking and PrevNBytes",
			chunkSize:          5,
			inputData:          "HelloWorldThisIsFun", // Chunks: Hello, World, ThisI, sFun
			expectedChunkDatas: []string{"Hello", "World", "ThisI", "sFun"},
			prevNChecks: []struct {
				chunkIndex int
				n          int
				expected   []byte
			}{
				// On chunk "Hello" (index 0), prev is nil
				{chunkIndex: 0, n: 3, expected: []byte("")},
				// On chunk "World" (index 1), prev is "Hello"
				{chunkIndex: 1, n: 3, expected: []byte("llo")},   // Last 3 of "Hello"
				{chunkIndex: 1, n: 5, expected: []byte("Hello")}, // All of "Hello"
				{chunkIndex: 1, n: 7, expected: []byte("Hello")}, // All of "Hello" (n > prev length)
				// On chunk "ThisI" (index 2), prev is "World"
				{chunkIndex: 2, n: 3, expected: []byte("rld")},      // Last 3 of "World"
				{chunkIndex: 2, n: 8, expected: []byte("lloWorld")}, // "llo" (from Hello) + "World"
				// On chunk "sFun" (index 3), prev is "ThisI"
				{chunkIndex: 3, n: 2, expected: []byte("sI")},               // Last 2 of "ThisI"
				{chunkIndex: 3, n: 10, expected: []byte("WorldThisI")},      // "World" + "ThisI"
				{chunkIndex: 3, n: 15, expected: []byte("HelloWorldThisI")}, // "Hello" + "World" + "ThisI"
				{chunkIndex: 3, n: 20, expected: []byte("HelloWorldThisI")}, // All prev data
			},
		},
		{
			name:               "Input smaller than chunk size, single chunk",
			chunkSize:          10,
			inputData:          "Tiny",
			expectedChunkDatas: []string{"Tiny"},
			prevNChecks: []struct {
				chunkIndex int
				n          int
				expected   []byte
			}{
				// On chunk "Tiny" (index 0), prev is nil
				{chunkIndex: 0, n: 2, expected: []byte("")},
				{chunkIndex: 0, n: 4, expected: []byte("")},
			},
		},
		{
			name:               "Empty input",
			chunkSize:          5,
			inputData:          "",
			expectedChunkDatas: []string{},
			prevNChecks: []struct {
				chunkIndex int
				n          int
				expected   []byte
			}{}, // No checks
		},
		{
			name:               "Multiple small writes, exact multiples of chunksize",
			chunkSize:          3,
			inputData:          "abcdefghi", // 3 chunks: abc, def, ghi
			expectedChunkDatas: []string{"abc", "def", "ghi"},
			prevNChecks: []struct {
				chunkIndex int
				n          int
				expected   []byte
			}{
				{chunkIndex: 0, n: 2, expected: []byte("")},
				{chunkIndex: 1, n: 2, expected: []byte("bc")},      // prev: "abc"
				{chunkIndex: 2, n: 2, expected: []byte("ef")},      // prev: "def"
				{chunkIndex: 2, n: 5, expected: []byte("bcdef")},   // prevs: "abc" + "def"
				{chunkIndex: 2, n: 6, expected: []byte("abcdef")},  // all prevs
				{chunkIndex: 2, n: 10, expected: []byte("abcdef")}, // n > all prevs
			},
		},
		{
			name:               "Write data, then close, flush partial last chunk",
			chunkSize:          5,
			inputData:          "HelloWorldPartial",                       // Hello, World, Parti, al
			expectedChunkDatas: []string{"Hello", "World", "Parti", "al"}, // Corrected
			prevNChecks: []struct {
				chunkIndex int
				n          int
				expected   []byte
			}{
				// Chunk 0: "Hello", prev: nil
				// Chunk 1: "World", prev: "Hello"
				// Chunk 2: "Parti", prev: "World"
				// Chunk 3: "al",    prev: "Parti"
				{chunkIndex: 2, n: 3, expected: []byte("rld")},           // On "Parti", prev "World", last 3 of "World"
				{chunkIndex: 2, n: 8, expected: []byte("lloWorld")},      // On "Parti", prevs "Hello"+"World"
				{chunkIndex: 3, n: 1, expected: []byte("i")},             // On "al", prev "Parti", last 1 of "Parti"
				{chunkIndex: 3, n: 7, expected: []byte("ldParti")},       // On "al", prevs "Parti"+"World" -> "ld" from World, "Parti"
				{chunkIndex: 3, n: 12, expected: []byte("loWorldParti")}, // Corrected: On "al", prevs "Parti"+"World"+"Hello" (tail of Hello)
			},
		},
		{
			name:               "Write data exactly chunksize, then close",
			chunkSize:          5,
			inputData:          "Exact", // Single chunk
			expectedChunkDatas: []string{"Exact"},
			prevNChecks: []struct {
				chunkIndex int
				n          int
				expected   []byte
			}{
				{chunkIndex: 0, n: 3, expected: []byte("")}, // No prev
			},
		},
		{
			name:               "Write data less than chunksize, then close",
			chunkSize:          10,
			inputData:          "Less", // Single chunk
			expectedChunkDatas: []string{"Less"},
			prevNChecks: []struct {
				chunkIndex int
				n          int
				expected   []byte
			}{
				{chunkIndex: 0, n: 3, expected: []byte("")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pr, pw := utils.NewPipe()

			go func() {
				pw.Write([]byte(tc.inputData))
				pw.Close()
			}()

			cm, err := NewTextChunkMaker(pr, WithChunkSize(tc.chunkSize))
			if err != nil {
				t.Fatalf("Failed to create ChunkMaker: %v", err)
			}

			var outputChunks []Chunk
			for chunk := range cm.OutputChannel() {
				outputChunks = append(outputChunks, chunk)
			}

			if len(outputChunks) != len(tc.expectedChunkDatas) {
				t.Fatalf("Expected %d chunks, got %d", len(tc.expectedChunkDatas), len(outputChunks))
			}

			for i, outChunk := range outputChunks {
				expectedData := []byte(tc.expectedChunkDatas[i])
				if !bytes.Equal(outChunk.Data(), expectedData) {
					t.Errorf("Chunk %d data: expected %q, got %q", i, string(expectedData), string(outChunk.Data()))
				}

				if i > 0 {
					if outChunk.LastChunk() != outputChunks[i-1] {
						t.Errorf("Chunk %d LastChunk() should be chunk %d (expected_ptr: %p), but got %p",
							i, i-1, outputChunks[i-1], outChunk.LastChunk())
					}
				} else {
					if outChunk.LastChunk() != nil {
						t.Errorf("Chunk 0 LastChunk() should be nil, but got %p", outChunk.LastChunk())
					}
				}
			}

			// Perform PrevNBytes checks
			for _, check := range tc.prevNChecks {
				if check.chunkIndex >= len(outputChunks) {
					t.Errorf("PrevNBytes check: chunkIndex %d out of bounds (%d chunks)", check.chunkIndex, len(outputChunks))
					continue
				}
				chunkToTest := outputChunks[check.chunkIndex]
				actualNBytes := chunkToTest.PrevNBytes(check.n)
				if !bytes.Equal(actualNBytes, check.expected) {
					// Construct a more informative message for PrevNBytes failure
					var prevChainStr string
					curr := chunkToTest.LastChunk()
					var history []string
					for curr != nil {
						history = append([]string{string(curr.Data())}, history...)
						curr = curr.LastChunk()
					}
					if len(history) > 0 {
						prevChainStr = strings.Join(history, " <- ")
					} else {
						prevChainStr = "<nil>"
					}
					t.Errorf("Chunk %d (data: %s, prev chain: [%s]).PrevNBytes(%d): expected %q, got %q",
						check.chunkIndex, string(chunkToTest.Data()), prevChainStr, check.n, string(check.expected), string(actualNBytes))
				}
			}
		})
	}
}
