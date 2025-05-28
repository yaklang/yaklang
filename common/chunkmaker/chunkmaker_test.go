package chunkmaker

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"

	"unicode/utf8"

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

			log.Info("start to create new chunk maker for test case: ", tc.name)

			// 创建 ChunkMaker
			cm, err := NewChunkMaker(pr, WithChunkSize(tc.chunkSize))
			if err != nil {
				t.Fatalf("Failed to create ChunkMaker: %v", err)
			}

			log.Info("start to collect output for test case: ", tc.name)
			// 收集输出
			var chunks []string
			for chunk := range cm.OutputChannel() {
				spew.Dump(chunk.Data())
				chunks = append(chunks, string(chunk.Data()))
			}

			log.Info("end to collect output for test case: ", tc.name)
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

			log.Info("start to create new chunk maker for test case: ", tc.name)

			// 创建 ChunkMaker
			cm, err := NewChunkMaker(pr, WithChunkSize(tc.chunkSize))

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

			log.Info("start to collect output for test case: ", tc.name)
			// 收集输出
			var chunks []string
			outputChan := cm.OutputChannel()
			for chunk := range outputChan {
				spew.Dump(chunk.Data())
				chunks = append(chunks, string(chunk.Data()))
			}
			log.Info("end to collect output for test case: ", tc.name)

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
