package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"path/filepath"
	"testing"
)

func createFingerprint(client ypb.YakClient, ruleName, matchExpr string) error {
	rule := &ypb.CreateFingerprintRequest{
		Rule: &ypb.FingerprintRule{
			RuleName:        ruleName,
			MatchExpression: matchExpr,
		},
	}
	_, err := client.CreateFingerprint(context.Background(), rule)
	return err
}

func createFingerprintGroups(client ypb.YakClient, groupNames []string) error {
	for _, group := range groupNames {
		req := &ypb.FingerprintGroup{
			GroupName: group,
		}
		_, err := client.CreateFingerprintGroup(context.Background(), req)
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteFingerprintGroup(client ypb.YakClient, groupNames []string) (int64, error) {
	req := &ypb.DeleteFingerprintGroupRequest{
		GroupNames: groupNames,
	}
	rsp, err := client.DeleteFingerprintGroup(context.Background(), req)
	if err != nil {
		return 0, err
	}
	return rsp.EffectRows, nil
}

func deleteFingerprintByNames(client ypb.YakClient, names []string) error {
	req := &ypb.DeleteFingerprintRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: names,
		},
	}
	_, err := client.DeleteFingerprint(context.Background(), req)
	return err
}

func addFingerprintGroups(client ypb.YakClient, ruleNames []string, groupNames []string) error {
	_, err := client.BatchUpdateFingerprintToGroup(context.Background(), &ypb.BatchUpdateFingerprintToGroupRequest{
		Filter: &ypb.FingerprintFilter{
			RuleName: ruleNames,
		},
		AppendGroupName: groupNames,
	})
	if err != nil {
		return err
	}
	return nil
}

func queryFingerprintGroupCount(client ypb.YakClient, wantCount int) (bool, error) {
	rsp, err := client.GetAllFingerprintGroup(context.Background(), &ypb.Empty{})
	if err != nil {
		return false, err
	}
	for _, datum := range rsp.Data {
		if int(datum.Count) != wantCount {
			return false, nil
		}
	}
	return true, nil
}

func queryFingerprintByName(client ypb.YakClient, ruleNames []string) ([]*ypb.FingerprintRule, error) {
	req := &ypb.QueryFingerprintRequest{
		Pagination: &ypb.Paging{Limit: -1},
		Filter: &ypb.FingerprintFilter{
			RuleName: ruleNames,
		},
	}
	rsp, err := client.QueryFingerprint(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return rsp.GetData(), nil
}

func TestGRPCMUSTPASS_Fingerprint_Export_And_Import(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	wantRulesCount := 16
	wantGroupsCount := 16
	ruleNames := make([]string, 0, wantRulesCount)
	groupNames := make([]string, 0, wantGroupsCount)
	// create groups
	for i := 0; i < wantGroupsCount; i++ {
		groupName := fmt.Sprintf("group_%s", uuid.NewString())
		groupNames = append(groupNames, groupName)
	}
	err = createFingerprintGroups(client, groupNames)
	require.NoError(t, err)
	t.Cleanup(func() {
		deleteFingerprintGroup(client, groupNames)
	})

	// create rules
	expr := uuid.NewString()
	for i := 0; i < wantRulesCount; i++ {
		ruleName := fmt.Sprintf("rule_%s", uuid.NewString())
		err = createFingerprint(client, ruleName, expr)
		ruleNames = append(ruleNames, ruleName)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		deleteFingerprintByNames(client, ruleNames)
	})
	err = addFingerprintGroups(client, ruleNames, groupNames)
	require.NoError(t, err)

	exportAndImportTest := func(t *testing.T, importRequest *ypb.ImportFingerprintRequest, exportRequest *ypb.ExportFingerprintRequest) {
		t.Helper()
		// export
		ctx := utils.TimeoutContextSeconds(50000)
		exportStream, err := client.ExportFingerprint(ctx, exportRequest)
		require.NoError(t, err)
		progress := 0.0
		for {
			msg, err := exportStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Logf("export stream error: %v", err)
				}
				break
			}
			progress = msg.Progress
		}
		require.Equal(t, 1.0, progress)
		// delete, for test import
		deleteFingerprintGroup(client, groupNames)
		deleteFingerprintByNames(client, ruleNames)

		// import
		importStream, err := client.ImportFingerprint(ctx, importRequest)
		require.NoError(t, err)
		progress = 0.0
		for {
			msg, err := importStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Logf("import stream error: %v", err)
				}
				break
			}
			progress = msg.Progress
		}
		require.Equal(t, 1.0, progress)

		// check rules
		rules, err := queryFingerprintByName(client, ruleNames)
		require.NoError(t, err)
		require.Len(t, rules, wantRulesCount)
		for _, rule := range rules {
			require.Equal(t, expr, rule.MatchExpression)
		}
		// check rule groups
		ok, err := queryFingerprintGroupCount(client, wantRulesCount)
		require.NoError(t, err)
		require.True(t, ok)
	}

	t.Run("no password", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "export.zip")
		exportAndImportTest(t, &ypb.ImportFingerprintRequest{
			InputPath: p,
		}, &ypb.ExportFingerprintRequest{
			Filter: &ypb.FingerprintFilter{
				GroupName: groupNames,
			},
			TargetPath: p,
		})
	})

	t.Run("password", func(t *testing.T) {
		password := uuid.NewString()
		p := filepath.Join(t.TempDir(), "export.zip.enc")
		exportAndImportTest(t, &ypb.ImportFingerprintRequest{
			InputPath: p,
			Password:  password,
		}, &ypb.ExportFingerprintRequest{
			Filter: &ypb.FingerprintFilter{
				GroupName: groupNames,
			},
			TargetPath: p,
			Password:   password,
		})
	})
}
