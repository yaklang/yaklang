//go:build !irify_exclude

package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetSSAWorkbenchDashboard(ctx context.Context, req *ypb.GetSSAWorkbenchDashboardRequest) (*ypb.GetSSAWorkbenchDashboardResponse, error) {
	if req == nil {
		req = &ypb.GetSSAWorkbenchDashboardRequest{}
	}

	profileDB := consts.GetGormProfileDatabase()
	ssaDB := s.GetSSADatabase()

	projectCount, err := yakit.CountSSAProject(profileDB, nil)
	if err != nil {
		return nil, err
	}

	ruleCount, err := yakit.QuerySyntaxFlowRuleCount(profileDB, req.GetRuleFilter())
	if err != nil {
		return nil, err
	}

	aiSessionFilter := &ypb.AISessionFilter{}
	if len(req.GetAIAuditSessionSources()) > 0 {
		aiSessionFilter.Source = req.GetAIAuditSessionSources()
	}
	aiAuditTaskCount, err := yakit.CountAISessionMeta(s.GetProjectDatabase(), aiSessionFilter)
	if err != nil {
		return nil, err
	}

	riskFilter := req.GetRiskFilter()
	levelCounts, err := yakit.GetSSARiskLevelCount(ssaDB, riskFilter)
	if err != nil {
		return nil, err
	}
	riskOverview, totalRiskCount := yakit.BuildSSAWorkbenchRiskOverview(levelCounts, severityVerbose)

	riskTypeDB := yakit.FilterSSARisk(ssaDB, riskFilter)
	riskDistribution := yakit.BuildSSAWorkbenchRiskDistribution(
		yakit.SSARiskColumnGroupCount(riskTypeDB, "risk_type"),
		schema.SSARiskTypeVerbose,
	)

	topRuleHits, err := yakit.QuerySSAWorkbenchTopRuleHits(ssaDB, profileDB, riskFilter, req.GetTopRuleHitLimit())
	if err != nil {
		return nil, err
	}

	recentProjects, err := yakit.QuerySSAWorkbenchRecentProjects(
		profileDB,
		ssaDB,
		nil,
		req.GetRecentProjectLimit(),
		severityVerbose,
	)
	if err != nil {
		return nil, err
	}

	return &ypb.GetSSAWorkbenchDashboardResponse{
		Summary: &ypb.SSAWorkbenchSummary{
			ProjectCount:     projectCount,
			RuleCount:        ruleCount,
			AIAuditTaskCount: aiAuditTaskCount,
		},
		TotalRiskCount:   totalRiskCount,
		RiskOverview:     riskOverview,
		RiskDistribution: riskDistribution,
		TopRuleHits:      topRuleHits,
		RecentProjects:   recentProjects,
	}, nil
}
