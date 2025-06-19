package lowhttp

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomChunkedSender_getRandomDelayTime(t *testing.T) {
	tests := []struct {
		name     string
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{
			name:     "normal range",
			minDelay: 50 * time.Millisecond,
			maxDelay: 100 * time.Millisecond,
		},
		{
			name:     "large range",
			minDelay: 1 * time.Second,
			maxDelay: 5 * time.Second,
		},
		{
			name:     "small range",
			minDelay: 1 * time.Millisecond,
			maxDelay: 2 * time.Millisecond,
		},
		{
			name:     "equal min and max",
			minDelay: 100 * time.Millisecond,
			maxDelay: 100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &randomChunkedSender{
				minDelay: tt.minDelay,
				maxDelay: tt.maxDelay,
			}

			// 测试多次确保范围正确
			for i := 0; i < 100; i++ {
				delay := sender.getRandomDelayTime()

				assert.GreaterOrEqual(t, delay, tt.minDelay,
					"delay should be >= minDelay")
				assert.LessOrEqual(t, delay, tt.maxDelay,
					"delay should be <= maxDelay")
			}
		})
	}
}

func TestRandomChunkedSender_calcRandomChunkedLen(t *testing.T) {
	tests := []struct {
		name           string
		minChunkLength int
		maxChunkLength int
	}{
		{
			name:           "normal range",
			minChunkLength: 256,
			maxChunkLength: 1024,
		},
		{
			name:           "large range",
			minChunkLength: 1024,
			maxChunkLength: 8192,
		},
		{
			name:           "small range",
			minChunkLength: 1,
			maxChunkLength: 10,
		},
		{
			name:           "equal min and max",
			minChunkLength: 512,
			maxChunkLength: 512,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &randomChunkedSender{
				minChunkLength: tt.minChunkLength,
				maxChunkLength: tt.maxChunkLength,
			}

			// 测试多次确保范围正确
			for i := 0; i < 100; i++ {
				length := sender.calcRandomChunkedLen()

				assert.GreaterOrEqual(t, length, tt.minChunkLength,
					"chunk length should be >= minChunkLength")
				assert.LessOrEqual(t, length, tt.maxChunkLength,
					"chunk length should be <= maxChunkLength")
			}
		})
	}
}

func TestRandomChunkedSender_send(t *testing.T) {
	tests := []struct {
		name           string
		requestPacket  string
		minChunkLength int
		maxChunkLength int
		minDelay       time.Duration
		maxDelay       time.Duration
		handler        ChunkedResultHandler
		expectError    bool
		validateFunc   func(t *testing.T, result string, originalBody string, minChunk, maxChunk int)
	}{
		{
			name: "basic send with body",
			requestPacket: `POST /api/test HTTP/1.1
Host: example.com
Transfer-Encoding: chunked

%s`,
			minChunkLength: 5,
			maxChunkLength: 10,
			minDelay:       0,
			maxDelay:       0,
			expectError:    false,
			validateFunc: func(t *testing.T, result string, originalBody string, minChunk, maxChunk int) {
				_, newBody := SplitHTTPHeadersAndBodyFromPacket([]byte(result))

				// 验证包含HTTP头部
				assert.Contains(t, result, "POST /api/test HTTP/1.1")
				assert.Contains(t, result, "Host: example.com")
				assert.Contains(t, result, "Transfer-Encoding: chunked")

				// 验证分块数据解码后正确
				bodyRaw, err := codec.HTTPChunkedDecode(newBody)
				assert.NoError(t, err)
				assert.Equal(t, originalBody, string(bodyRaw))

				// 验证以结束分块标记结尾
				assert.True(t, strings.HasSuffix(result, "0"+DoubleCRLF))

				// 验证分块长度在范围内
				validateChunkSizes(t, newBody, minChunk, maxChunk)
			},
		},
		{
			name: "empty body",
			requestPacket: `GET /api/test HTTP/1.1
Host: example.com
Transfer-Encoding: chunked

`,
			minChunkLength: 1,
			maxChunkLength: 10,
			expectError:    false,
			validateFunc: func(t *testing.T, result string, originalBody string, minChunk, maxChunk int) {
				// 验证包含HTTP头部
				assert.Contains(t, result, "GET /api/test HTTP/1.1")
				assert.Contains(t, result, "Host: example.com")

				// 验证直接以结束分块标记结尾
				assert.True(t, strings.HasSuffix(result, "0"+DoubleCRLF))
			},
		},
		{
			name: "small chunks",
			requestPacket: `POST /api/test HTTP/1.1
Host: example.com
Transfer-Encoding: chunked

%s`,
			minChunkLength: 1,
			maxChunkLength: 3,
			expectError:    false,
			validateFunc: func(t *testing.T, result string, originalBody string, minChunk, maxChunk int) {
				_, newBody := SplitHTTPHeadersAndBodyFromPacket([]byte(result))

				// 验证分块数据解码后正确
				bodyRaw, err := codec.HTTPChunkedDecode(newBody)
				assert.NoError(t, err)
				assert.Equal(t, originalBody, string(bodyRaw))

				// 验证分块长度在范围内
				validateChunkSizes(t, newBody, minChunk, maxChunk)
			},
		},
		{
			name: "large chunks",
			requestPacket: `POST /api/test HTTP/1.1
Host: example.com
Transfer-Encoding: chunked

%s`,
			minChunkLength: 50,
			maxChunkLength: 100,
			expectError:    false,
			validateFunc: func(t *testing.T, result string, originalBody string, minChunk, maxChunk int) {
				_, newBody := SplitHTTPHeadersAndBodyFromPacket([]byte(result))

				// 验证分块数据解码后正确
				bodyRaw, err := codec.HTTPChunkedDecode(newBody)
				assert.NoError(t, err)
				assert.Equal(t, originalBody, string(bodyRaw))

				// 验证分块长度在范围内
				validateChunkSizes(t, newBody, minChunk, maxChunk)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var originalBody string
			if strings.Contains(tt.requestPacket, "%s") {
				if tt.name == "large chunks" {
					originalBody = strings.Repeat("A", 1000) // 大数据用于测试大分块
				} else {
					originalBody = uuid.NewString()
				}
			}

			requestPacket := []byte(fmt.Sprintf(tt.requestPacket, originalBody))

			var buffer bytes.Buffer
			sender := &randomChunkedSender{
				ctx:            context.Background(),
				requestPacket:  requestPacket,
				minChunkLength: tt.minChunkLength,
				maxChunkLength: tt.maxChunkLength,
				minDelay:       tt.minDelay,
				maxDelay:       tt.maxDelay,
				handler:        tt.handler,
			}

			err := sender.send(&buffer)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			result := buffer.String()
			t.Logf("Result length: %d", len(result))

			if tt.validateFunc != nil {
				tt.validateFunc(t, result, originalBody, tt.minChunkLength, tt.maxChunkLength)
			}
		})
	}
}

