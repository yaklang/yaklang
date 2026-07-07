//go:build irify_exclude

// Package yakgrpc: SSA / SyntaxFlow gRPC and language-server stubs for yak-slim (irify_exclude) builds.
package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

const irifyExcludeRPCMsg = "not available in yak-slim (irify_exclude) build"

func irifyExcludeUnimplemented(method string) error {
	return grpcstatus.Errorf(codes.Unimplemented, "%s: %s", method, irifyExcludeRPCMsg)
}

// SSA Risk

func (s *Server) QuerySSARisks(ctx context.Context, req *ypb.QuerySSARisksRequest) (*ypb.QuerySSARisksResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySSARisks")
}

func (s *Server) DeleteSSARisks(ctx context.Context, req *ypb.DeleteSSARisksRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("DeleteSSARisks")
}

func (s *Server) UpdateSSARiskTags(ctx context.Context, req *ypb.UpdateSSARiskTagsRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("UpdateSSARiskTags")
}

func (s *Server) GetSSARiskFieldGroup(ctx context.Context, req *ypb.Empty) (*ypb.SSARiskFieldGroupResponse, error) {
	return nil, irifyExcludeUnimplemented("GetSSARiskFieldGroup")
}

func (s *Server) GetSSARiskFieldGroupEx(ctx context.Context, req *ypb.GetSSARiskFieldGroupRequest) (*ypb.SSARiskFieldGroupResponse, error) {
	return nil, irifyExcludeUnimplemented("GetSSARiskFieldGroupEx")
}

func (s *Server) NewSSARiskRead(ctx context.Context, req *ypb.NewSSARiskReadRequest) (*ypb.NewSSARiskReadResponse, error) {
	return nil, irifyExcludeUnimplemented("NewSSARiskRead")
}

func (s *Server) QueryNewSSARisks(ctx context.Context, req *ypb.QueryNewSSARisksRequest) (*ypb.QueryNewSSARisksResponse, error) {
	return nil, irifyExcludeUnimplemented("QueryNewSSARisks")
}

func (s *Server) SSARiskFeedbackToOnline(ctx context.Context, req *ypb.SSARiskFeedbackToOnlineRequest) (*ypb.Empty, error) {
	return nil, irifyExcludeUnimplemented("SSARiskFeedbackToOnline")
}

func (s *Server) ExportSSARisk(req *ypb.ExportSSARiskRequest, stream ypb.Yak_ExportSSARiskServer) error {
	return irifyExcludeUnimplemented("ExportSSARisk")
}

func (s *Server) ImportSSARisk(req *ypb.ImportSSARiskRequest, stream ypb.Yak_ImportSSARiskServer) error {
	return irifyExcludeUnimplemented("ImportSSARisk")
}

func (s *Server) SSARiskDiff(req *ypb.SSARiskDiffRequest, server ypb.Yak_SSARiskDiffServer) error {
	return irifyExcludeUnimplemented("SSARiskDiff")
}

// SSA Risk Disposals

func (s *Server) CreateSSARiskDisposals(ctx context.Context, req *ypb.CreateSSARiskDisposalsRequest) (*ypb.CreateSSARiskDisposalsResponse, error) {
	return nil, irifyExcludeUnimplemented("CreateSSARiskDisposals")
}

func (s *Server) QuerySSARiskDisposals(ctx context.Context, req *ypb.QuerySSARiskDisposalsRequest) (*ypb.QuerySSARiskDisposalsResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySSARiskDisposals")
}

func (s *Server) DeleteSSARiskDisposals(ctx context.Context, req *ypb.DeleteSSARiskDisposalsRequest) (*ypb.DeleteSSARiskDisposalsResponse, error) {
	return nil, irifyExcludeUnimplemented("DeleteSSARiskDisposals")
}

func (s *Server) UpdateSSARiskDisposals(ctx context.Context, req *ypb.UpdateSSARiskDisposalsRequest) (*ypb.UpdateSSARiskDisposalsResponse, error) {
	return nil, irifyExcludeUnimplemented("UpdateSSARiskDisposals")
}

