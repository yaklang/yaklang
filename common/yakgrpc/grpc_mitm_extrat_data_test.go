package yakgrpc

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
	_ = yakit.CallPostInitDatabase()
	// 确保 extracted_data 表存在（单独运行测试时可能未迁移）
	_ = consts.GetGormProjectDatabase().AutoMigrate(&schema.ExtractedData{}).Error
}

func TestMUSTPASS_MITM_Extracted(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	db := consts.GetGormProjectDatabase()

	traceID := uuid.NewString()
	ruleName := utils.RandStringBytes(10)
	ruleName2 := utils.RandStringBytes(10)
	data := utils.RandStringBytes(10)

	t.Cleanup(func() {
		db.Unscoped().Where("trace_id = ?", traceID).Delete(&schema.ExtractedData{})
	})
	for i := 0; i < 5; i++ {
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName,
			Data:        data,
		}))
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName2,
			Data:        data,
		}))
	}

	client, err := NewLocalClient()
	require.NoError(t, err)

	testCases := []struct {
		name            string
		exportType      string
		expectedLineCnt int
		description     string
	}{
		{
			name:            "导出CSV格式",
			exportType:      "csv",
			expectedLineCnt: 3, // header + 2 unique rules (deduped from 10 to 2)
			description:     "CSV导出应去重后得到2条规则数据+1行表头",
		},
		{
			name:            "导出JSON格式",
			exportType:      "json",
			expectedLineCnt: 2, // 2 unique rule records
			description:     "JSON导出应去重后得到2条规则记录",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stream, err := client.ExportMITMRuleExtractedData(ctx, &ypb.ExportMITMRuleExtractedDataRequest{
				Type: tc.exportType,
				Filter: &ypb.ExtractedDataFilter{
					TraceID: []string{traceID},
				},
			})
			require.NoError(t, err)

			var exportPath string
			progressOk := false
			for {
				rsp, err := stream.Recv()
				if err != nil {
					break
				}
				if rsp.GetExportFilePath() != "" {
					exportPath = rsp.GetExportFilePath()
				}
				if rsp.Percent == 1 {
					progressOk = true
				}
			}

			t.Cleanup(func() {
				if exportPath != "" {
					os.Remove(exportPath)
				}
			})

			require.True(t, progressOk, tc.description)
			fileData, err := os.ReadFile(exportPath)
			require.NoError(t, err, "read export file fail: %s", exportPath)

			switch tc.exportType {
			case "csv":
				require.Equalf(t, bytes.Count(fileData, []byte("\n")), tc.expectedLineCnt, "%s: %s", tc.description, string(fileData))
				require.Contains(t, string(fileData), ruleName)
				require.Contains(t, string(fileData), ruleName2)
			case "json":
				type ExportedData struct {
					VerboseName string `json:"提取规则名"`
					Data        string `json:"数据内容"`
				}
				var exportedData []*ExportedData
				require.NoError(t, json.Unmarshal(fileData, &exportedData))
				require.Len(t, exportedData, tc.expectedLineCnt, tc.description)
				ruleNames := make(map[string]bool)
				for _, d := range exportedData {
					ruleNames[d.VerboseName] = true
				}
				require.True(t, ruleNames[ruleName], "JSON应包含规则名 %s", ruleName)
				require.True(t, ruleNames[ruleName2], "JSON应包含规则名 %s", ruleName2)
			}
		})
	}
}

func TestMUSTPASS_MITM_Query_Extracted(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	db := consts.GetGormProjectDatabase()

	traceID := uuid.NewString()
	ruleName := utils.RandStringBytes(10)
	data := utils.RandStringBytes(10)

	t.Cleanup(func() {
		db.Unscoped().Where("trace_id = ?", traceID).Delete(&schema.ExtractedData{})
	})
	for i := 0; i < 5; i++ {
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName,
			Data:        data,
		}))
	}
	ruleName2 := utils.RandStringBytes(10)
	for i := 0; i < 5; i++ {
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName2,
			Data:        data,
		}))
	}

	client, err := NewLocalClient()
	require.NoError(t, err)

	testCases := []struct {
		name          string
		filter        *ypb.ExtractedDataFilter
		expectedCount int
		description   string
	}{
		{
			name: "按TraceID查询",
			filter: &ypb.ExtractedDataFilter{
				TraceID: []string{traceID},
			},
			expectedCount: 10,
			description:   "TraceID过滤应返回10条",
		},
		{
			name: "按RuleVerbose查询",
			filter: &ypb.ExtractedDataFilter{
				TraceID:     []string{traceID},
				RuleVerbose: []string{ruleName},
			},
			expectedCount: 5,
			description:   "RuleVerbose过滤应返回5条",
		},
		{
			name: "按Keyword模糊搜索规则名",
			filter: &ypb.ExtractedDataFilter{
				TraceID: []string{traceID},
				Keyword: ruleName[:5], // 用规则名前缀做模糊匹配
			},
			expectedCount: 5,
			description:   "Keyword模糊搜索规则名应返回5条",
		},
		{
			name: "按Keyword模糊搜索数据内容",
			filter: &ypb.ExtractedDataFilter{
				TraceID: []string{traceID},
				Keyword: data[:5], // 用数据内容前缀做模糊匹配
			},
			expectedCount: 10,
			description:   "Keyword模糊搜索数据内容应返回10条",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.QueryMITMRuleExtractedData(ctx, &ypb.QueryMITMRuleExtractedDataRequest{
				Filter: tc.filter,
			})
			require.NoError(t, err)
			require.Lenf(t, resp.GetData(), tc.expectedCount, tc.description)
		})
	}
}

