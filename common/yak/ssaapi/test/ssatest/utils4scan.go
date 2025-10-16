package ssatest

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SFScanRiskTestSuite 是一个测试套件结构体，用于管理 SyntaxFlow 扫描风险测试的完整生命周期
type SFScanRiskTestSuite struct {
	t           *testing.T
	Client      ypb.YakClient
	ProgramName string
	TestRisks   []SFScanTestRisk
	TaskIDs     []string
	Language    consts.Language
}

// SFScanTestRisk 定义测试风险的结构
type SFScanTestRisk struct {
	ID       string
	Title    string
	SinkName string
}

// SFScanRiskData 定义URL查询返回的风险数据结构
type SFScanRiskData struct {
	Name  string
	Type  string
	Count int
}

func NewSFScanRiskTestSuite(t *testing.T, client ypb.YakClient, programName string, language consts.Language) (*SFScanRiskTestSuite, func()) {
	suite := &SFScanRiskTestSuite{
		t:           t,
		Client:      client,
		ProgramName: programName,
		TestRisks:   make([]SFScanTestRisk, 0),
		TaskIDs:     make([]string, 0),
		Language:    language,
	}
	return suite, func() {
		suite.Cleanup()
	}
}

func NewSFScanTestRisk(id, title, sinkName string) SFScanTestRisk {
	return SFScanTestRisk{
		ID:       id,
		Title:    title,
		SinkName: sinkName,
	}
}
func (suite *SFScanRiskTestSuite) InitSimpleProgram(code string, riskConfigs ...SFScanTestRisk) *SFScanRiskTestSuite {
	suite.TestRisks = riskConfigs
	vf := filesys.NewVirtualFs()
	vf.AddFile("test"+suite.Language.GetFileExt(), code)
	programs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(suite.Language),
		ssaapi.WithProgramName(suite.ProgramName),
		ssaapi.WithReCompile(len(suite.TaskIDs) > 0),
	)
	require.NoError(suite.t, err)
	require.NotEmpty(suite.t, programs)
	suite.t.Logf("已初始化程序 %s，包含 %d 个风险配置", suite.ProgramName, len(riskConfigs))
	return suite
}

func (suite *SFScanRiskTestSuite) InitProgram(fs fi.FileSystem, riskConfigs ...SFScanTestRisk) *SFScanRiskTestSuite {
	suite.TestRisks = riskConfigs
	programs, err := ssaapi.ParseProjectWithFS(
		fs,
		ssaapi.WithLanguage(suite.Language),
		ssaapi.WithProgramName(suite.ProgramName),
		ssaapi.WithReCompile(len(suite.TaskIDs) > 0),
	)
	require.NoError(suite.t, err)
	require.NotEmpty(suite.t, programs)
	suite.t.Logf("已初始化程序 %s，包含 %d 个风险配置", suite.ProgramName, len(riskConfigs))
	return suite
}

func (suite *SFScanRiskTestSuite) Scan() *SFScanRiskTestSuite {
	rule := suite.buildTestRule()

	stream, err := suite.Client.SyntaxFlowScan(context.Background())
	require.NoError(suite.t, err)
	err = stream.Send(&ypb.SyntaxFlowScanRequest{
		ControlMode: "start",
		ProgramName: []string{suite.ProgramName},
		RuleInput: &ypb.SyntaxFlowRuleInput{
			Content:  rule,
			Language: string(suite.Language),
		},
	})
	require.NoError(suite.t, err)

	// 等待扫描完成
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(suite.t, err)
		}
		if resp.GetStatus() == "finished" || resp.GetStatus() == "error" {
			break
		}
	}

	// 获取最新扫描的TaskID
	taskID := suite.getLatestTaskID()
	suite.TaskIDs = append(suite.TaskIDs, taskID)
	suite.t.Logf("扫描完成，TaskID: %s，当前总任务数: %d", taskID, len(suite.TaskIDs))
	return suite
}

func (suite *SFScanRiskTestSuite) ScanWithRule(rule string) *SFScanRiskTestSuite {
	stream, err := suite.Client.SyntaxFlowScan(context.Background())
	require.NoError(suite.t, err)
	err = stream.Send(&ypb.SyntaxFlowScanRequest{
		ControlMode: "start",
		ProgramName: []string{suite.ProgramName},
		RuleInput: &ypb.SyntaxFlowRuleInput{
			Content:  rule,
			Language: string(suite.Language),
		},
	})
	require.NoError(suite.t, err)

	// 等待扫描完成
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(suite.t, err)
		}
		if resp.GetStatus() == "finished" || resp.GetStatus() == "error" {
			break
		}
	}

	// 获取最新扫描的TaskID
	taskID := suite.getLatestTaskID()
	suite.TaskIDs = append(suite.TaskIDs, taskID)
	suite.t.Logf("扫描完成，TaskID: %s，当前总任务数: %d", taskID, len(suite.TaskIDs))
	return suite
}

