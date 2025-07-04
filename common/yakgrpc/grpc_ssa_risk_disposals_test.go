package yakgrpc

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func prepareTestData(t *testing.T) (client ypb.YakClient, taskId string, riskIds []int64) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	taskId = uuid.NewString()

	// create risk
	createRisk := func(filePath, severity, riskType string) int64 {
		risk := &schema.SSARisk{
			CodeSourceUrl: filePath,
			Severity:      schema.ValidSeverityType(severity),
			RiskType:      riskType,
			RuntimeId:     taskId,
			Title:         "Test Risk",
			TitleVerbose:  "Test Risk Verbose",
			ProgramName:   "TestProgram",
		}
		err := yakit.CreateSSARisk(ssadb.GetDB(), risk)
		require.NoError(t, err)
		return int64(risk.ID)
	}

	riskIds = []int64{
		createRisk("ssadb://prog1/1", "high", "sql-injection"),
		createRisk("ssadb://prog1/2", "medium", "xss"),
		createRisk("ssadb://prog2/1", "low", "path-traversal"),
		createRisk("ssadb://prog2/2", "high", "code-injection"),
		createRisk("ssadb://prog3/1", "medium", "weak-crypto"),
	}

	return client, taskId, riskIds
}

func TestGRPCMUSTPASS_SSARiskDisposals_CreateAndQuery(t *testing.T) {
	client, _, riskIds := prepareTestData(t)

	testUUID := uuid.NewString()

	tests := []struct {
		name            string
		createRequest   *ypb.CreateSSARiskDisposalsRequest
		expectError     bool
		expectedCount   int
		expectedComment string
	}{
		{
			name: "创建单个风险处置",
			createRequest: &ypb.CreateSSARiskDisposalsRequest{
				RiskIds: []int64{riskIds[0]},
				Status:  "is_issue",
				Comment: "确认是安全问题-" + testUUID + "-single",
			},
			expectError:     false,
			expectedCount:   1,
			expectedComment: "确认是安全问题-" + testUUID + "-single",
		},
		{
			name: "创建多个风险处置",
			createRequest: &ypb.CreateSSARiskDisposalsRequest{
				RiskIds: riskIds[1:3],
				Status:  "not_issue",
				Comment: "误报已确认-" + testUUID + "-multiple",
			},
			expectError:     false,
			expectedCount:   2,
			expectedComment: "误报已确认-" + testUUID + "-multiple",
		},
		{
			name: "创建风险处置-带特殊字符",
			createRequest: &ypb.CreateSSARiskDisposalsRequest{
				RiskIds: []int64{riskIds[3]},
				Status:  "suspicious",
				Comment: "需要进一步分析@#$%^&*()-" + testUUID + "-special",
			},
			expectError:     false,
			expectedCount:   1,
			expectedComment: "需要进一步分析@#$%^&*()-" + testUUID + "-special",
		},
		{
			name: "空RiskIds应该失败",
			createRequest: &ypb.CreateSSARiskDisposalsRequest{
				RiskIds: []int64{},
				Status:  "is_issue",
				Comment: "测试空RiskIds-" + testUUID,
			},
			expectError:   true,
			expectedCount: 0,
		},
		{
			name:          "nil请求应该失败",
			createRequest: nil,
			expectError:   true,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			createResp, err := client.CreateSSARiskDisposals(ctx, tt.createRequest)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, createResp)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, createResp)
			require.Len(t, createResp.Data, tt.expectedCount)

			if tt.expectedCount > 0 {
				for _, data := range createResp.Data {
					require.Equal(t, tt.expectedComment, data.Comment)
					require.Contains(t, tt.createRequest.RiskIds, data.RiskId)
					require.NotZero(t, data.Id)
					require.NotZero(t, data.CreatedAt)
				}
			}
		})
	}
}