func TestMUSTPASS_MITM_Deduplicate_Extracted(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	client, srv, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	db := srv.GetProjectDatabase()
	require.NotNil(t, db, "project database not initialized")
	require.NoError(t, db.AutoMigrate(&schema.ExtractedData{}).Error)

	traceID1 := uuid.NewString()
	traceID2 := uuid.NewString()
	traceID3 := uuid.NewString()
	ruleName := utils.RandStringBytes(10)
	data := utils.RandStringBytes(10)

	t.Cleanup(func() {
		db.Unscoped().Where("trace_id IN (?)", []string{traceID1, traceID2, traceID3}).Delete(&schema.ExtractedData{})
	})

	// 创建测试数据：包1 5条重复，包2 5条重复，包3 3条不重复
	createTestData := func() {
		db.Unscoped().Where("trace_id IN (?)", []string{traceID1, traceID2, traceID3}).Delete(&schema.ExtractedData{})
		for i := 0; i < 5; i++ {
			require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
				SourceType: "test", TraceId: traceID1, RuleVerbose: ruleName, Data: data,
			}))
		}
		for i := 0; i < 5; i++ {
			require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
				SourceType: "test", TraceId: traceID2, RuleVerbose: ruleName, Data: data,
			}))
		}
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType: "test", TraceId: traceID3, RuleVerbose: "rule_a", Data: "data_1",
		}))
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType: "test", TraceId: traceID3, RuleVerbose: "rule_b", Data: "data_2",
		}))
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType: "test", TraceId: traceID3, RuleVerbose: "rule_c", Data: "data_3",
		}))
	}

	t.Run("只对包1去重", func(t *testing.T) {
		createTestData()
		rsp, err := client.DeduplicateMITMRuleExtractedData(ctx, &ypb.DeduplicateMITMRuleExtractedDataRequest{
			Filter: &ypb.ExtractedDataFilter{TraceID: []string{traceID1}},
		})
		require.NoError(t, err)
		require.Equal(t, int64(4), rsp.GetDeletedCount(), "只对包1去重应删 4 条")
		var c1, c2, c3 int64
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID1).Count(&c1).Error)
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID2).Count(&c2).Error)
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID3).Count(&c3).Error)
		require.Equal(t, int64(1), c1, "包1应剩 1 条")
		require.Equal(t, int64(5), c2, "包2应保持 5 条（未去重）")
		require.Equal(t, int64(3), c3, "包3应保持 3 条（未去重）")
	})

	t.Run("只对包1和包2去重", func(t *testing.T) {
		createTestData()
		rsp, err := client.DeduplicateMITMRuleExtractedData(ctx, &ypb.DeduplicateMITMRuleExtractedDataRequest{
			Filter: &ypb.ExtractedDataFilter{TraceID: []string{traceID1, traceID2}},
		})
		require.NoError(t, err)
		require.Equal(t, int64(8), rsp.GetDeletedCount(), "对包1和包2去重应删 8 条")
		var c1, c2, c3 int64
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID1).Count(&c1).Error)
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID2).Count(&c2).Error)
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID3).Count(&c3).Error)
		require.Equal(t, int64(1), c1, "包1应剩 1 条")
		require.Equal(t, int64(1), c2, "包2应剩 1 条")
		require.Equal(t, int64(3), c3, "包3应保持 3 条（未去重）")
	})

	t.Run("全表去重_TraceID为空", func(t *testing.T) {
		createTestData()
		rsp, err := client.DeduplicateMITMRuleExtractedData(ctx, &ypb.DeduplicateMITMRuleExtractedDataRequest{
			Filter: nil, // 全表去重
		})
		require.NoError(t, err)
		require.Equal(t, int64(8), rsp.GetDeletedCount(), "全表去重应删 8 条")
		var c1, c2, c3 int64
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID1).Count(&c1).Error)
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID2).Count(&c2).Error)
		require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID3).Count(&c3).Error)
		require.Equal(t, int64(1), c1, "包1应剩 1 条")
		require.Equal(t, int64(1), c2, "包2应剩 1 条")
		require.Equal(t, int64(3), c3, "包3应剩 3 条")
	})
}

func TestMUSTPASS_MITM_Delete_Extracted(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	db := consts.GetGormProjectDatabase()

	traceID := uuid.NewString()
	ruleName := utils.RandStringBytes(10)
	data := utils.RandStringBytes(10)

	t.Cleanup(func() {
		db.Unscoped().Where("trace_id = ?", traceID).Delete(&schema.ExtractedData{})
	})

	var ids []int64
	for i := 0; i < 5; i++ {
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName,
			Data:        data,
		}))
	}
	var list []schema.ExtractedData
	require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID).Find(&list).Error)
	for _, r := range list {
		ids = append(ids, int64(r.ID))
	}
	require.GreaterOrEqual(t, len(ids), 3, "需要至少3条记录")

	client, err := NewLocalClient()
	require.NoError(t, err)

	testCases := []struct {
		name               string
		req                *ypb.DeleteMITMRuleExtractedDataRequest
		expectedCountAfter int
		description        string
	}{
		{
			name:               "按Id删除单条",
			req:                &ypb.DeleteMITMRuleExtractedDataRequest{Id: ids[0]},
			expectedCountAfter: len(ids) - 1,
			description:        "按Id删除后应减少1条",
		},
		{
			name:               "按Ids批量删除",
			req:                &ypb.DeleteMITMRuleExtractedDataRequest{Ids: ids[1:]},
			expectedCountAfter: 0,
			description:        "按Ids批量删除后应剩0条",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.DeleteMITMRuleExtractedData(ctx, tc.req)
			require.NoError(t, err)
			var count int64
			require.NoError(t, db.Model(&schema.ExtractedData{}).Where("trace_id = ?", traceID).Count(&count).Error)
			require.Equal(t, int64(tc.expectedCountAfter), count, tc.description)
		})
	}
}