func TestRandomChunkedSender_hanlder(t *testing.T) {
	t.Run("test with handler callback", func(t *testing.T) {
		minBlock := 100 / 25
		maxBlock := 100 / 4
		log.Info("minBlock:", minBlock)
		log.Info("maxBlock:", maxBlock)

		blockNum := 0
		options := []RandomChunkedHTTPOption{
			WithRandomChunkedLength(10, 25),
			WithRandomChunkedDelay(time.Millisecond*100, time.Millisecond*500),
			WithRandomChunkedHandler(func(chunkIndex int, chunkRaw []byte, totalDuration time.Duration, chunkDuration time.Duration) {
				m := make(map[string]any)
				m["id"] = chunkIndex
				m["totalTime"] = totalDuration
				m["data"] = string(chunkRaw)
				m["chunkDuration"] = chunkDuration
				t.Log(m)
				blockNum = chunkIndex + 1
			}),
		}
		token := strings.Repeat("A", 200)
		req := fmt.Sprintf(`POST /api/test HTTP/1.1
	Host: example.com
		Transfer-Encoding: chunked

		%s
		`, token)
		sender, err := newRandomChunkedSender([]byte(req), options...)
		require.NoError(t, err)

		var buffer bytes.Buffer
		sender.send(&buffer)

		require.GreaterOrEqual(t, blockNum, minBlock)
		require.LessOrEqual(t, blockNum, maxBlock)
	})
}

// validateChunkSizes 验证分块大小是否在指定范围内
func validateChunkSizes(t *testing.T, chunkedBody []byte, minChunk, maxChunk int) {
	// 解析分块数据，验证每个分块的大小
	reader := bytes.NewReader(chunkedBody)
	chunkSizes := []int{}

	for {
		// 读取分块大小行
		var sizeLine []byte
		for {
			b := make([]byte, 1)
			n, err := reader.Read(b)
			if err != nil || n == 0 {
				return // 读取结束
			}
			sizeLine = append(sizeLine, b[0])
			if len(sizeLine) >= 2 && string(sizeLine[len(sizeLine)-2:]) == CRLF {
				sizeLine = sizeLine[:len(sizeLine)-2] // 移除CRLF
				break
			}
		}

		if len(sizeLine) == 0 {
			continue
		}

		// 解析十六进制大小
		sizeStr := string(sizeLine)
		if sizeStr == "0" {
			break // 结束分块
		}

		var chunkSize int
		_, err := fmt.Sscanf(sizeStr, "%x", &chunkSize)
		if err != nil {
			t.Logf("Failed to parse chunk size: %s", sizeStr)
			continue
		}

		chunkSizes = append(chunkSizes, chunkSize)

		// 跳过分块数据和CRLF
		skipBytes := make([]byte, chunkSize+2) // +2 for CRLF
		reader.Read(skipBytes)
	}

	// 验证分块大小
	for i, chunkSize := range chunkSizes {
		isLastChunk := (i == len(chunkSizes)-1)

		// 对于最后一个分块，如果小于minChunk，说明是剩余数据不足的情况，这是允许的
		if isLastChunk && chunkSize < minChunk {
			// 最后一个分块可以小于minChunk（剩余数据不足）
			t.Logf("Last chunk size %d is smaller than minChunk %d (remaining data)", chunkSize, minChunk)
		} else {
			// 其他分块必须在范围内
			assert.GreaterOrEqual(t, chunkSize, minChunk,
				"chunk %d size %d should be >= minChunk %d", i, chunkSize, minChunk)
		}

		// 所有分块都不能超过maxChunk
		assert.LessOrEqual(t, chunkSize, maxChunk,
			"chunk %d size %d should be <= maxChunk %d", i, chunkSize, maxChunk)
	}

	// 如果有多个分块，至少要有一个分块在指定范围内
	if len(chunkSizes) > 1 {
		hasValidSizeChunk := false
		for i, chunkSize := range chunkSizes {
			isLastChunk := (i == len(chunkSizes)-1)
			if !isLastChunk && chunkSize >= minChunk && chunkSize <= maxChunk {
				hasValidSizeChunk = true
				break
			}
		}
		if !hasValidSizeChunk && len(chunkSizes) > 1 {
			// 如果只有最后一个分块且它小于minChunk，检查是否因为数据太小
			lastChunk := chunkSizes[len(chunkSizes)-1]
			if len(chunkSizes) == 1 || lastChunk >= minChunk {
				assert.True(t, hasValidSizeChunk, "should have at least one chunk in the specified range")
			}
		}
	}
}
