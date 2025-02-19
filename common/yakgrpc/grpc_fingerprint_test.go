package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestGRPC_FingerprintCURD_Base(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Run("Test Create Fingerprint", func(t *testing.T) {
		testName := utils.RandStringBytes(10)
		group := uuid.NewString()
		req := &ypb.CreateFingerprintRequest{
			Rule: &ypb.FingerprintRule{
				RuleName:  testName,
				GroupName: []string{group},
			},
		}
		message, err := client.CreateFingerprint(ctx, req)
		defer func() {
			yakit.DeleteGeneralRuleByName(consts.GetGormProfileDatabase(), testName)
		}()
		require.NoError(t, err)
		require.NotNil(t, message)
		require.Equal(t, "create", message.Operation)
		require.Equal(t, int64(1), message.EffectRows)

		getRule, err := yakit.GetGeneralRuleByRuleName(consts.GetGormProfileDatabase(), testName)
		require.NoError(t, err)
		require.NotNil(t, getRule)

		_, res, err := yakit.QueryGeneralRule(consts.GetGormProfileDatabase(), &ypb.FingerprintFilter{GroupName: []string{group}}, &ypb.Paging{})
		require.NoError(t, err)
		require.Len(t, res, 1)

		groupRes, err := client.GetAllFingerprintGroup(ctx, &ypb.Empty{})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(groupRes.Data), 1)
		var ok bool
		for _, datum := range groupRes.Data {
			if datum.GroupName == group || datum.Count == 1 {
				ok = true
				break
			}
		}
		require.True(t, ok)
	})

	t.Run("Test Delete Fingerprint", func(t *testing.T) {
		testName := utils.RandStringBytes(10)
		rule := &schema.GeneralRule{
			RuleName: testName,
		}
		err := yakit.CreateGeneralRule(consts.GetGormProfileDatabase(), rule)
		require.NoError(t, err)
		req := &ypb.DeleteFingerprintRequest{
			Filter: &ypb.FingerprintFilter{
				IncludeId: []int64{int64(rule.ID)},
			},
		}
		message, err := client.DeleteFingerprint(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, message)
		require.Equal(t, "delete", message.Operation)
		require.Equal(t, int64(1), message.EffectRows)

		_, err = yakit.GetGeneralRuleByID(consts.GetGormProfileDatabase(), int64(rule.ID))
		require.Error(t, err)
	})

	t.Run("Test Update Fingerprint By ID", func(t *testing.T) {
		testName := utils.RandStringBytes(10)
		rule := &schema.GeneralRule{
			RuleName: testName,
		}
		err := yakit.CreateGeneralRule(consts.GetGormProfileDatabase(), rule)
		require.NoError(t, err)
		defer func() {
			yakit.DeleteGeneralRuleByID(consts.GetGormProfileDatabase(), int64(rule.ID))
		}()
		newName := utils.RandStringBytes(10)
		req := &ypb.UpdateFingerprintRequest{
			Id:   int64(rule.ID),
			Rule: &ypb.FingerprintRule{RuleName: newName},
		}
		message, err := client.UpdateFingerprint(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, message)
		require.Equal(t, "update", message.Operation)
		require.Equal(t, int64(1), message.EffectRows)

		newRule, err := yakit.GetGeneralRuleByID(consts.GetGormProfileDatabase(), int64(rule.ID))
		require.NoError(t, err)
		require.Equal(t, newName, newRule.RuleName)
	})

	t.Run("Test Update Fingerprint By Name", func(t *testing.T) {
		testName := utils.RandStringBytes(10)
		testExpr := utils.RandStringBytes(10)
		rule := &schema.GeneralRule{
			RuleName:        testName,
			MatchExpression: testExpr,
		}
		err := yakit.CreateGeneralRule(consts.GetGormProfileDatabase(), rule)
		require.NoError(t, err)
		newExpr := utils.RandStringBytes(10)
		defer func() {
			yakit.DeleteGeneralRuleByID(consts.GetGormProfileDatabase(), int64(rule.ID))
		}()
		req := &ypb.UpdateFingerprintRequest{
			RuleName: testName,
			Rule:     &ypb.FingerprintRule{MatchExpression: newExpr},
		}
		message, err := client.UpdateFingerprint(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, message)
		require.Equal(t, "update", message.Operation)
		require.Equal(t, int64(1), message.EffectRows)

		newRule, err := yakit.GetGeneralRuleByID(consts.GetGormProfileDatabase(), int64(rule.ID))
		require.NoError(t, err)
		require.Equal(t, newExpr, newRule.MatchExpression)
	})

	t.Run("Test Query Fingerprint By Name", func(t *testing.T) {
		testVendor := utils.RandStringBytes(10)
		for i := 0; i < 10; i++ {
			err := yakit.CreateGeneralRule(consts.GetGormProfileDatabase(), &schema.GeneralRule{
				CPE: &schema.CPE{
					Vendor: testVendor,
				},
				RuleName: uuid.New().String(),
			})
			require.NoError(t, err)
		}
		defer func() {
			yakit.DeleteGeneralRuleByFilter(consts.GetGormProfileDatabase(), &ypb.FingerprintFilter{Vendor: []string{testVendor}})
		}()
		req := &ypb.QueryFingerprintRequest{
			Filter: &ypb.FingerprintFilter{Vendor: []string{testVendor}},
			Pagination: &ypb.Paging{
				Limit: 100,
			},
		}
		fingerprintResp, err := client.QueryFingerprint(ctx, req)
		require.NoError(t, err)
		require.Len(t, fingerprintResp.Data, 10)
	})

}