func (s *Server) GetSSARiskDisposal(ctx context.Context, req *ypb.GetSSARiskDisposalRequest) (*ypb.GetSSARiskDisposalResponse, error) {
	return nil, irifyExcludeUnimplemented("GetSSARiskDisposal")
}

// SSA Report / Program / Workbench

func (s *Server) GenerateSSAReport(ctx context.Context, req *ypb.GenerateSSAReportRequest) (*ypb.GenerateSSAReportResponse, error) {
	return nil, irifyExcludeUnimplemented("GenerateSSAReport")
}

func (s *Server) QuerySSAPrograms(ctx context.Context, req *ypb.QuerySSAProgramRequest) (*ypb.QuerySSAProgramResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySSAPrograms")
}

func (s *Server) UpdateSSAProgram(ctx context.Context, req *ypb.UpdateSSAProgramRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("UpdateSSAProgram")
}

func (s *Server) DeleteSSAPrograms(ctx context.Context, req *ypb.DeleteSSAProgramRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("DeleteSSAPrograms")
}

func (s *Server) GetSSAWorkbenchDashboard(ctx context.Context, req *ypb.GetSSAWorkbenchDashboardRequest) (*ypb.GetSSAWorkbenchDashboardResponse, error) {
	return nil, irifyExcludeUnimplemented("GetSSAWorkbenchDashboard")
}

// SSA Project

func (s *Server) QuerySSAProject(ctx context.Context, req *ypb.QuerySSAProjectRequest) (*ypb.QuerySSAProjectResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySSAProject")
}

func (s *Server) CreateSSAProject(ctx context.Context, req *ypb.CreateSSAProjectRequest) (*ypb.CreateSSAProjectResponse, error) {
	return nil, irifyExcludeUnimplemented("CreateSSAProject")
}

func (s *Server) UpdateSSAProject(ctx context.Context, req *ypb.UpdateSSAProjectRequest) (*ypb.UpdateSSAProjectResponse, error) {
	return nil, irifyExcludeUnimplemented("UpdateSSAProject")
}

func (s *Server) DeleteSSAProject(ctx context.Context, req *ypb.DeleteSSAProjectRequest) (*ypb.DeleteSSAProjectResponse, error) {
	return nil, irifyExcludeUnimplemented("DeleteSSAProject")
}

func (s *Server) MigrateSSAProject(req *ypb.MigrateSSAProjectRequest, stream ypb.Yak_MigrateSSAProjectServer) error {
	return irifyExcludeUnimplemented("MigrateSSAProject")
}

// SyntaxFlow Rule

func (s *Server) QuerySyntaxFlowRule(ctx context.Context, req *ypb.QuerySyntaxFlowRuleRequest) (*ypb.QuerySyntaxFlowRuleResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySyntaxFlowRule")
}

func (s *Server) CreateSyntaxFlowRuleEx(ctx context.Context, req *ypb.CreateSyntaxFlowRuleRequest) (*ypb.CreateSyntaxFlowRuleResponse, error) {
	return nil, irifyExcludeUnimplemented("CreateSyntaxFlowRuleEx")
}

func (s *Server) CreateSyntaxFlowRule(ctx context.Context, req *ypb.CreateSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("CreateSyntaxFlowRule")
}

func (s *Server) UpdateSyntaxFlowRuleEx(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleRequest) (*ypb.UpdateSyntaxFlowRuleResponse, error) {
	return nil, irifyExcludeUnimplemented("UpdateSyntaxFlowRuleEx")
}

func (s *Server) UpdateSyntaxFlowRule(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("UpdateSyntaxFlowRule")
}

func (s *Server) DeleteSyntaxFlowRule(ctx context.Context, req *ypb.DeleteSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("DeleteSyntaxFlowRule")
}

func (s *Server) SyntaxFlowRuleToOnline(req *ypb.SyntaxFlowRuleToOnlineRequest, stream ypb.Yak_SyntaxFlowRuleToOnlineServer) error {
	return irifyExcludeUnimplemented("SyntaxFlowRuleToOnline")
}

func (s *Server) DownloadSyntaxFlowRule(req *ypb.DownloadSyntaxFlowRuleRequest, stream ypb.Yak_DownloadSyntaxFlowRuleServer) error {
	return irifyExcludeUnimplemented("DownloadSyntaxFlowRule")
}

