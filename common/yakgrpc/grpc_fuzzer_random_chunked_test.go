package yakgrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_HTTPFuzzer_RandomChunked(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		minChunkLength int64
		maxChunkLength int64
		minDelay       int64
		maxDelay       int64
		description    string
	}{
		{
			name:           "Small_Data_Fine_Chunks",
			requestBody:    `{"test": "data"}`,
			minChunkLength: 3,
			maxChunkLength: 8,
			minDelay:       50,
			maxDelay:       150,
			description:    "小数据精细分块测试 - 16字节数据，3-8字节分块",
		},
		{
			name:           "Medium_Data_Medium_Chunks",
			requestBody:    `{"test": "data for encoding"}`,
			minChunkLength: 4,
			maxChunkLength: 10,
			minDelay:       300,
			maxDelay:       1200,
			description:    "中等数据中等分块测试 - 28字节数据，4-10字节分块",
		},
		{
			name:           "Tiny_Data_Tiny_Chunks",
			requestBody:    `{"x":"y"}`,
			minChunkLength: 1,
			maxChunkLength: 3,
			minDelay:       200,
			maxDelay:       800,
			description:    "微小数据微小分块测试 - 9字节数据，1-3字节分块",
		},
		{
			name:           "Single_Byte_Chunks",
			requestBody:    `{"ab":"cd"}`,
			minChunkLength: 1,
			maxChunkLength: 1,
			minDelay:       12,
			maxDelay:       50,
			description:    "单字节分块测试 - 12字节数据，每个chunk 1字节",
		},
		{
			name:           "Large_Chunks",
			requestBody:    `{"test": "data"}`,
			minChunkLength: 8,
			maxChunkLength: 20,
			minDelay:       54,
			maxDelay:       150,
			description:    "大分块测试 - 16字节数据，8-20字节分块",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("执行测试: %s", tt.description)

			// 创建echo服务器
			host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
				// 解析HTTP请求，提取body
				reqStr := string(req)
				bodyStart := "\r\n\r\n"
				if idx := strings.Index(reqStr, bodyStart); idx != -1 {
					body := reqStr[idx+len(bodyStart):]
					response := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
					return []byte(response)
				}
				// 如果没有body，返回空响应
				return []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
			})

			client, err := NewLocalClient()
			require.NoError(t, err)

			// 发送带RandomChunked的请求
			stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
				Request: fmt.Sprintf(`POST /test HTTP/1.1
Host: %s
Content-Type: application/json

%s`, utils.HostPort(host, port), tt.requestBody),
				EnableRandomChunked:    true,
				RandomChunkedMinLength: tt.minChunkLength,
				RandomChunkedMaxLength: tt.maxChunkLength,
				RandomChunkedMinDelay:  tt.minDelay,
				RandomChunkedMaxDelay:  tt.maxDelay,
				DisableUseConnPool:     true,
			})
			require.NoError(t, err)

			var receivedResponse *ypb.FuzzerResponse
			for {
				rsp, err := stream.Recv()
				if err != nil {
					break
				}
				if rsp.Ok {
					receivedResponse = rsp
					break
				}
			}

			require.NotNil(t, receivedResponse, "应该收到成功的响应")

			// 验证RandomChunkedData字段存在且有数据
			require.NotNil(t, receivedResponse.RandomChunkedData, "RandomChunkedData字段不应为nil")
			require.Greater(t, len(receivedResponse.RandomChunkedData), 0, "应该有chunked数据")

			t.Logf("收到 %d 个chunks", len(receivedResponse.RandomChunkedData))

			// 验证chunk数据并重建原始数据
			var reconstructedData []byte
			totalChunks := len(receivedResponse.RandomChunkedData)

			for i, chunk := range receivedResponse.RandomChunkedData {
				t.Logf("  Chunk %d: Index=%d, Length=%d, Data=%q DelayTime = %dms TotalDelayTime = %dms",
					i+1, chunk.Index, chunk.ChunkedLength, string(chunk.Data), chunk.CurrentChunkedDelayTime, chunk.TotalDelayTime)

				// 验证chunk的基本字段
				require.Equal(t, int64(i+1), chunk.Index, "chunk索引应该按顺序递增")
				require.Greater(t, len(chunk.Data), 0, "chunk数据不应为空")
				require.Greater(t, chunk.ChunkedLength, int64(0), "chunk长度应该大于0")

				// 验证每个chunk大小在设定范围内（最后一个chunk可能小于最小值）
				chunkDataLen := int64(len(chunk.Data))
				isLastChunk := (i == totalChunks-1)

				if isLastChunk && chunkDataLen < tt.minChunkLength {
					// 最后一个chunk允许小于最小长度（剩余数据不足时）
					t.Logf("    最后chunk长度 %d 小于最小值 %d（剩余数据不足）", chunkDataLen, tt.minChunkLength)
				} else {
					// 非最后chunk或最后chunk足够大时，必须在范围内
					require.GreaterOrEqual(t, chunkDataLen, tt.minChunkLength,
						"chunk大小应该大于等于最小值 %d", tt.minChunkLength)
				}

				require.LessOrEqual(t, chunkDataLen, tt.maxChunkLength,
					"chunk大小应该小于等于最大值 %d", tt.maxChunkLength)

				// 重建原始数据
				reconstructedData = append(reconstructedData, chunk.Data...)
			}

			// 验证数据完整性
			expectedDataLength := len(tt.requestBody)
			completeness := float64(len(reconstructedData)) / float64(expectedDataLength) * 100
			t.Logf("数据完整性: %.1f%% (%d/%d bytes)", completeness, len(reconstructedData), expectedDataLength)

			require.Equal(t, tt.requestBody, string(reconstructedData),
				"重建的数据应该与原始请求数据完全一致")

			// 验证延迟时间是累积的
			var lastTotalDelay int64 = 0
			for _, chunk := range receivedResponse.RandomChunkedData {
				require.GreaterOrEqual(t, chunk.TotalDelayTime, lastTotalDelay, "总延迟时间应该是累积的")
				lastTotalDelay = chunk.TotalDelayTime
			}

			t.Logf("✓ 测试 %s 完成: %d chunks, %d bytes, %.1f%% 完整性",
				tt.name, totalChunks, len(reconstructedData), completeness)
		})
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_RandomChunked_WithPool(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		minChunkLength int64
		maxChunkLength int64
		minDelay       int64
		maxDelay       int64
		description    string
	}{
		{
			name:           "Pool_Small_Data_Fine_Chunks",
			requestBody:    `{"test": "pool"}`,
			minChunkLength: 2,
			maxChunkLength: 6,
			minDelay:       32,
			maxDelay:       100,
			description:    "连接池小数据精细分块测试 - 17字节数据，2-6字节分块",
		},
		{
			name:           "Pool_Medium_Data_Medium_Chunks",
			requestBody:    `{"test": "pool data encoding"}`,
			minChunkLength: 4,
			maxChunkLength: 8,
			minDelay:       24,
			maxDelay:       877,
			description:    "连接池中等数据中等分块测试 - 30字节数据，4-8字节分块",
		},
		{
			name:           "Pool_Tiny_Chunks",
			requestBody:    `{"pool":"test"}`,
			minChunkLength: 1,
			maxChunkLength: 2,
			minDelay:       15,
			maxDelay:       51,
			description:    "连接池微小分块测试 - 16字节数据，1-2字节分块",
		},
		{
			name:           "Pool_Single_Byte_Chunks",
			requestBody:    `{"ab":"cd"}`,
			minChunkLength: 1,
			maxChunkLength: 1,
			minDelay:       12,
			maxDelay:       32,
			description:    "连接池单字节分块测试 - 12字节数据，每个chunk 1字节",
		},
		{
			name:           "Pool_Large_Pool_Size",
			requestBody:    `{"large":"pool"}`,
			minChunkLength: 3,
			maxChunkLength: 7,
			minDelay:       21,
			maxDelay:       300,
			description:    "大连接池测试 - 17字节数据，3-7字节分块",
		},
		{
			name:           "Big Request",
			requestBody:    strings.Repeat("A", 200),
			minChunkLength: 10,
			maxChunkLength: 25,
			minDelay:       20,
			maxDelay:       50,
			description:    "大请求体测试",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("执行连接池测试: %s ", tt.description)

			// 创建echo服务器
			host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
				// 解析HTTP请求，提取body
				reqStr := string(req)
				bodyStart := "\r\n\r\n"
				if idx := strings.Index(reqStr, bodyStart); idx != -1 {
					body := reqStr[idx+len(bodyStart):]
					response := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
					return []byte(response)
				}
				// 如果没有body，返回空响应
				return []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
			})

			client, err := NewLocalClient()
			require.NoError(t, err)

			// 发送带RandomChunked和HTTPPool的请求
			stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
				Request: fmt.Sprintf(`POST /test HTTP/1.1
Host: %s
Content-Type: application/json

%s`, utils.HostPort(host, port), tt.requestBody),
				EnableRandomChunked:    true,
				RandomChunkedMinLength: tt.minChunkLength,
				RandomChunkedMaxLength: tt.maxChunkLength,
				RandomChunkedMinDelay:  tt.minDelay,
				RandomChunkedMaxDelay:  tt.maxDelay,
				// 启用HTTP连接池
				DisableUseConnPool: false,
			})
			require.NoError(t, err)

			var receivedResponse *ypb.FuzzerResponse
			for {
				rsp, err := stream.Recv()
				if err != nil {
					break
				}
				if rsp.Ok {
					receivedResponse = rsp
					break
				}
			}

			require.NotNil(t, receivedResponse, "应该收到成功的响应")

			// 验证RandomChunkedData字段存在且有数据
			require.NotNil(t, receivedResponse.RandomChunkedData, "RandomChunkedData字段不应为nil")
			require.Greater(t, len(receivedResponse.RandomChunkedData), 0, "应该有chunked数据")

			t.Logf("收到 %d 个chunks", len(receivedResponse.RandomChunkedData))

			// 验证chunk数据并重建原始数据
			var reconstructedData []byte
			totalChunks := len(receivedResponse.RandomChunkedData)

			for i, chunk := range receivedResponse.RandomChunkedData {
				t.Logf("  Chunk %d: Index=%d, Length=%d, Data=%q DelayTime = %dms TotalDelayTime = %dms",
					i+1, chunk.Index, chunk.ChunkedLength, string(chunk.Data), chunk.CurrentChunkedDelayTime, chunk.TotalDelayTime)

				// 验证chunk的基本字段
				require.Equal(t, int64(i+1), chunk.Index, "chunk索引应该按顺序递增")
				require.Greater(t, len(chunk.Data), 0, "chunk数据不应为空")
				require.Greater(t, chunk.ChunkedLength, int64(0), "chunk长度应该大于0")

				// 验证每个chunk大小在设定范围内（最后一个chunk可能小于最小值）
				chunkDataLen := int64(len(chunk.Data))
				isLastChunk := (i == totalChunks-1)

				if isLastChunk && chunkDataLen < tt.minChunkLength {
					// 最后一个chunk允许小于最小长度（剩余数据不足时）
					t.Logf("    最后chunk长度 %d 小于最小值 %d（剩余数据不足）", chunkDataLen, tt.minChunkLength)
				} else {
					// 非最后chunk或最后chunk足够大时，必须在范围内
					require.GreaterOrEqual(t, chunkDataLen, tt.minChunkLength,
						"chunk大小应该大于等于最小值 %d", tt.minChunkLength)
				}

				require.LessOrEqual(t, chunkDataLen, tt.maxChunkLength,
					"chunk大小应该小于等于最大值 %d", tt.maxChunkLength)

				// 重建原始数据
				reconstructedData = append(reconstructedData, chunk.Data...)
			}

			// 验证数据完整性 - 要求100%数据完整性
			expectedDataLength := len(tt.requestBody)
			completeness := float64(len(reconstructedData)) / float64(expectedDataLength) * 100
			t.Logf("数据完整性: %.1f%% (%d/%d bytes)", completeness, len(reconstructedData), expectedDataLength)

			require.Equal(t, tt.requestBody, string(reconstructedData),
				"重建的数据应该与原始请求数据完全一致")

			// 验证延迟时间是累积的
			var lastTotalDelay int64 = 0
			for _, chunk := range receivedResponse.RandomChunkedData {
				require.GreaterOrEqual(t, chunk.TotalDelayTime, lastTotalDelay, "总延迟时间应该是累积的")
				lastTotalDelay = chunk.TotalDelayTime
			}

			t.Logf("✓ 连接池测试 %s 完成: %d chunks, %d bytes, %.1f%% 完整性,",
				tt.name, totalChunks, len(reconstructedData), completeness)
		})
	}
}