func TestGRPCMUSTPASS_SSARiskDisposals_Query(t *testing.T) {
	client, _, riskIds := prepareTestData(t)

	// 为查询测试生成唯一标识
	queryTestUUID := uuid.NewString()

	// 先创建一些测试数据
	createData := []struct {
		riskIds []int64
		status  string
		comment string
	}{
		{[]int64{riskIds[0]}, "is_issue", "确认问题-" + queryTestUUID + "-1"},
		{[]int64{riskIds[1]}, "not_issue", "误报-" + queryTestUUID + "-2"},
		{[]int64{riskIds[2]}, "suspicious", "可疑-" + queryTestUUID + "-3"},
		{[]int64{riskIds[3], riskIds[4]}, "is_issue", "批量确认-" + queryTestUUID + "-4"},
	}

	ctx := context.Background()
	for _, data := range createData {
		_, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
			RiskIds: data.riskIds,
			Status:  data.status,
			Comment: data.comment,
		})
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		queryRequest  *ypb.QuerySSARiskDisposalsRequest
		expectedCount int
		validateFunc  func(t *testing.T, data []*ypb.SSARiskDisposalData)
	}{
		{
			name: "搜索Comment中的特定内容",
			queryRequest: &ypb.QuerySSARiskDisposalsRequest{
				Filter: &ypb.SSARiskDisposalsFilter{
					Search: queryTestUUID + "-1",
				},
			},
			expectedCount: 1,
			validateFunc: func(t *testing.T, data []*ypb.SSARiskDisposalData) {
				require.Contains(t, data[0].Comment, queryTestUUID+"-1")
			},
		},
		{
			name: "按RiskId过滤",
			queryRequest: &ypb.QuerySSARiskDisposalsRequest{
				Filter: &ypb.SSARiskDisposalsFilter{
					RiskId: []int64{riskIds[0], riskIds[1]},
				},
			},
			expectedCount: 2,
			validateFunc: func(t *testing.T, data []*ypb.SSARiskDisposalData) {
				for _, item := range data {
					require.Contains(t, []int64{riskIds[0], riskIds[1]}, item.RiskId)
				}
			},
		},
		{
			name: "搜索关键词",
			queryRequest: &ypb.QuerySSARiskDisposalsRequest{
				Filter: &ypb.SSARiskDisposalsFilter{
					Search: "误报-" + queryTestUUID,
				},
			},
			expectedCount: 1,
			validateFunc: func(t *testing.T, data []*ypb.SSARiskDisposalData) {
				require.Contains(t, data[0].Comment, "误报-"+queryTestUUID)
			},
		},
		{
			name: "分页测试-指定限制条数",
			queryRequest: &ypb.QuerySSARiskDisposalsRequest{
				Pagination: &ypb.Paging{
					Page:  1,
					Limit: 2,
				},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queryResp, err := client.QuerySSARiskDisposals(ctx, tt.queryRequest)

			require.NoError(t, err)
			require.NotNil(t, queryResp)
			require.Len(t, queryResp.Data, tt.expectedCount)

			if tt.validateFunc != nil {
				tt.validateFunc(t, queryResp.Data)
			}
		})
	}
}

func TestGRPCMUSTPASS_SSARiskDisposals_Update(t *testing.T) {
	client, _, riskIds := prepareTestData(t)

	updateTestUUID := uuid.NewString()

	ctx := context.Background()
	createResp, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
		RiskIds: riskIds[:3],
		Status:  "not_set",
		Comment: "初始状态-" + updateTestUUID,
	})
	require.NoError(t, err)
	require.Len(t, createResp.Data, 3)

	disposalIds := make([]int64, len(createResp.Data))
	for i, data := range createResp.Data {
		disposalIds[i] = data.Id
	}

	tests := []struct {
		name          string
		updateRequest *ypb.UpdateSSARiskDisposalsRequest
		expectedCount int
		validateFunc  func(t *testing.T, data []*ypb.SSARiskDisposalData)
	}{
		{
			name: "按ID更新comment",
			updateRequest: &ypb.UpdateSSARiskDisposalsRequest{
				Filter: &ypb.SSARiskDisposalsFilter{
					ID: disposalIds[:2],
				},
				Status:  "is_issue",
				Comment: "更新为确认问题-" + updateTestUUID,
			},
			expectedCount: 2,
			validateFunc: func(t *testing.T, data []*ypb.SSARiskDisposalData) {
				for _, item := range data {
					require.Equal(t, "更新为确认问题-"+updateTestUUID, item.Comment)
				}
			},
		},
		{
			name: "按RiskId更新comment",
			updateRequest: &ypb.UpdateSSARiskDisposalsRequest{
				Filter: &ypb.SSARiskDisposalsFilter{
					RiskId: []int64{riskIds[2]},
				},
				Status:  "not_issue",
				Comment: "更新为误报-" + updateTestUUID,
			},
			expectedCount: 1,
			validateFunc: func(t *testing.T, data []*ypb.SSARiskDisposalData) {
				require.Equal(t, "更新为误报-"+updateTestUUID, data[0].Comment)
				require.Equal(t, riskIds[2], data[0].RiskId)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateResp, err := client.UpdateSSARiskDisposals(ctx, tt.updateRequest)

			require.NoError(t, err)
			require.NotNil(t, updateResp)
			require.Len(t, updateResp.Data, tt.expectedCount)

			if tt.validateFunc != nil {
				tt.validateFunc(t, updateResp.Data)
			}
		})
	}
}