func (s *Server) CheckSyntaxFlowRuleUpdate(ctx context.Context, req *ypb.CheckSyntaxFlowRuleUpdateRequest) (*ypb.CheckSyntaxFlowRuleUpdateResponse, error) {
	return nil, irifyExcludeUnimplemented("CheckSyntaxFlowRuleUpdate")
}

func (s *Server) ApplySyntaxFlowRuleUpdate(req *ypb.ApplySyntaxFlowRuleUpdateRequest, stream ypb.Yak_ApplySyntaxFlowRuleUpdateServer) error {
	return irifyExcludeUnimplemented("ApplySyntaxFlowRuleUpdate")
}

// SyntaxFlow Rule Group

func (s *Server) QuerySyntaxFlowRuleGroup(ctx context.Context, req *ypb.QuerySyntaxFlowRuleGroupRequest) (*ypb.QuerySyntaxFlowRuleGroupResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySyntaxFlowRuleGroup")
}

func (s *Server) DeleteSyntaxFlowRuleGroup(ctx context.Context, req *ypb.DeleteSyntaxFlowRuleGroupRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("DeleteSyntaxFlowRuleGroup")
}

func (s *Server) CreateSyntaxFlowRuleGroup(ctx context.Context, req *ypb.CreateSyntaxFlowGroupRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("CreateSyntaxFlowRuleGroup")
}

func (s *Server) UpdateSyntaxFlowRuleAndGroup(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleAndGroupRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("UpdateSyntaxFlowRuleAndGroup")
}

func (s *Server) UpdateSyntaxFlowRuleGroup(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleGroupRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("UpdateSyntaxFlowRuleGroup")
}

func (s *Server) QuerySyntaxFlowSameGroup(ctx context.Context, req *ypb.QuerySyntaxFlowSameGroupRequest) (*ypb.QuerySyntaxFlowSameGroupResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySyntaxFlowSameGroup")
}

// SyntaxFlow Result / Scan / Export

func (s *Server) QuerySyntaxFlowResult(ctx context.Context, req *ypb.QuerySyntaxFlowResultRequest) (*ypb.QuerySyntaxFlowResultResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySyntaxFlowResult")
}

func (s *Server) DeleteSyntaxFlowResult(ctx context.Context, req *ypb.DeleteSyntaxFlowResultRequest) (*ypb.DeleteSyntaxFlowResultResponse, error) {
	return nil, irifyExcludeUnimplemented("DeleteSyntaxFlowResult")
}

func (s *Server) SyntaxFlowScan(stream ypb.Yak_SyntaxFlowScanServer) error {
	return irifyExcludeUnimplemented("SyntaxFlowScan")
}

func (s *Server) QuerySyntaxFlowScanTask(ctx context.Context, request *ypb.QuerySyntaxFlowScanTaskRequest) (*ypb.QuerySyntaxFlowScanTaskResponse, error) {
	return nil, irifyExcludeUnimplemented("QuerySyntaxFlowScanTask")
}

func (s *Server) DeleteSyntaxFlowScanTask(ctx context.Context, request *ypb.DeleteSyntaxFlowScanTaskRequest) (*ypb.DbOperateMessage, error) {
	return nil, irifyExcludeUnimplemented("DeleteSyntaxFlowScanTask")
}

func (s *Server) ExportSyntaxFlows(req *ypb.ExportSyntaxFlowsRequest, stream ypb.Yak_ExportSyntaxFlowsServer) error {
	return irifyExcludeUnimplemented("ExportSyntaxFlows")
}

func (s *Server) ImportSyntaxFlows(req *ypb.ImportSyntaxFlowsRequest, stream ypb.Yak_ImportSyntaxFlowsServer) error {
	return irifyExcludeUnimplemented("ImportSyntaxFlows")
}

// Language server (syntaxflow completion)

func SyntaxFlowServer(req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, bool) {
	return nil, false
}

// Database (irify-only grouping helper)

func (s *Server) GroupTableColumn(ctx context.Context, req *ypb.GroupTableColumnRequest) (*ypb.GroupTableColumnResponse, error) {
	return nil, irifyExcludeUnimplemented("GroupTableColumn")
}
