package yakgrpc

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_MatcherActions(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	testUUID := uuid.NewString()[:8]
	testFlows := []struct {
		name        string
		requestRaw  []byte
		responseRaw []byte
		shouldMatch bool
		description string
	}{
		{
			name:        "matched_flow_1",
			requestRaw:  []byte("GET /test1 HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			responseRaw: []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html><body>success_keyword_" + testUUID + "</body></html>"),
			shouldMatch: true,
			description: "应该匹配的流量1",
		},
		{
			name:        "matched_flow_2",
			requestRaw:  []byte("GET /test2 HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			responseRaw: []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html><body>success_keyword_" + testUUID + "_extra</body></html>"),
			shouldMatch: true,
			description: "应该匹配的流量2",
		},
		{
			name:        "unmatched_flow_1",
			requestRaw:  []byte("GET /test3 HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			responseRaw: []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html><body>normal content</body></html>"),
			shouldMatch: false,
			description: "不应该匹配的流量1",
		},
		{
			name:        "unmatched_flow_2",
			requestRaw:  []byte("GET /test4 HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			responseRaw: []byte("HTTP/1.1 404 Not Found\r\nContent-Type: text/html\r\n\r\n<html><body>error page</body></html>"),
			shouldMatch: false,
			description: "不应该匹配的流量2",
		},
	}

	// 测试匹配器的三种动作模式
	testCases := []struct {
		name               string
		matcher            *ypb.HTTPResponseMatcher
		expectedMatchCount int
		expectedDiscard    bool
		description        string
	}{
		{
			name: "only_match_action",
			matcher: &ypb.HTTPResponseMatcher{
				MatcherType: "word",
				Scope:       "body",
				Condition:   "or",
				Group:       []string{"success_keyword_" + testUUID},
				HitColor:    "green",
				Action:      "",
			},
			expectedMatchCount: 2,
			expectedDiscard:    false,
			description:        "空动作：匹配并继续处理",
		},
		{
			name: "retain_action",
			matcher: &ypb.HTTPResponseMatcher{
				MatcherType: "word",
				Scope:       "body",
				Condition:   "or",
				Group:       []string{"success_keyword_" + testUUID},
				Action:      "retain", // 保留：继续处理流量
			},
			expectedMatchCount: 2,
			expectedDiscard:    false,
			description:        "保留动作：匹配并继续处理",
		},
		{
			name: "discard_action",
			matcher: &ypb.HTTPResponseMatcher{
				MatcherType: "word",
				Scope:       "body",
				Condition:   "or",
				Group:       []string{"success_keyword_" + testUUID},
				Action:      "discard", // 丢弃：跳过后续处理
			},
			expectedMatchCount: 2,
			expectedDiscard:    true,
			description:        "丢弃动作：匹配但跳过后续处理",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建测试HTTP Flows
			var flowIDs []int64
			for _, flowData := range testFlows {
				flow, err := yakit.CreateHTTPFlow(
					yakit.CreateHTTPFlowWithRequestRaw(flowData.requestRaw),
					yakit.CreateHTTPFlowWithResponseRaw(flowData.responseRaw),
					yakit.CreateHTTPFlowWithFromPlugin("测试匹配器动作_"+testUUID+"_"+flowData.name),
				)
				require.NoError(t, err)

				// 保存到数据库
				err = yakit.SaveHTTPFlow(consts.GetGormProjectDatabase(), flow)
				require.NoError(t, err)
				flowIDs = append(flowIDs, int64(flow.ID))
			}

			// 创建replacer规则，用于验证是否继续处理
			replacer := &ypb.MITMContentReplacer{
				Rule:              ".*",
				EnableForRequest:  true,
				EnableForResponse: true,
				EnableForHeader:   true,
				EnableForBody:     true,
				NoReplace:         true,
				ExtraTag: []string{
					"测试规则_" + testUUID,
				},
				Color: "yellow",
			}

			// 执行分析
			stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
				Source: &ypb.AnalyzedDataSource{
					SourceType: AnalyzeHTTPFlowSourceDatabase,
					HTTPFlowFilter: &ypb.QueryHTTPFlowRequest{
						IncludeId: flowIDs,
					},
				},
				Replacers: []*ypb.MITMContentReplacer{replacer},
				Matchers:  []*ypb.HTTPResponseMatcher{tc.matcher},
			})
			require.NoError(t, err)

			var streamResults []*ypb.AnalyzeHTTPFlowResponse
			var discardCount int64
			for {
				result, err := stream.Recv()
				if err != nil {
					break
				}
				streamResults = append(streamResults, result)

				// 检查是否有丢弃流量数的状态信息
				if result.GetExecResult() != nil && result.GetExecResult().GetMessage() != nil {
					var msg struct {
						Type    string `json:"type"`
						Content struct {
							Level string `json:"level"`
							Data  string `json:"data"`
						} `json:"content"`
					}

					if json.Unmarshal(result.GetExecResult().GetMessage(), &msg) == nil {
						if msg.Type == "log" && msg.Content.Level == "feature-status-card-data" {
							// 解析内层data字段
							var contentData struct {
								ID   string `json:"id"`
								Data string `json:"data"`
							}
							if json.Unmarshal([]byte(msg.Content.Data), &contentData) == nil {
								if contentData.ID == "丢弃流量数" {
									if count, err := strconv.ParseInt(contentData.Data, 10, 64); err == nil {
										discardCount = count
										t.Logf("Found discard count: %d", discardCount)
									}
								}
							}
						}
					}
				}
			}

			if tc.matcher.GetAction() == "" {
				// 仅匹配动作：通过color验证是否匹配成功
				replacerExecutedCount := 0
				for i, flowID := range flowIDs {
					flow, err := yakit.GetHTTPFlow(consts.GetGormProjectDatabase(), flowID)
					require.NoError(t, err)

					// 检查是否包含replacer的标签
					hasReplacerTag := flow.HasColor("YAKIT_COLOR_YELLOW")
					if hasReplacerTag {
						replacerExecutedCount++
					}

					// 调试replacer执行情况
					t.Logf("Flow %d replacer check: hasYellow=%v, tags=%s", i, hasReplacerTag, flow.Tags)
				}

				// 空动作：所有流量都执行replacer
				expectedReplacerCount := len(testFlows)
				require.Equal(t, expectedReplacerCount, replacerExecutedCount, "仅匹配模式下replacer应该执行: %s", tc.description)
			} else {
				// retain和discard动作：通过丢弃流量数来验证
				expectedDiscardCount := int64(0)
				if tc.expectedDiscard {
					// discard动作：匹配成功的流量被丢弃
					expectedDiscardCount = int64(tc.expectedMatchCount)
				} else {
					// retain动作：不匹配的流量被丢弃
					expectedDiscardCount = int64(len(testFlows) - tc.expectedMatchCount)
				}
				t.Logf("Expected discard count: %d, actual discard count: %d", expectedDiscardCount, discardCount)
				require.Equal(t, expectedDiscardCount, discardCount, "丢弃流量数不符合预期: %s", tc.description)
			}

			// 清理测试数据
			for _, flowID := range flowIDs {
				err := yakit.DeleteHTTPFlowByID(consts.GetGormProjectDatabase(), flowID)
				require.NoError(t, err)
			}
		})
	}
}
