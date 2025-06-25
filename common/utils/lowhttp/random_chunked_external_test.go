package lowhttp_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestRandomChunkedHTTPExternal(t *testing.T) {
	tests := []struct {
		name             string
		requestBody      string
		minChunkLength   int
		maxChunkLength   int
		minDelay         time.Duration
		maxDelay         time.Duration
		expectError      bool
		validateHandler  bool
		validateResponse func(t *testing.T, responseBody string, originalBody string)
		serverHandler    func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:            "basic chunked request",
			requestBody:     strings.Repeat("A", 50),
			minChunkLength:  5,
			maxChunkLength:  15,
			minDelay:        10 * time.Millisecond,
			maxDelay:        50 * time.Millisecond,
			expectError:     false,
			validateHandler: true,
			validateResponse: func(t *testing.T, responseBody string, originalBody string) {
				assert.Equal(t, originalBody, responseBody, "response should echo request body")
			},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusInternalServerError)
					return
				}
				w.Write(body)
			},
		},
		{
			name:            "large data chunked",
			requestBody:     strings.Repeat("B", 200),
			minChunkLength:  20,
			maxChunkLength:  50,
			minDelay:        5 * time.Millisecond,
			maxDelay:        20 * time.Millisecond,
			expectError:     false,
			validateHandler: true,
			validateResponse: func(t *testing.T, responseBody string, originalBody string) {
				assert.Equal(t, originalBody, responseBody, "response should echo request body")
			},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusInternalServerError)
					return
				}
				w.Write(body)
			},
		},
		{
			name:            "small chunks",
			requestBody:     "Hello World Test",
			minChunkLength:  1,
			maxChunkLength:  3,
			minDelay:        1 * time.Millisecond,
			maxDelay:        5 * time.Millisecond,
			expectError:     false,
			validateHandler: true,
			validateResponse: func(t *testing.T, responseBody string, originalBody string) {
				assert.Equal(t, originalBody, responseBody, "response should echo request body")
			},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusInternalServerError)
					return
				}
				w.Write(body)
			},
		},
		{
			name:            "json payload",
			requestBody:     `{"message":"test","data":{"items":[1,2,3],"status":"active"}}`,
			minChunkLength:  10,
			maxChunkLength:  30,
			minDelay:        0,
			maxDelay:        10 * time.Millisecond,
			expectError:     false,
			validateHandler: true,
			validateResponse: func(t *testing.T, responseBody string, originalBody string) {
				assert.Equal(t, originalBody, responseBody, "response should echo request body")
			},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusInternalServerError)
					return
				}
				w.Write(body)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := utils.DebugMockHTTPHandlerFunc(tt.serverHandler)

			var handlerCallbacks []map[string]interface{}
			var actualHandler func(id int, chunkRaw []byte, totalTime time.Duration, chunkSendTime time.Duration)

			if tt.validateHandler {
				actualHandler = func(id int, chunkRaw []byte, totalTime time.Duration, chunkSendTime time.Duration) {
					callback := map[string]interface{}{
						"chunk_id":        id,
						"chunk_raw":       string(chunkRaw),
						"chunk_length":    len(chunkRaw),
						"total_time":      totalTime,
						"chunk_send_time": chunkSendTime,
					}
					handlerCallbacks = append(handlerCallbacks, callback)
					t.Logf("Chunk %d: data=%q, length=%d, totalTime=%v, chunkTime=%v",
						id, string(chunkRaw), len(chunkRaw), totalTime, chunkSendTime)
				}
			}

			opts := []lowhttp.LowhttpOpt{
				lowhttp.WithRequest(fmt.Sprintf(`POST /echo HTTP/1.1
Host: %s
Accept-Language: en-US;q=0.9,en;q=0.8
User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36
Content-Type: application/json

%s`, utils.HostPort(host, port), tt.requestBody)),
				lowhttp.WithEnableRandomChunked(true),
				lowhttp.WithRandomChunkedLength(tt.minChunkLength, tt.maxChunkLength),
			}

			opts = append(opts, lowhttp.WithRandomChunkedDelay(tt.minDelay, tt.maxDelay))
			if actualHandler != nil {
				opts = append(opts, lowhttp.WithRandomChunkedHandler(actualHandler))
			}

			// 发送请求
			start := time.Now()
			rsp, err := lowhttp.HTTP(opts...)
			duration := time.Since(start)

			// 验证错误期望
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err, "HTTP request should succeed")
			require.NotNil(t, rsp, "response should not be nil")

			// 验证响应
			responseBody := string(rsp.GetBody())
			t.Logf("Response length: %d", len(responseBody))

			if tt.validateResponse != nil {
				tt.validateResponse(t, responseBody, tt.requestBody)
			}

			// 分块结果回调
			if tt.validateHandler {
				// 验证回调被调用
				assert.Greater(t, len(handlerCallbacks), 0, "handler should be called at least once")

				// 重建所有chunk数据
				var reconstructedData []byte
				for i, callback := range handlerCallbacks {
					chunkId := callback["chunk_id"].(int)

					// 验证chunkIndex递增
					assert.Equal(t, i+1, callback["chunk_id"], "chunk index should be sequential starting from 1")

					// 验证数据大小在范围内
					chunkLength := callback["chunk_length"].(int)
					isLastChunk := (i == len(handlerCallbacks)-1)
					if isLastChunk && chunkLength < tt.minChunkLength {
						t.Logf("Last chunk length %d is smaller than minChunk %d (remaining data)", chunkLength, tt.minChunkLength)
					} else {
						assert.GreaterOrEqual(t, chunkLength, tt.minChunkLength, "chunk length should be >= minChunk")
					}
					assert.LessOrEqual(t, chunkLength, tt.maxChunkLength, "chunk length should be <= maxChunk")

					// 验证时间
					totalTime := callback["total_time"].(time.Duration)
					chunkTime := callback["chunk_send_time"].(time.Duration)
					if chunkId != 1 {
						assert.Greater(t, totalTime, time.Duration(0), "total time should be positive")
					}

					// 只有当chunkTime > 0时才验证它小于等于totalTime
					if chunkId > 1 {
						assert.LessOrEqual(t, chunkTime, totalTime, "chunk time should be <= total time")
					}
					// 收集数据重建
					reconstructedData = append(reconstructedData, []byte(callback["chunk_raw"].(string))...)
				}

				// 验证重建的数据与原始数据一致
				assert.Equal(t, tt.requestBody, string(reconstructedData), "reconstructed chunk data should match original request body")

				// 验证时间序列递增
				for i := 1; i < len(handlerCallbacks); i++ {
					prevTotal := handlerCallbacks[i-1]["total_time"].(time.Duration)
					currTotal := handlerCallbacks[i]["total_time"].(time.Duration)
					assert.GreaterOrEqual(t, currTotal, prevTotal, "total time should not decrease")
				}

				// 验证延迟效果（如果设置了延迟）
				// 时间测试不进去CI
				//if tt.minDelay > 0 && len(handlerCallbacks) > 1 {
				//	expectedMinDuration := time.Duration(len(handlerCallbacks)-1) * tt.minDelay / 2 // 允许误差
				//	assert.GreaterOrEqual(t, duration, expectedMinDuration, "total duration should include chunk delays")
				//}
			}
			t.Logf("Test completed in %v with %d chunks", duration, len(handlerCallbacks))
		})
	}
}

