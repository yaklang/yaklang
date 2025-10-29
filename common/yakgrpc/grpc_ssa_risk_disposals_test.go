package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func prepareTestData(t *testing.T) (client ypb.YakClient, taskId string, riskIds []int64) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	titleId := uuid.NewString()
	taskId = uuid.NewString()

	// create risk
	createRisk := func(filePath, severity, riskType string) int64 {
		risk := &schema.SSARisk{
			CodeSourceUrl: filePath,
			Severity:      schema.ValidSeverityType(severity),
			RiskType:      riskType,
			RuntimeId:     taskId,
			Title:         "Test Risk" + titleId,
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

func TestGRPCMUSTPASS_SSARiskDisposals_DeleteAndUpdateRiskStatus(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	taskId := uuid.NewString()
	testUUID := uuid.NewString()

	// 创建测试用的风险记录
	createRisk := func(filePath, severity, riskType string) int64 {
		risk := &schema.SSARisk{
			CodeSourceUrl: filePath,
			Severity:      schema.ValidSeverityType(severity),
			RiskType:      riskType,
			RuntimeId:     taskId,
			Title:         "Test Risk - " + testUUID,
			TitleVerbose:  "Test Risk Verbose",
			ProgramName:   "TestProgram",
		}
		err := yakit.CreateSSARisk(ssadb.GetDB(), risk)
		require.NoError(t, err)
		return int64(risk.ID)
	}

	// 创建风险记录
	riskId := createRisk("ssadb://prog1/1", "high", "sql-injection")

	// 清理测试数据
	defer func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskId},
		})
		yakit.DeleteSSARiskDisposals(ssadb.GetDB(), &ypb.DeleteSSARiskDisposalsRequest{
			Filter: &ypb.SSARiskDisposalsFilter{
				RiskId: []int64{riskId},
			},
		})
	}()

	ctx := context.Background()

	// 第一步：验证初始状态为not_set
	t.Run("验证初始状态为not_set", func(t *testing.T) {
		queryResp, err := client.QuerySSARisks(ctx, &ypb.QuerySSARisksRequest{
			Filter: &ypb.SSARisksFilter{
				ID: []int64{riskId},
			},
		})
		require.NoError(t, err)
		require.Len(t, queryResp.Data, 1)
		require.Equal(t, "not_set", queryResp.Data[0].LatestDisposalStatus)
	})

	// 第二步：创建处置记录
	var disposalId int64
	t.Run("创建处置记录", func(t *testing.T) {
		createResp, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
			RiskIds: []int64{riskId},
			Status:  "is_issue",
			Comment: "确认为安全问题-" + testUUID,
		})
		require.NoError(t, err)
		require.Len(t, createResp.Data, 1)
		disposalId = createResp.Data[0].Id
		require.NotZero(t, disposalId)
	})

	// 第三步：验证处置记录创建后，风险状态更新
	t.Run("验证处置记录创建后风险状态更新", func(t *testing.T) {
		queryResp, err := client.QuerySSARisks(ctx, &ypb.QuerySSARisksRequest{
			Filter: &ypb.SSARisksFilter{
				ID: []int64{riskId},
			},
		})
		require.NoError(t, err)
		require.Len(t, queryResp.Data, 1)
		require.Equal(t, "is_issue", queryResp.Data[0].LatestDisposalStatus)
	})

	// 第四步：删除处置记录
	t.Run("删除处置记录", func(t *testing.T) {
		deleteResp, err := client.DeleteSSARiskDisposals(ctx, &ypb.DeleteSSARiskDisposalsRequest{
			Filter: &ypb.SSARiskDisposalsFilter{
				ID: []int64{disposalId},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, deleteResp)
		require.NotNil(t, deleteResp.Message)
		require.Equal(t, int64(1), deleteResp.Message.EffectRows)
	})

	// 第五步：验证删除处置记录后，风险状态重置为not_set
	t.Run("验证删除处置记录后风险状态重置为not_set", func(t *testing.T) {
		queryResp, err := client.QuerySSARisks(ctx, &ypb.QuerySSARisksRequest{
			Filter: &ypb.SSARisksFilter{
				ID: []int64{riskId},
			},
		})
		require.NoError(t, err)
		require.Len(t, queryResp.Data, 1)
		require.Equal(t, "not_set", queryResp.Data[0].LatestDisposalStatus)
	})

	// 第六步：验证处置记录确实已被删除
	t.Run("验证处置记录确实已被删除", func(t *testing.T) {
		queryResp, err := client.QuerySSARiskDisposals(ctx, &ypb.QuerySSARiskDisposalsRequest{
			Filter: &ypb.SSARiskDisposalsFilter{
				ID: []int64{disposalId},
			},
		})
		require.NoError(t, err)
		require.Len(t, queryResp.Data, 0)
	})
}

func TestGRPCMUSTPASS_SSARiskDisposal_InheritanceFeature(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	testCode := `
a = source() 
sink(a)
	`
	risk := uuid.NewString()
	testRule := fmt.Sprintf(`
sink as $sink
$sink #-> as $result
alert $result for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	level:"high"
	risk:"%s"
}
	`, risk)

	// 执行两次独立的扫描，生成具有相同 RiskFeatureHash 的风险
	riskCount := 2
	risks := make([]*schema.SSARisk, riskCount)
	programName := "inheritance_test_" + uuid.NewString() // 使用相同的程序名，这样会有不同的批次号

	for i := 0; i < riskCount; i++ {

		// 使用现有的扫描模式，创建程序并扫描
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.yak", testCode)

		programs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramName(programName))
		require.NoError(t, err)
		require.NotEmpty(t, programs)

		// 使用 gRPC 调用进行扫描，这样会自动产生扫描批次
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		// 发送开始扫描请求
		err = stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			ProgramName: []string{programName},
			RuleInput: &ypb.SyntaxFlowRuleInput{
				Content:  testRule,
				Language: "yak",
			},
		})
		require.NoError(t, err)

		// 等待扫描完成
		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			if resp.GetStatus() == "finished" || resp.GetStatus() == "error" {
				break
			}
		}

		// 查询生成的 Risk
		_, queryRisk, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		}, nil)
		require.NoError(t, err)
		require.NotEmpty(t, queryRisk)
		risks[i] = queryRisk[0]

		// 添加延迟确保下次扫描的时间戳不同
		time.Sleep(1 * time.Second)
	}

	// 清理测试数据
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		})
	}()

	require.Len(t, risks, riskCount)
	// 验证 RiskFeatureHash 相同
	require.Equal(t, risks[0].RiskFeatureHash, risks[1].RiskFeatureHash, "RiskFeatureHash 应该相同")
	require.NotEmpty(t, risks[0].RiskFeatureHash, "RiskFeatureHash 不应该为空")
	require.NotEmpty(t, risks[1].RiskFeatureHash, "RiskFeatureHash 不应该为空")

	// 添加调试信息
	t.Logf("Risk1: ID=%d, RiskFeatureHash=%s, RuntimeId=%s", risks[0].ID, risks[0].RiskFeatureHash, risks[0].RuntimeId)
	t.Logf("Risk2: ID=%d, RiskFeatureHash=%s, RuntimeId=%s", risks[1].ID, risks[1].RiskFeatureHash, risks[1].RuntimeId)

	ctx := context.Background()
	testUUID := uuid.NewString()

	// 在所有子测试完成后统一清理处置记录
	defer func() {
		// 清理所有相关的处置记录
		yakit.DeleteSSARiskDisposals(ssadb.GetDB(), &ypb.DeleteSSARiskDisposalsRequest{
			Filter: &ypb.SSARiskDisposalsFilter{
				Search: testUUID,
			},
		})
	}()

	// 为第一个 Risk 创建处置信息
	t.Run("为第一个Risk创建处置信息", func(t *testing.T) {
		createResp, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
			RiskIds: []int64{int64(risks[0].ID)},
			Status:  "not_issue",
			Comment: "第一次扫描的处置-" + testUUID,
		})
		require.NoError(t, err)
		require.Len(t, createResp.Data, 1)
		require.Equal(t, int64(risks[0].ID), createResp.Data[0].RiskId)
	})

	// 等待数据库更新
	time.Sleep(100 * time.Millisecond)

	// 为第二个 Risk 创建处置信息
	t.Run("为第二个Risk创建处置信息", func(t *testing.T) {
		createResp, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
			RiskIds: []int64{int64(risks[1].ID)},
			Status:  "is_issue",
			Comment: "第二次扫描的处置-" + testUUID,
		})
		require.NoError(t, err)
		require.Len(t, createResp.Data, 1)
		require.Equal(t, int64(risks[1].ID), createResp.Data[0].RiskId)
	})

	// 等待数据库更新
	time.Sleep(100 * time.Millisecond)

	// 测试第一个 Risk 的继承查询
	t.Run("测试第一个Risk的继承查询", func(t *testing.T) {
		getResp, err := client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
			RiskId: int64(risks[0].ID),
		})
		require.NoError(t, err)
		require.NotNil(t, getResp)
		// 只能搜到第一次扫描的处置记录
		t.Logf("=== 第一个Risk(ID=%d)的查询结果 ===", risks[1].ID)
		t.Logf("查询到 %d 条处置记录:", len(getResp.Data))
		for i, disposal := range getResp.Data {
			t.Logf("  [%d] DisposalID=%d, RiskID=%d, Status=%s, TaskName=%s, Comment=%s, CreatedAt=%d",
				i+1, disposal.Id, disposal.RiskId, disposal.Status, disposal.TaskName, disposal.Comment, disposal.CreatedAt)
		}

		require.Equal(t, len(getResp.Data), 1)
		require.Equal(t, getResp.Data[0].Status, "not_issue")
		require.Equal(t, getResp.Data[0].Comment, "第一次扫描的处置-"+testUUID)
	})

	// 测试第二个 Risk 的继承查询
	t.Run("测试第二个Risk的继承查询", func(t *testing.T) {
		getResp, err := client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
			RiskId: int64(risks[1].ID),
		})
		require.NoError(t, err)
		require.NotNil(t, getResp)
		t.Logf("=== 第二个Risk(ID=%d)的查询结果 ===", risks[0].ID)
		t.Logf("查询到 %d 条处置记录:", len(getResp.Data))
		for i, disposal := range getResp.Data {
			t.Logf("  [%d] DisposalID=%d, RiskID=%d, TaskName =%s,Status=%s, Comment=%s, CreatedAt=%d",
				i+1, disposal.Id, disposal.RiskId, disposal.TaskName, disposal.Status, disposal.Comment, disposal.CreatedAt)
		}

		// 新扫描的项目处置信息会排在前面
		rspData := getResp.Data
		require.Equal(t, len(rspData), 2)
		require.Equal(t, rspData[0].Status, "is_issue", "应该包含 not_issue 状态")
		require.Equal(t, rspData[1].Status, "not_issue", "应该包含 is_issue 状态")
		require.Equal(t, rspData[0].Comment, "第二次扫描的处置-"+testUUID, "应该包含第二次扫描的备注")
		require.Equal(t, rspData[1].Comment, "第一次扫描的处置-"+testUUID, "应该包含第一次扫描的备注")
		// 验证TaskName格式（应该是"程序名_批次X"的格式）
		require.Contains(t, rspData[0].TaskName, "批次2", "TaskName应该包含批次信息")
		require.Contains(t, rspData[1].TaskName, "批次1", "TaskName应该包含批次信息")
		t.Logf("第一个Risk继承查询验证通过: 共 %d 条处置记录", len(getResp.Data))
	})
}