func TestGRPC_FingerprintCURD_Group(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("Test Create Fingerprint Group", func(t *testing.T) {
		testName := utils.RandStringBytes(10)
		req := &ypb.FingerprintGroup{
			GroupName: testName,
		}
		_, err := client.CreateFingerprintGroup(ctx, req)
		require.NoError(t, err)
		group, err := yakit.GetGeneralRuleGroupByName(consts.GetGormProfileDatabase(), testName)
		require.NoError(t, err)
		require.Equal(t, testName, group.GroupName)
	})

	t.Run("Test Delete Fingerprint Group", func(t *testing.T) {
		testName := utils.RandStringBytes(10)
		group := &schema.GeneralRuleGroup{
			GroupName: testName,
		}
		err := yakit.CreateGeneralRuleGroup(consts.GetGormProfileDatabase(), group)
		require.NoError(t, err)
		req := &ypb.DeleteFingerprintGroupRequest{
			GroupNames: []string{testName},
		}
		message, err := client.DeleteFingerprintGroup(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, message)
		require.Equal(t, "delete", message.Operation)
		require.Equal(t, int64(1), message.EffectRows)

		_, err = yakit.GetGeneralRuleGroupByName(consts.GetGormProfileDatabase(), testName)
		require.Error(t, err)
	})

	t.Run("Test Rename Fingerprint group", func(t *testing.T) {
		testName := utils.RandStringBytes(10)
		newName := utils.RandStringBytes(10)
		group := &schema.GeneralRuleGroup{
			GroupName: testName,
		}
		err := yakit.CreateGeneralRuleGroup(consts.GetGormProfileDatabase(), group)
		require.NoError(t, err)
		req := &ypb.RenameFingerprintGroupRequest{
			GroupName:    testName,
			NewGroupName: newName,
		}
		message, err := client.RenameFingerprintGroup(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, message)
		require.Equal(t, "update", message.Operation)
		require.Equal(t, int64(1), message.EffectRows)

		newGroup, err := yakit.GetGeneralRuleGroupByName(consts.GetGormProfileDatabase(), newName)
		require.NoError(t, err)
		require.Equal(t, newName, newGroup.GroupName)
	})

}