func TestGRPCMUSTPASS_SSARiskDisposals_Delete(t *testing.T) {
	client, _, riskIds := prepareTestData(t)

	deleteTestUUID := uuid.NewString()

	ctx := context.Background()
	createResp, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
		RiskIds: riskIds[:4],
		Status:  "not_set",
		Comment: "待删除的记录-" + deleteTestUUID,
	})
	require.NoError(t, err)
	require.Len(t, createResp.Data, 4)

	disposalIds := make([]int64, len(createResp.Data))
	for i, data := range createResp.Data {
		disposalIds[i] = data.Id
	}

	tests := []struct {
		name          string
		deleteRequest *ypb.DeleteSSARiskDisposalsRequest
		expectedCount int64
	}{
		{
			name: "按ID删除处置记录",
			deleteRequest: &ypb.DeleteSSARiskDisposalsRequest{
				Filter: &ypb.SSARiskDisposalsFilter{
					ID: disposalIds[:2],
				},
			},
			expectedCount: 2,
		},
		{
			name: "按RiskId删除",
			deleteRequest: &ypb.DeleteSSARiskDisposalsRequest{
				Filter: &ypb.SSARiskDisposalsFilter{
					RiskId: []int64{riskIds[2]},
				},
			},
			expectedCount: 1,
		},
		{
			name: "按状态删除",
			deleteRequest: &ypb.DeleteSSARiskDisposalsRequest{
				Filter: &ypb.SSARiskDisposalsFilter{
					Status: []string{"not_set"},
				},
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleteResp, err := client.DeleteSSARiskDisposals(ctx, tt.deleteRequest)
			require.NoError(t, err)
			require.NotNil(t, deleteResp)
			require.NotNil(t, deleteResp.Message)
			require.Equal(t, tt.expectedCount, deleteResp.Message.EffectRows)
			require.Equal(t, "ssa_risk_disposals", deleteResp.Message.TableName)
		})
	}
}

