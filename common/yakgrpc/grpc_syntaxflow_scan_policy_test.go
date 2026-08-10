//go:build !irify_exclude

package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SyntaxFlow_ScanPolicies(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("query all policies", func(t *testing.T) {
		rsp, err := client.QuerySyntaxFlowScanPolicies(context.Background(), &ypb.QuerySyntaxFlowScanPoliciesRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, rsp.GetVersion())
		require.NotEmpty(t, rsp.GetPolicies())
		require.NotEmpty(t, rsp.GetCategories())
		require.NotEmpty(t, rsp.GetCustomTagOptions())

		ids := map[string]struct{}{}
		for _, p := range rsp.GetPolicies() {
			ids[p.GetId()] = struct{}{}
			require.NotEmpty(t, p.GetName())
			require.NotNil(t, p.GetFilter())
		}
		require.Contains(t, ids, ssaconfig.PolicyTypeCriticalHigh)
		require.Contains(t, ids, ssaconfig.PolicyTypeOWASPWeb)
		require.Contains(t, ids, ssaconfig.PolicyTypeCustom)

		critical, err := client.QuerySyntaxFlowScanPolicies(context.Background(), &ypb.QuerySyntaxFlowScanPoliciesRequest{
			PolicyIds: []string{ssaconfig.PolicyTypeCriticalHigh},
		})
		require.NoError(t, err)
		require.Len(t, critical.GetPolicies(), 1)
		require.ElementsMatch(t, []string{"critical", "high"}, critical.GetPolicies()[0].GetFilter().GetSeverity())
		require.Empty(t, critical.GetPolicies()[0].GetFilter().GetGroupNames())
	})

	t.Run("policy to rule filter", func(t *testing.T) {
		rsp, err := client.SyntaxFlowScanPolicyToRuleFilter(context.Background(), &ypb.SyntaxFlowScanPolicyToRuleFilterRequest{
			PolicyId:   ssaconfig.PolicyTypeCriticalHigh,
			Language:   []string{"java"},
			GroupNames: []string{schema.SyntaxFlowGroupBuiltin},
		})
		require.NoError(t, err)
		require.NotNil(t, rsp.GetFilter())
		require.ElementsMatch(t, []string{"critical", "high"}, rsp.GetFilter().GetSeverity())
		require.Equal(t, []string{"java"}, rsp.GetFilter().GetLanguage())
		require.Equal(t, []string{schema.SyntaxFlowGroupBuiltin}, rsp.GetFilter().GetGroupNames())
	})

	t.Run("custom policy to rule filter", func(t *testing.T) {
		rsp, err := client.SyntaxFlowScanPolicyToRuleFilter(context.Background(), &ypb.SyntaxFlowScanPolicyToRuleFilterRequest{
			PolicyId: ssaconfig.PolicyTypeCustom,
			Tags:     []string{"spring", "cwe:89"},
			Severity: []string{"high"},
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"spring", "cwe:89"}, rsp.GetFilter().GetTag())
		require.ElementsMatch(t, []string{"high"}, rsp.GetFilter().GetSeverity())
	})
}
