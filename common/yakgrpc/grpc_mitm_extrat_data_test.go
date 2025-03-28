package yakgrpc

import (
	"bytes"
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"testing"
)

func TestMUSTPASS_MITM_Extracted(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	db := consts.GetGormProjectDatabase()

	traceID := uuid.NewString()
	ruleName := utils.RandStringBytes(10)
	data := utils.RandStringBytes(10)

	t.Cleanup(func() {
		db.Unscoped().Where("trace_id = ?", traceID).Delete(&schema.ExtractedData{})
	})
	for i := 0; i < 5; i++ {
		err := yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName,
			Data:        data,
		})
		require.NoError(t, err)
	}

	ruleName2 := utils.RandStringBytes(10)

	for i := 0; i < 5; i++ {
		err := yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName2,
			Data:        data,
		})
		require.NoError(t, err)
	}

	client, err := NewLocalClient()
	require.NoError(t, err)
	stream, err := client.ExportMITMRuleExtractedData(ctx, &ypb.ExportMITMRuleExtractedDataRequest{
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
		spew.Dump(rsp)
	}

	t.Cleanup(func() {
		os.Remove(exportPath)
	})

	require.True(t, progressOk)
	csvData, err := os.ReadFile(exportPath)
	require.NoError(t, err, "read export file fail: %s", exportPath)
	require.Equalf(t, bytes.Count(csvData, []byte("\n")), 3, string(csvData))
	require.Contains(t, string(csvData), ruleName)
	require.Contains(t, string(csvData), ruleName2)
}

func TestMUSTPASS_MITM_Extracted_Json(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	db := consts.GetGormProjectDatabase()

	traceID := uuid.NewString()
	ruleName := utils.RandStringBytes(10)
	data := utils.RandStringBytes(10)

	t.Cleanup(func() {
		db.Unscoped().Where("trace_id = ?", traceID).Delete(&schema.ExtractedData{})
	})
	for i := 0; i < 5; i++ {
		err := yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName,
			Data:        data,
		})
		require.NoError(t, err)
	}

	ruleName2 := utils.RandStringBytes(10)

	for i := 0; i < 5; i++ {
		err := yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName2,
			Data:        data,
		})
		require.NoError(t, err)
	}

	client, err := NewLocalClient()
	require.NoError(t, err)
	stream, err := client.ExportMITMRuleExtractedData(ctx, &ypb.ExportMITMRuleExtractedDataRequest{
		Type: "json",
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
		spew.Dump(rsp)
	}

	t.Cleanup(func() {
		os.Remove(exportPath)
	})

	type ExportedData struct {
		VerboseName string `json:"提取规则名"`
		Data        string `json:"内容数据"`
	}
	require.True(t, progressOk)
	jsonData, err := os.ReadFile(exportPath)
	require.NoError(t, err, "read export file fail: %s", exportPath)
	var exportedData []*ExportedData
	err = json.Unmarshal(jsonData, &exportedData)
	require.NoError(t, err)
	spew.Dump(exportedData)
	require.Equal(t, len(exportedData), 2)
	checkRuleName := false
	checkRuleName2 := false
	for _, data := range exportedData {
		if data.VerboseName == ruleName {
			checkRuleName = true
		}
		if data.VerboseName == ruleName2 {
			checkRuleName2 = true
		}
	}
	require.True(t, checkRuleName)
	require.True(t, checkRuleName2)
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
		err := yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName,
			Data:        data,
		})
		require.NoError(t, err)
	}

	ruleName2 := utils.RandStringBytes(10)

	for i := 0; i < 5; i++ {
		err := yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName2,
			Data:        data,
		})
		require.NoError(t, err)
	}

	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("traceID test", func(t *testing.T) {
		extractedData, err := client.QueryMITMRuleExtractedData(ctx, &ypb.QueryMITMRuleExtractedDataRequest{
			Filter: &ypb.ExtractedDataFilter{
				TraceID: []string{traceID},
			},
		})
		require.NoError(t, err)
		require.Lenf(t, extractedData.GetData(), 10, "extracted data count not equal 10")
	})

	t.Run("traceID test (legacy)", func(t *testing.T) {
		extractedData, err := client.QueryMITMRuleExtractedData(ctx, &ypb.QueryMITMRuleExtractedDataRequest{
			HTTPFlowHash: traceID,
		})
		require.NoError(t, err)
		require.Lenf(t, extractedData.GetData(), 10, "extracted data count not equal 10")

		extractedData, err = client.QueryMITMRuleExtractedData(ctx, &ypb.QueryMITMRuleExtractedDataRequest{
			HTTPFlowHiddenIndex: traceID,
		})
		require.NoError(t, err)
		require.Lenf(t, extractedData.GetData(), 10, "extracted data count not equal 10")
	})

	t.Run("ruleVerbose test", func(t *testing.T) {
		extractedData, err := client.QueryMITMRuleExtractedData(ctx, &ypb.QueryMITMRuleExtractedDataRequest{
			Filter: &ypb.ExtractedDataFilter{
				TraceID:     []string{traceID},
				RuleVerbose: []string{ruleName},
			},
		})
		require.NoError(t, err)
		require.Lenf(t, extractedData.GetData(), 5, "extracted data count not equal 5")
	})
}
