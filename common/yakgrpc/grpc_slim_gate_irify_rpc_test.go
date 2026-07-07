//go:build irify_exclude

package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func requireUnimplementedRPC(t *testing.T, err error, method string) {
	t.Helper()
	require.Error(t, err, method)
	st, ok := grpcstatus.FromError(err)
	require.True(t, ok, "%s: expected grpc status, got %v", method, err)
	require.Equal(t, codes.Unimplemented, st.Code(), "%s: %v", method, st.Message())
	require.Contains(t, st.Message(), irifyExcludeRPCMsg, "%s: %v", method, st.Message())
}

// TestSlimGate_IrifyRPCs_Unimplemented verifies all irify_exclude SSA/SyntaxFlow stubs
// return gRPC Unimplemented instead of panicking when called via local gRPC client.
func TestSlimGate_IrifyRPCs_Unimplemented(t *testing.T) {
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	ctx := context.Background()

	type unaryCase struct {
		name string
		call func() error
	}
	cases := []unaryCase{
		{"QuerySSARisks", func() error {
			_, err := local.QuerySSARisks(ctx, &ypb.QuerySSARisksRequest{})
			return err
		}},
		{"DeleteSSARisks", func() error {
			_, err := local.DeleteSSARisks(ctx, &ypb.DeleteSSARisksRequest{})
			return err
		}},
		{"UpdateSSARiskTags", func() error {
			_, err := local.UpdateSSARiskTags(ctx, &ypb.UpdateSSARiskTagsRequest{})
			return err
		}},
		{"GetSSARiskFieldGroup", func() error {
			_, err := local.GetSSARiskFieldGroup(ctx, &ypb.Empty{})
			return err
		}},
		{"GetSSARiskFieldGroupEx", func() error {
			_, err := local.GetSSARiskFieldGroupEx(ctx, &ypb.GetSSARiskFieldGroupRequest{})
			return err
		}},
		{"NewSSARiskRead", func() error {
			_, err := local.NewSSARiskRead(ctx, &ypb.NewSSARiskReadRequest{})
			return err
		}},
		{"QueryNewSSARisks", func() error {
			_, err := local.QueryNewSSARisks(ctx, &ypb.QueryNewSSARisksRequest{})
			return err
		}},
		{"SSARiskFeedbackToOnline", func() error {
			_, err := local.SSARiskFeedbackToOnline(ctx, &ypb.SSARiskFeedbackToOnlineRequest{})
			return err
		}},
		{"CreateSSARiskDisposals", func() error {
			_, err := local.CreateSSARiskDisposals(ctx, &ypb.CreateSSARiskDisposalsRequest{})
			return err
		}},
		{"QuerySSARiskDisposals", func() error {
			_, err := local.QuerySSARiskDisposals(ctx, &ypb.QuerySSARiskDisposalsRequest{})
			return err
		}},
		{"DeleteSSARiskDisposals", func() error {
			_, err := local.DeleteSSARiskDisposals(ctx, &ypb.DeleteSSARiskDisposalsRequest{})
			return err
		}},
		{"UpdateSSARiskDisposals", func() error {
			_, err := local.UpdateSSARiskDisposals(ctx, &ypb.UpdateSSARiskDisposalsRequest{})
			return err
		}},
		{"GetSSARiskDisposal", func() error {
			_, err := local.GetSSARiskDisposal(ctx, &ypb.GetSSARiskDisposalRequest{})
			return err
		}},
		{"GenerateSSAReport", func() error {
			_, err := local.GenerateSSAReport(ctx, &ypb.GenerateSSAReportRequest{})
			return err
		}},
		{"QuerySSAPrograms", func() error {
			_, err := local.QuerySSAPrograms(ctx, &ypb.QuerySSAProgramRequest{})
			return err
		}},
		{"UpdateSSAProgram", func() error {
			_, err := local.UpdateSSAProgram(ctx, &ypb.UpdateSSAProgramRequest{})
			return err
		}},
		{"DeleteSSAPrograms", func() error {
			_, err := local.DeleteSSAPrograms(ctx, &ypb.DeleteSSAProgramRequest{})
			return err
		}},
		{"GetSSAWorkbenchDashboard", func() error {
			_, err := local.GetSSAWorkbenchDashboard(ctx, &ypb.GetSSAWorkbenchDashboardRequest{})
			return err
		}},
		{"QuerySSAProject", func() error {
			_, err := local.QuerySSAProject(ctx, &ypb.QuerySSAProjectRequest{})
			return err
		}},
		{"CreateSSAProject", func() error {
			_, err := local.CreateSSAProject(ctx, &ypb.CreateSSAProjectRequest{})
			return err
		}},
		{"UpdateSSAProject", func() error {
			_, err := local.UpdateSSAProject(ctx, &ypb.UpdateSSAProjectRequest{})
			return err
		}},
		{"DeleteSSAProject", func() error {
			_, err := local.DeleteSSAProject(ctx, &ypb.DeleteSSAProjectRequest{})
			return err
		}},
		{"QuerySyntaxFlowRule", func() error {
			_, err := local.QuerySyntaxFlowRule(ctx, &ypb.QuerySyntaxFlowRuleRequest{})
			return err
		}},
		{"CreateSyntaxFlowRuleEx", func() error {
			_, err := local.CreateSyntaxFlowRuleEx(ctx, &ypb.CreateSyntaxFlowRuleRequest{})
			return err
		}},
		{"CreateSyntaxFlowRule", func() error {
			_, err := local.CreateSyntaxFlowRule(ctx, &ypb.CreateSyntaxFlowRuleRequest{})
			return err
		}},
		{"UpdateSyntaxFlowRuleEx", func() error {
			_, err := local.UpdateSyntaxFlowRuleEx(ctx, &ypb.UpdateSyntaxFlowRuleRequest{})
			return err
		}},
		{"UpdateSyntaxFlowRule", func() error {
			_, err := local.UpdateSyntaxFlowRule(ctx, &ypb.UpdateSyntaxFlowRuleRequest{})
			return err
		}},
		{"DeleteSyntaxFlowRule", func() error {
			_, err := local.DeleteSyntaxFlowRule(ctx, &ypb.DeleteSyntaxFlowRuleRequest{})
			return err
		}},
		{"CheckSyntaxFlowRuleUpdate", func() error {
			_, err := local.CheckSyntaxFlowRuleUpdate(ctx, &ypb.CheckSyntaxFlowRuleUpdateRequest{})
			return err
		}},
		{"QuerySyntaxFlowRuleGroup", func() error {
			_, err := local.QuerySyntaxFlowRuleGroup(ctx, &ypb.QuerySyntaxFlowRuleGroupRequest{})
			return err
		}},
		{"DeleteSyntaxFlowRuleGroup", func() error {
			_, err := local.DeleteSyntaxFlowRuleGroup(ctx, &ypb.DeleteSyntaxFlowRuleGroupRequest{})
			return err
		}},
		{"CreateSyntaxFlowRuleGroup", func() error {
			_, err := local.CreateSyntaxFlowRuleGroup(ctx, &ypb.CreateSyntaxFlowGroupRequest{})
			return err
		}},
		{"UpdateSyntaxFlowRuleAndGroup", func() error {
			_, err := local.UpdateSyntaxFlowRuleAndGroup(ctx, &ypb.UpdateSyntaxFlowRuleAndGroupRequest{})
			return err
		}},
		{"UpdateSyntaxFlowRuleGroup", func() error {
			_, err := local.UpdateSyntaxFlowRuleGroup(ctx, &ypb.UpdateSyntaxFlowRuleGroupRequest{})
			return err
		}},
		{"QuerySyntaxFlowSameGroup", func() error {
			_, err := local.QuerySyntaxFlowSameGroup(ctx, &ypb.QuerySyntaxFlowSameGroupRequest{})
			return err
		}},
		{"QuerySyntaxFlowResult", func() error {
			_, err := local.QuerySyntaxFlowResult(ctx, &ypb.QuerySyntaxFlowResultRequest{})
			return err
		}},
		{"DeleteSyntaxFlowResult", func() error {
			_, err := local.DeleteSyntaxFlowResult(ctx, &ypb.DeleteSyntaxFlowResultRequest{})
			return err
		}},
		{"QuerySyntaxFlowScanTask", func() error {
			_, err := local.QuerySyntaxFlowScanTask(ctx, &ypb.QuerySyntaxFlowScanTaskRequest{})
			return err
		}},
		{"DeleteSyntaxFlowScanTask", func() error {
			_, err := local.DeleteSyntaxFlowScanTask(ctx, &ypb.DeleteSyntaxFlowScanTaskRequest{})
			return err
		}},
		{"GroupTableColumn", func() error {
			_, err := local.GroupTableColumn(ctx, &ypb.GroupTableColumnRequest{})
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireUnimplementedRPC(t, tc.call(), tc.name)
		})
	}
}