func TestGRPCMUSTPASS_SSARiskDisposals_Get(t *testing.T) {
	client, _, riskIds := prepareTestData(t)

	getTestUUID := uuid.NewString()

	ctx := context.Background()
	createResp, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
		RiskIds: []int64{riskIds[0], riskIds[1]},
		Status:  "is_issue",
		Comment: "测试获取记录-" + getTestUUID + "-first",
	})
	require.NoError(t, err)
	require.Len(t, createResp.Data, 2)

	// 为同一个risk创建多条记录
	_, err = client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
		RiskIds: []int64{riskIds[0]},
		Status:  "not_issue",
		Comment: "第二条记录-" + getTestUUID + "-second",
	})
	require.NoError(t, err)

	tests := []struct {
		name         string
		getRequest   *ypb.GetSSARiskDisposalRequest
		expectError  bool
		expectedLen  int
		validateFunc func(t *testing.T, data []*ypb.SSARiskDisposalData)
	}{
		{
			name: "获取单个风险的处置记录",
			getRequest: &ypb.GetSSARiskDisposalRequest{
				RiskId: riskIds[1],
			},
			expectError: false,
			expectedLen: 1,
			validateFunc: func(t *testing.T, data []*ypb.SSARiskDisposalData) {
				require.Equal(t, riskIds[1], data[0].RiskId)
				require.Contains(t, data[0].Comment, getTestUUID+"-first")
			},
		},
		{
			name: "获取有多条记录的风险",
			getRequest: &ypb.GetSSARiskDisposalRequest{
				RiskId: riskIds[0],
			},
			expectError: false,
			expectedLen: 2,
			validateFunc: func(t *testing.T, data []*ypb.SSARiskDisposalData) {
				for _, item := range data {
					require.Equal(t, riskIds[0], item.RiskId)
				}
				// 检查两条记录的comment不同
				comments := make([]string, len(data))
				for i, item := range data {
					comments[i] = item.Comment
				}
				hasFirst := false
				hasSecond := false
				for _, comment := range comments {
					if strings.Contains(comment, getTestUUID+"-first") {
						hasFirst = true
					}
					if strings.Contains(comment, getTestUUID+"-second") {
						hasSecond = true
					}
				}
				require.True(t, hasFirst, "应该包含first记录")
				require.True(t, hasSecond, "应该包含second记录")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getResp, err := client.GetSSARiskDisposal(ctx, tt.getRequest)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, getResp)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, getResp)
			require.Len(t, getResp.Data, tt.expectedLen)

			if tt.validateFunc != nil {
				tt.validateFunc(t, getResp.Data)
			}
		})
	}
}

func TestGRPCMUSTPASS_SSARiskDisposals_FullWorkflow(t *testing.T) {
	client, _, riskIds := prepareTestData(t)
	ctx := context.Background()

	workflowTestUUID := uuid.NewString()

	// 1. 创建处置记录
	createResp, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
		RiskIds: []int64{riskIds[0]},
		Status:  "not_set",
		Comment: "初始状态-" + workflowTestUUID,
	})
	require.NoError(t, err)
	require.Len(t, createResp.Data, 1)
	disposalId := createResp.Data[0].Id

	// 2. 查询验证创建成功
	queryResp, err := client.QuerySSARiskDisposals(ctx, &ypb.QuerySSARiskDisposalsRequest{
		Filter: &ypb.SSARiskDisposalsFilter{
			ID: []int64{disposalId},
		},
	})
	require.NoError(t, err)
	require.Len(t, queryResp.Data, 1)
	require.Contains(t, queryResp.Data[0].Comment, "初始状态-"+workflowTestUUID)

	// 3. 更新状态
	updateResp, err := client.UpdateSSARiskDisposals(ctx, &ypb.UpdateSSARiskDisposalsRequest{
		Filter: &ypb.SSARiskDisposalsFilter{
			ID: []int64{disposalId},
		},
		Status:  "is_issue",
		Comment: "确认为安全问题-" + workflowTestUUID,
	})
	require.NoError(t, err)
	require.Len(t, updateResp.Data, 1)
	require.Contains(t, updateResp.Data[0].Comment, "确认为安全问题-"+workflowTestUUID)

	// 4. 通过GetSSARiskDisposal验证更新
	getResp, err := client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
		RiskId: riskIds[0],
	})
	require.NoError(t, err)
	require.Len(t, getResp.Data, 1)
	require.Contains(t, getResp.Data[0].Comment, "确认为安全问题-"+workflowTestUUID)

	// 5. 删除记录
	deleteResp, err := client.DeleteSSARiskDisposals(ctx, &ypb.DeleteSSARiskDisposalsRequest{
		Filter: &ypb.SSARiskDisposalsFilter{
			ID: []int64{disposalId},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), deleteResp.Message.EffectRows)

	// 6. 验证删除成功
	queryResp2, err := client.QuerySSARiskDisposals(ctx, &ypb.QuerySSARiskDisposalsRequest{
		Filter: &ypb.SSARiskDisposalsFilter{
			ID: []int64{disposalId},
		},
	})
	require.NoError(t, err)
	require.Len(t, queryResp2.Data, 0)
}