func TestRandomChunkedHTTPWithConnectionPool(t *testing.T) {
	tests := []struct {
		name             string
		requestBody      string
		minChunkLength   int
		maxChunkLength   int
		minDelay         time.Duration
		maxDelay         time.Duration
		poolSize         int
		requestCount     int
		validateHandler  bool
		validateResponse func(t *testing.T, responseBody string, originalBody string)
		serverHandler    func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:            "connection pool with chunked - basic",
			requestBody:     strings.Repeat("POOL_TEST_", 10),
			minChunkLength:  8,
			maxChunkLength:  20,
			minDelay:        5 * time.Millisecond,
			maxDelay:        15 * time.Millisecond,
			poolSize:        3,
			requestCount:    1,
			validateHandler: true,
			validateResponse: func(t *testing.T, responseBody string, originalBody string) {
				assert.Equal(t, originalBody, responseBody, "response should echo request body")
			},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusInternalServerError)
					return
				}
				w.Write(body)
			},
		},
		{
			name:            "connection pool with chunked - multiple requests",
			requestBody:     `{"test":"connection_pool","data":"` + strings.Repeat("X", 50) + `"}`,
			minChunkLength:  10,
			maxChunkLength:  25,
			minDelay:        0,
			maxDelay:        10 * time.Millisecond,
			poolSize:        2,
			requestCount:    3,
			validateHandler: true,
			validateResponse: func(t *testing.T, responseBody string, originalBody string) {
				assert.Equal(t, originalBody, responseBody, "response should echo request body")
			},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusInternalServerError)
					return
				}
				w.Write(body)
			},
		},
		{
			name:            "connection pool with large chunks",
			requestBody:     strings.Repeat("LARGE_CHUNK_DATA_", 20),
			minChunkLength:  30,
			maxChunkLength:  60,
			minDelay:        1 * time.Millisecond,
			maxDelay:        5 * time.Millisecond,
			poolSize:        4,
			requestCount:    2,
			validateHandler: true,
			validateResponse: func(t *testing.T, responseBody string, originalBody string) {
				assert.Equal(t, originalBody, responseBody, "response should echo request body")
			},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read request body", http.StatusInternalServerError)
					return
				}
				w.Write(body)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := utils.DebugMockHTTPHandlerFunc(tt.serverHandler)

			// 为每个请求收集回调数据
			var allHandlerCallbacks [][]map[string]interface{}

			for i := 0; i < tt.requestCount; i++ {
				var handlerCallbacks []map[string]interface{}
				var actualHandler func(id int, chunkRaw []byte, totalTime time.Duration, chunkSendTime time.Duration)

				if tt.validateHandler {
					actualHandler = func(id int, chunkRaw []byte, totalTime time.Duration, chunkSendTime time.Duration) {
						callback := map[string]interface{}{
							"chunk_id":        id,
							"chunk_raw":       string(chunkRaw),
							"chunk_length":    len(chunkRaw),
							"total_time":      totalTime,
							"chunk_send_time": chunkSendTime,
						}
						handlerCallbacks = append(handlerCallbacks, callback)
						t.Logf("Request %d - Chunk %d: data=%q, length=%d, totalTime=%v, chunkTime=%v",
							i+1, id, string(chunkRaw), len(chunkRaw), totalTime, chunkSendTime)
					}
				}

				// 创建自定义连接池
				customPool := lowhttp.NewHttpConnPool(context.Background(), tt.poolSize*10, tt.poolSize)

				opts := []lowhttp.LowhttpOpt{
					lowhttp.WithRequest(fmt.Sprintf(`POST /echo HTTP/1.1
Host: %s
Accept-Language: en-US;q=0.9,en;q=0.8
User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36
Content-Type: application/json

%s`, utils.HostPort(host, port), tt.requestBody)),
					lowhttp.WithEnableRandomChunked(true),
					lowhttp.WithRandomChunkedLength(tt.minChunkLength, tt.maxChunkLength),
					lowhttp.WithRandomChunkedDelay(tt.minDelay, tt.maxDelay),
					// 启用连接池
					lowhttp.WithConnPool(true),
					lowhttp.ConnPool(customPool),
				}

				if actualHandler != nil {
					opts = append(opts, lowhttp.WithRandomChunkedHandler(actualHandler))
				}

				// 发送请求
				start := time.Now()
				rsp, err := lowhttp.HTTP(opts...)
				duration := time.Since(start)

				require.NoError(t, err, "HTTP request %d should succeed", i+1)
				require.NotNil(t, rsp, "response %d should not be nil", i+1)

				// 验证响应
				responseBody := string(rsp.GetBody())
				t.Logf("Request %d - Response length: %d", i+1, len(responseBody))

				if tt.validateResponse != nil {
					tt.validateResponse(t, responseBody, tt.requestBody)
				}

				// 验证分块回调
				if tt.validateHandler {
					// 验证回调被调用
					assert.Greater(t, len(handlerCallbacks), 0, "handler should be called at least once for request %d", i+1)

					// 重建所有chunk数据
					var reconstructedData []byte
					for j, callback := range handlerCallbacks {
						chunkId := callback["chunk_id"].(int)
						// 验证chunkIndex递增
						assert.Equal(t, j+1, chunkId, "chunk index should be sequential starting from 1 for request %d", i+1)

						// 验证数据大小在范围内
						chunkLength := callback["chunk_length"].(int)
						isLastChunk := (j == len(handlerCallbacks)-1)
						if isLastChunk && chunkLength < tt.minChunkLength {
							t.Logf("Request %d - Last chunk length %d is smaller than minChunk %d (remaining data)", i+1, chunkLength, tt.minChunkLength)
						} else {
							assert.GreaterOrEqual(t, chunkLength, tt.minChunkLength, "chunk length should be >= minChunk for request %d", i+1)
						}
						assert.LessOrEqual(t, chunkLength, tt.maxChunkLength, "chunk length should be <= maxChunk for request %d", i+1)

						// 验证时间
						totalTime := callback["total_time"].(time.Duration)
						chunkTime := callback["chunk_send_time"].(time.Duration)
						if chunkId != 1 {
							assert.Greater(t, totalTime, time.Duration(0), "total time should be positive for request %d", i+1)
						}
						// 只有当chunkTime > 0时才验证它小于等于totalTime
						if chunkId > 1 {
							assert.LessOrEqual(t, chunkTime, totalTime, "chunk time should be <= total time for request %d", i+1)
						}
						// 收集数据重建
						reconstructedData = append(reconstructedData, []byte(callback["chunk_raw"].(string))...)
					}

					// 验证重建的数据与原始数据一致
					assert.Equal(t, tt.requestBody, string(reconstructedData), "reconstructed chunk data should match original request body for request %d", i+1)

					// 验证时间序列递增
					for j := 1; j < len(handlerCallbacks); j++ {
						prevTotal := handlerCallbacks[j-1]["total_time"].(time.Duration)
						currTotal := handlerCallbacks[j]["total_time"].(time.Duration)
						assert.GreaterOrEqual(t, currTotal, prevTotal, "total time should not decrease for request %d", i+1)
					}

					// 保存当前请求的回调数据
					allHandlerCallbacks = append(allHandlerCallbacks, handlerCallbacks)
				}

				t.Logf("Request %d completed in %v with %d chunks", i+1, duration, len(handlerCallbacks))
			}

			// 验证所有请求的整体情况
			if tt.validateHandler {
				totalChunks := 0
				for i, callbacks := range allHandlerCallbacks {
					totalChunks += len(callbacks)
					t.Logf("Request %d used %d chunks", i+1, len(callbacks))
				}
				t.Logf("Total chunks across all requests: %d", totalChunks)
				assert.Greater(t, totalChunks, 0, "should have processed some chunks across all requests")
			}
		})
	}
}
