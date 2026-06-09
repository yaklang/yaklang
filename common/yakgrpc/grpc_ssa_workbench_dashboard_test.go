package yakgrpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGetSSAWorkbenchDashboard(t *testing.T) {
	client, server, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	projectDB := server.GetProjectDatabase()

	ctx := context.Background()
	taskID := uuid.NewString()
	projectName := fmt.Sprintf("workbench-project-%s", uuid.NewString())
	sessionID := fmt.Sprintf("workbench-session-%s", uuid.NewString())

	createResp, err := client.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{
		Project: &ypb.SSAProject{
			ProjectName: projectName,
			Language:    "go",
			Description: "workbench dashboard test",
		},
	})
	require.NoError(t, err)
	projectID := createResp.GetProject().GetID()

	profileDB := server.GetProfileDatabase()
	ssaDB := ssadb.GetDB()

	defer func() {
		_ = yakit.DeleteSSARisks(ssaDB, &ypb.SSARisksFilter{RuntimeID: []string{taskID}})
		_ = yakit.DeleteSSARisksByProjectID(ssaDB, uint(projectID))
		_ = profileDB.Unscoped().
			Where("id = ?", projectID).
			Delete(&schema.SSAProject{}).Error
		_ = projectDB.Unscoped().
			Where("session_id = ?", sessionID).
			Delete(&schema.AISession{}).Error
	}()
	createRisk := func(index int64, severity, riskType, fromRule string) {
		err := yakit.CreateSSARisk(ssaDB, &schema.SSARisk{
			SSAProjectID:  uint64(projectID),
			ProgramName:   projectName,
			Severity:      schema.ValidSeverityType(severity),
			RiskType:      riskType,
			FromRule:      fromRule,
			RuntimeId:     taskID,
			Index:         index,
			CodeSourceUrl: fmt.Sprintf("ssadb://workbench/%d.go", index),
		})
		require.NoError(t, err)
	}

	createRisk(1, "critical", "sqli", "rule-sqli")
	createRisk(2, "high", "sqli", "rule-sqli")
	createRisk(3, "middle", "xss", "rule-xss")
	createRisk(4, "middle", "xss", "rule-xss")
	createRisk(5, "low", "info-leak", "rule-info")

	_, err = yakit.EnsureAISessionMeta(projectDB, sessionID, "irify")
	require.NoError(t, err)
	require.NoError(t, projectDB.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		Update("title", "workbench ai audit").Error)
	require.NoError(t, projectDB.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		Update("updated_at", time.Now()).Error)

	resp, err := client.GetSSAWorkbenchDashboard(ctx, &ypb.GetSSAWorkbenchDashboardRequest{
		RiskFilter: &ypb.SSARisksFilter{
			RuntimeID: []string{taskID},
		},
		RecentProjectLimit:    5,
		TopRuleHitLimit:       3,
		AIAuditSessionSources: []string{"irify"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetSummary())
	require.GreaterOrEqual(t, resp.GetSummary().GetProjectCount(), int64(1))
	require.GreaterOrEqual(t, resp.GetSummary().GetAIAuditTaskCount(), int64(1))
	require.Equal(t, int64(5), resp.GetTotalRiskCount())

	severityCount := map[string]int64{}
	for _, item := range resp.GetRiskOverview() {
		severityCount[item.GetSeverity()] = item.GetCount()
	}
	require.Equal(t, int64(1), severityCount["critical"])
	require.Equal(t, int64(1), severityCount["high"])
	require.Equal(t, int64(2), severityCount["middle"])
	require.Equal(t, int64(1), severityCount["low"])

	require.NotEmpty(t, resp.GetRiskDistribution())
	require.NotEmpty(t, resp.GetTopRuleHits())
	require.Equal(t, "rule-sqli", resp.GetTopRuleHits()[0].GetRuleName())
	require.Equal(t, int64(2), resp.GetTopRuleHits()[0].GetHitCount())

	foundProject := false
	for _, project := range resp.GetRecentProjects() {
		if project.GetID() != projectID {
			continue
		}
		foundProject = true
		require.Equal(t, projectName, project.GetProjectName())
		require.Equal(t, "golang", project.GetLanguage())
		require.Equal(t, int64(5), project.GetRiskCount())
		require.Equal(t, "critical", project.GetHighestRiskSeverity())
		require.Equal(t, "严重", project.GetHighestRiskVerbose())
	}
	require.True(t, foundProject, "recent project list should contain created project")
}

func TestBuildSSAWorkbenchRiskOverviewPercent(t *testing.T) {
	items, total := yakit.BuildSSAWorkbenchRiskOverview([]*yakit.SSARiskLevelCount{
		{Severity: "critical", Count: 1},
		{Severity: "high", Count: 1},
		{Severity: "middle", Count: 2},
		{Severity: "low", Count: 1},
	}, severityVerbose)

	require.Equal(t, int64(5), total)
	require.Len(t, items, 4)
	require.Equal(t, 20.0, items[0].GetPercent())
	require.Equal(t, 40.0, items[2].GetPercent())
}
