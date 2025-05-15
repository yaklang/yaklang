package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/samber/lo"
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

		test2Name := utils.RandStringBytes(10)
		req = &ypb.CreateFingerprintRequest{
			Rule: &ypb.FingerprintRule{
				RuleName:  test2Name,
				GroupName: []string{group},
			},
		}
		message, err = client.CreateFingerprint(ctx, req)
		defer func() {
			yakit.DeleteGeneralRuleByName(consts.GetGormProfileDatabase(), test2Name)
		}()
		require.NoError(t, err)
		require.NotNil(t, message)
		require.Equal(t, "create", message.Operation)
		require.Equal(t, int64(1), message.EffectRows)

		getRule, err = yakit.GetGeneralRuleByRuleName(consts.GetGormProfileDatabase(), test2Name)
		require.NoError(t, err)
		require.NotNil(t, getRule)

		_, res, err = yakit.QueryGeneralRule(consts.GetGormProfileDatabase(), &ypb.FingerprintFilter{GroupName: []string{group}}, &ypb.Paging{})
		require.NoError(t, err)
		require.Len(t, res, 2)
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

func createTestFingerprints(client ypb.YakClient, count int) ([]string, error) {
	testFingerprintName := make([]string, 0)
	for i := 0; i < count; i++ {
		name := uuid.NewString()
		testFingerprintName = append(testFingerprintName, name)
		err := createFingerprint(client, name, uuid.NewString())
		if err != nil {
			return nil, err
		}
	}
	return testFingerprintName, nil
}

func createTestFingerprintGroups(client ypb.YakClient, count int) ([]string, error) {
	testFingerprintGroupName := make([]string, 0)
	for i := 0; i < count; i++ {
		testFingerprintGroupName = append(testFingerprintGroupName, uuid.NewString())
	}
	if err := createFingerprintGroups(client, testFingerprintGroupName); err != nil {
		return nil, err
	}
	return testFingerprintGroupName, nil
}

func TestGRPC_FingerprintCURD_Associations(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testName, err := createTestFingerprints(client, 10)
	require.NoError(t, err)
	t.Cleanup(func() {
		deleteFingerprintByNames(client, testName)
	})
	testGroupName, err := createTestFingerprintGroups(client, 10)
	require.NoError(t, err)
	t.Cleanup(func() {
		deleteFingerprintGroup(client, testGroupName)
	})

	// test Batch append FingerprintToGroup Associations
	_, err = client.BatchUpdateFingerprintToGroup(ctx, &ypb.BatchUpdateFingerprintToGroupRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: testName,
		},
		AppendGroupName: testGroupName,
	})

	require.NoError(t, err)
	fingerprints, err := queryFingerprintByName(client, testName)
	require.NoError(t, err)
	require.Len(t, fingerprints, 10)
	for _, f := range fingerprints {
		require.Len(t, f.GroupName, 10)
	}

	// test Batch delete FingerprintToGroup Associations
	_, err = client.BatchUpdateFingerprintToGroup(ctx, &ypb.BatchUpdateFingerprintToGroupRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: testName,
		},
		DeleteGroupName: testGroupName,
	})

	require.NoError(t, err)
	fingerprints, err = queryFingerprintByName(client, testName)
	require.NoError(t, err)
	require.Len(t, fingerprints, 10)
	for _, f := range fingerprints {
		require.Len(t, f.GroupName, 0)
	}
}

func TestGRPC_FingerprintGroupSet(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// init test data
	testName, err := createTestFingerprints(client, 2)
	require.NoError(t, err)
	t.Cleanup(func() {
		deleteFingerprintByNames(client, testName)
	})

	testName1, testName2 := testName[0], testName[1]

	testGroupName, err := createTestFingerprintGroups(client, 3)
	require.NoError(t, err)
	t.Cleanup(func() {
		deleteFingerprintGroup(client, testGroupName)
	})

	// all fingerprint in testGroupAll , t1->g1 t2->g2
	testGroupAll, testGroup1, testGroup2 := testGroupName[0], testGroupName[1], testGroupName[2]

	_, err = client.BatchUpdateFingerprintToGroup(ctx, &ypb.BatchUpdateFingerprintToGroupRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: testName,
		},
		AppendGroupName: []string{testGroupAll},
	})
	require.NoError(t, err)

	_, err = client.BatchUpdateFingerprintToGroup(ctx, &ypb.BatchUpdateFingerprintToGroupRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: []string{testName1},
		},
		AppendGroupName: []string{testGroup1},
	})
	require.NoError(t, err)

	_, err = client.BatchUpdateFingerprintToGroup(ctx, &ypb.BatchUpdateFingerprintToGroupRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: []string{testName2},
		},
		AppendGroupName: []string{testGroup2},
	})
	require.NoError(t, err)

	rules, err := queryFingerprintByName(client, testName)
	if err != nil {
		return
	}

	for _, rule := range rules {
		require.Contains(t, rule.GroupName, testGroupAll)
		require.Len(t, rule.GroupName, 2)
		if rule.RuleName == testName1 {
			require.Contains(t, rule.GroupName, testGroup1)
		} else if rule.RuleName == testName2 {
			require.Contains(t, rule.GroupName, testGroup2)
		} else {
			require.Fail(t, "unexpected rule name")
		}
	}

	// test intersection
	groupSet, err := client.GetFingerprintGroupSetByFilter(ctx, &ypb.GetFingerprintGroupSetRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: testName,
		},
	})
	require.NoError(t, err)
	require.Len(t, groupSet.Data, 1)
	require.Equal(t, testGroupAll, groupSet.Data[0].GroupName)

	// test union
	groupSet, err = client.GetFingerprintGroupSetByFilter(ctx, &ypb.GetFingerprintGroupSetRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: testName,
		},
		Union: true,
	})
	require.NoError(t, err)
	require.Len(t, groupSet.Data, 3)

	groupNameSet := lo.Map(groupSet.Data, func(item *ypb.FingerprintGroup, _ int) string {
		return item.GroupName
	})
	require.Contains(t, groupNameSet, testGroupAll)
	require.Contains(t, groupNameSet, testGroup1)
	require.Contains(t, groupNameSet, testGroup2)
}