func TestGRPCMUSTPASS_SSARiskDisposal_ComplexInheritanceScenario(t *testing.T) {
	// 测试复杂场景
	// 	1.发起一次扫描A，扫描A产生Risk 11、Risk12
	//	 处置扫描A产生的漏洞，给Risk11 Risk 12都打上评论
	//	 > Risk11(新增) Risk12(新增)
	//	2. 发起第二次扫描B，扫描B产生Risk 21、Risk22、Risk23,其中Risk 21和Risk 22和扫描A的Risk 11、Risk 12为属于同一个风险。那么Risk 11和Risk 12会携带上Risk 11、Risk 12的审计信息。
	//	 处置B产生的漏洞，给Risk 22,  Risk 23打上评论
	//	 > Risk 21(携带) Risk 22（新增+携带） Risk 23（新增）
	//	3. 第三次扫描C,产生Risk 32、Risk 33
	//	 对比上次少了个Risk 21，Risk 32 Risk 33携带上次的评论
	//   Risk 31(无) Risk 32(携带) Risk 33（携带）
	client, err := NewLocalClient()
	require.NoError(t, err)

	// risk会用来计算riskFeatureHash，所以使用uuid做risk避免旧暑假影响
	uuidRisk := uuid.NewString()
	rule := fmt.Sprintf(`
sink1 as $sink1
alert $sink1 for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	risk:"%s"
}

sink2 as $sink2
alert $sink2 for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	risk:"%s"
}

sink3 as $sink3
alert $sink3 for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	risk:"%s"
}
		`, uuidRisk, uuidRisk, uuidRisk)

	ctx := context.Background()
	testUUID := uuid.NewString()

	programsToClean := make([]string, 0)
	// 在所有子测试完成后统一清理处置记录
	defer func() {
		yakit.DeleteSSARiskDisposals(ssadb.GetDB(), &ypb.DeleteSSARiskDisposalsRequest{
			Filter: &ypb.SSARiskDisposalsFilter{
				Search: testUUID,
			},
		})
		yakit.DeleteSSAProgram(ssadb.GetDB(), &ypb.SSAProgramFilter{ProgramNames: programsToClean})
	}()

	// === 第一次扫描 A ===
	t.Run("扫描A: 产生Risk11和Risk12", func(t *testing.T) {
		programName := "complex_test_scan_A_" + uuid.NewString()
		programsToClean = append(programsToClean, programName)

		testCode := `
sink1()  
sink2() 
`

		// 创建虚拟文件系统并解析项目
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.yak", testCode)

		programs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramName(programName))
		require.NoError(t, err)
		require.NotEmpty(t, programs)

		// 使用 gRPC 调用进行扫描，这样会自动产生扫描批次
		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		err = stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			ProgramName: []string{programName},
			RuleInput: &ypb.SyntaxFlowRuleInput{
				Content:  rule,
				Language: "yak",
			},
		})
		require.NoError(t, err)

		// 等待扫描完成
		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			if resp.GetStatus() == "finished" || resp.GetStatus() == "error" {
				break
			}
		}

		// 查询生成的 Risk
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		}, &ypb.Paging{OrderBy: "id", Order: "asc"})
		require.NoError(t, err)
		require.Len(t, risks, 2)

		t.Logf("=== 扫描A生成的Risk ===")
		for i, risk := range risks {
			t.Logf("Risk%d: ID=%d, Title=%s, RiskFeatureHash=%s",
				i+1, risk.ID, risk.Title, risk.RiskFeatureHash)
		}

		// 为 Risk11 和 Risk12 创建处置信息
		createResp, err := client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
			RiskIds: []int64{int64(risks[0].ID), int64(risks[1].ID)},
			Status:  "not_issue",
			Comment: "扫描A的处置-" + testUUID,
		})
		require.NoError(t, err)
		require.Len(t, createResp.Data, 2)

		time.Sleep(1 * time.Second)
	})

	// === 第二次扫描 B ===
	var scanBRisks []*schema.SSARisk
	t.Run("扫描B: 产生Risk21、Risk22、Risk23", func(t *testing.T) {
		programName := "complex_test_scan_B_" + uuid.NewString()
		programsToClean = append(programsToClean, programName)

		testCode := `
sink1()  
sink2()  
sink3()  
`

		vf := filesys.NewVirtualFs()
		vf.AddFile("test.yak", testCode)

		programs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramName(programName))
		require.NoError(t, err)
		require.NotEmpty(t, programs)

		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		err = stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			ProgramName: []string{programName},
			RuleInput: &ypb.SyntaxFlowRuleInput{
				Content:  rule,
				Language: "yak",
			},
		})
		require.NoError(t, err)

		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			if resp.GetStatus() == "finished" || resp.GetStatus() == "error" {
				break
			}
		}

		// 查询生成的 Risk
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		}, &ypb.Paging{OrderBy: "id", Order: "asc"})
		require.NoError(t, err)
		require.Len(t, risks, 3) // 应该有3个risk
		scanBRisks = risks

		t.Logf("=== 扫描B生成的Risk ===")
		for i, risk := range risks {
			t.Logf("Risk%d: ID=%d, Title=%s, RiskFeatureHash=%s",
				i+1, risk.ID, risk.Title, risk.RiskFeatureHash)
		}

		// 验证 Risk21 和 Risk22 继承了扫描A的处置信息
		for i := 0; i < 2; i++ {
			getResp, err := client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
				RiskId: int64(risks[i].ID),
			})
			require.NoError(t, err)
			require.NotNil(t, getResp)
			require.Len(t, getResp.Data, 1, "Risk2%d应该继承扫描A的处置信息", i+1)
			require.Equal(t, "not_issue", getResp.Data[0].Status)
			require.Equal(t, getResp.Data[0].Comment, "扫描A的处置-"+testUUID)
			t.Logf("Risk2%d 成功继承了扫描A的处置信息", i+1)
		}

		// Risk23 应该没有处置信息
		getResp, err := client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
			RiskId: int64(risks[2].ID),
		})
		require.NoError(t, err)
		require.Len(t, getResp.Data, 0, "Risk23应该没有处置信息")

		// 为 Risk22 和 Risk23 创建新的处置信息
		// Risk22 修改为新的评论
		_, err = client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
			RiskIds: []int64{int64(risks[1].ID)},
			Status:  "is_issue",
			Comment: "扫描B-Risk22的新处置-" + testUUID,
		})
		require.NoError(t, err)

		// Risk23 新增评论
		_, err = client.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{
			RiskIds: []int64{int64(risks[2].ID)},
			Status:  "suspicious",
			Comment: "扫描B-Risk23的处置-" + testUUID,
		})
		require.NoError(t, err)

		time.Sleep(1 * time.Second)
	})

	// 验证扫描B的处置状态
	t.Run("验证扫描B的处置状态", func(t *testing.T) {
		// Risk21: 只有继承的评论
		getResp, err := client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
			RiskId: int64(scanBRisks[0].ID),
		})
		require.NoError(t, err)
		require.Len(t, getResp.Data, 1)
		require.Equal(t, "not_issue", getResp.Data[0].Status)
		require.Contains(t, getResp.Data[0].Comment, "扫描A的处置-"+testUUID)

		// Risk22: 应该有两条记录，新的在前面
		getResp, err = client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
			RiskId: int64(scanBRisks[1].ID),
		})
		require.NoError(t, err)
		require.Len(t, getResp.Data, 2)
		require.Equal(t, "is_issue", getResp.Data[0].Status)
		require.Contains(t, getResp.Data[0].Comment, "扫描B-Risk22的新处置-"+testUUID)
		require.Equal(t, "not_issue", getResp.Data[1].Status)
		require.Contains(t, getResp.Data[1].Comment, "扫描A的处置-"+testUUID)

		// Risk23: 只有新的评论
		getResp, err = client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
			RiskId: int64(scanBRisks[2].ID),
		})
		require.NoError(t, err)
		require.Len(t, getResp.Data, 1)
		require.Equal(t, "suspicious", getResp.Data[0].Status)
		require.Contains(t, getResp.Data[0].Comment, "扫描B-Risk23的处置-"+testUUID)
	})

	// === 第三次扫描 C ===
	t.Run("扫描C: 产生Risk32、Risk33，缺少Risk21", func(t *testing.T) {
		programName := "complex_test_scan_C_" + uuid.NewString()
		programsToClean = append(programsToClean, programName)

		testCode := `
sink2()  
sink3()  
`

		vf := filesys.NewVirtualFs()
		vf.AddFile("test.yak", testCode)

		programs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.Yak), ssaapi.WithProgramName(programName))
		require.NoError(t, err)
		require.NotEmpty(t, programs)

		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		err = stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			ProgramName: []string{programName},
			RuleInput: &ypb.SyntaxFlowRuleInput{
				Content:  rule,
				Language: "yak",
			},
		})
		require.NoError(t, err)

		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			if resp.GetStatus() == "finished" || resp.GetStatus() == "error" {
				break
			}
		}

		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		}, &ypb.Paging{OrderBy: "id", Order: "asc"})
		require.NoError(t, err)
		require.Len(t, risks, 2) // 应该有2个risk (Risk32, Risk33)

		t.Logf("=== 扫描C生成的Risk ===")
		for i, risk := range risks {
			t.Logf("Risk3%d: ID=%d, Title=%s, RiskFeatureHash=%s",
				i+2, risk.ID, risk.Title, risk.RiskFeatureHash)
		}

		// 验证 Risk32 和 Risk33 继承了扫描B的处置信息
		// Risk32 (对应之前的Risk22): 应该继承最新的处置信息
		getResp, err := client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
			RiskId: int64(risks[0].ID),
		})
		require.NoError(t, err)
		require.NotNil(t, getResp)
		require.Len(t, getResp.Data, 2, "Risk32应该继承扫描B的所有处置信息")
		require.Equal(t, "is_issue", getResp.Data[0].Status)
		require.Contains(t, getResp.Data[0].Comment, "扫描B-Risk22的新处置-"+testUUID)
		t.Logf("Risk32 成功继承了扫描B的处置信息")

		// Risk33 (对应之前的Risk23): 应该继承扫描B的处置信息
		getResp, err = client.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{
			RiskId: int64(risks[1].ID),
		})
		require.NoError(t, err)
		require.Len(t, getResp.Data, 1, "Risk33应该继承扫描B的处置信息")
		require.Equal(t, "suspicious", getResp.Data[0].Status)
		require.Contains(t, getResp.Data[0].Comment, "扫描B-Risk23的处置-"+testUUID)
		t.Logf("Risk33 成功继承了扫描B的处置信息")
	})
}