func (suite *SFScanRiskTestSuite) Disposal(riskTitle, status, comment string) *SFScanRiskTestSuite {
	_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{suite.ProgramName},
		Search:      riskTitle,
	}, nil)
	require.NoError(suite.t, err)
	require.NotEmpty(suite.t, risks, "未找到标题为 %s 的风险", riskTitle)

	var targetRisk *schema.SSARisk
	for _, risk := range risks {
		if risk.Title == riskTitle {
			targetRisk = risk
			break
		}
	}
	require.NotNil(suite.t, targetRisk, "未找到标题为 %s 的风险", riskTitle)

	_, err = suite.Client.CreateSSARiskDisposals(context.Background(), &ypb.CreateSSARiskDisposalsRequest{
		RiskIds: []int64{int64(targetRisk.ID)},
		Status:  status,
		Comment: comment,
	})
	require.NoError(suite.t, err)

	suite.t.Logf("已处置风险: %s (ID: %d, Hash: %s) - 状态: %s", riskTitle, targetRisk.ID, targetRisk.RiskFeatureHash, status)
	return suite
}

// checkRiskCount 检查风险数量
func (suite *SFScanRiskTestSuite) CheckRiskCount(t *testing.T, expectedCount int, taskIndex ...int) *SFScanRiskTestSuite {
	var filter *ypb.SSARisksFilter

	if len(taskIndex) > 0 && taskIndex[0] < len(suite.TaskIDs) {
		filter = &ypb.SSARisksFilter{
			RuntimeID: []string{suite.TaskIDs[taskIndex[0]]},
		}
	} else {
		filter = &ypb.SSARisksFilter{
			ProgramName: []string{suite.ProgramName},
		}
	}

	_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
	require.NoError(t, err)
	require.Len(t, risks, expectedCount, "风险数量不符合预期")

	t.Logf("风险数量检查通过: 期望 %d，实际 %d", expectedCount, len(risks))
	return suite
}

func (suite *SFScanRiskTestSuite) CheckIncrementalResult(t *testing.T, taskIndex, expectedCount int) *SFScanRiskTestSuite {
	require.True(t, taskIndex < len(suite.TaskIDs), "任务索引超出范围")

	taskID := suite.TaskIDs[taskIndex]
	_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		RuntimeID:   []string{taskID},
		Incremental: true,
	}, nil)
	require.NoError(t, err)
	require.Len(t, risks, expectedCount, "增量查询结果数量不符合预期")

	t.Logf("增量查询检查通过: 任务=%s，未处置风险数量=%d", taskID, expectedCount)
	return suite
}

func (suite *SFScanRiskTestSuite) CheckRiskTitlesContain(t *testing.T, expectedTitles []string, taskIndex ...int) *SFScanRiskTestSuite {
	var filter *ypb.SSARisksFilter

	if len(taskIndex) > 0 && taskIndex[0] < len(suite.TaskIDs) {
		filter = &ypb.SSARisksFilter{
			RuntimeID: []string{suite.TaskIDs[taskIndex[0]]},
		}
	} else {
		filter = &ypb.SSARisksFilter{
			ProgramName: []string{suite.ProgramName},
		}
	}

	_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
	require.NoError(t, err)

	actualTitles := make([]string, len(risks))
	titleSet := make(map[string]bool)
	for i, risk := range risks {
		actualTitles[i] = risk.Title
		titleSet[risk.Title] = true
	}

	for _, expectedTitle := range expectedTitles {
		require.True(t, titleSet[expectedTitle], "未找到期望的风险标题: %s", expectedTitle)
	}

	t.Logf("风险标题包含检查通过: 期望包含标题=%v，实际标题=%v", expectedTitles, actualTitles)
	return suite
}

func (suite *SFScanRiskTestSuite) Cleanup() {
	ssadb.DeleteProgram(ssadb.GetDB(), suite.ProgramName)
	yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{suite.ProgramName},
	})
	suite.t.Logf("已清理测试数据: %s", suite.ProgramName)
}

// buildTestRule 构建测试规则
func (suite *SFScanRiskTestSuite) buildTestRule() string {
	if len(suite.TestRisks) == 0 {
		return ""
	}

	var rules []string
	for _, risk := range suite.TestRisks {
		rule := fmt.Sprintf(`
%s as $%s
alert $%s for {
	desc: "Source-Sink vulnerability"
	Title: "%s"
	level: "high"
	risk: "%s"
}`, risk.SinkName, risk.SinkName, risk.SinkName, risk.Title, risk.ID)
		rules = append(rules, rule)
	}

	return strings.Join(rules, "\n")
}

func (suite *SFScanRiskTestSuite) getLatestTaskID() string {
	_, allRisks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{suite.ProgramName},
	}, nil)
	require.NoError(suite.t, err)
	require.NotEmpty(suite.t, allRisks)
	for _, risk := range allRisks {
		if !lo.Contains(suite.TaskIDs, risk.RuntimeId) {
			return risk.RuntimeId
		}
	}

	return allRisks[0].RuntimeId
}

func (suite *SFScanRiskTestSuite) HandleLastTaskRisks(fn func(risks []*schema.SSARisk) error) error {
	taskID := suite.getLatestTaskID()
	if taskID == "" {
		return utils.Error("no task id found")
	}
	_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		RuntimeID: []string{taskID},
	}, nil)
	if err != nil {
		return utils.Wrapf(err, "failed to query risks")
	}
	return fn(risks)
}
