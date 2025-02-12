package yakit

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestExtracted_data(t *testing.T) {
	traceID := uuid.NewString()
	db := consts.GetGormProjectDatabase()
	for i := 0; i < 5; i++ {
		ruleName := uuid.NewString()
		data := utils.RandStringBytes(10)
		err2 := CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "test",
			TraceId:     traceID,
			RuleVerbose: ruleName,
			Data:        data,
		})
		require.NoError(t, err2)
	}

	// onlyName会将所有的都查出来，不会受到Pagination的影响
	_, extractedData, err := QueryExtractedDataPagination(db, &ypb.QueryMITMRuleExtractedDataRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 1},
		OnlyName:   true,
		Filter:     &ypb.ExtractedDataFilter{TraceID: []string{traceID}},
	})
	require.NoError(t, err)
	require.True(t, len(extractedData) > 0)
	require.True(t, len(extractedData) != 1 && len(extractedData) == 5)
	for _, datum := range extractedData {
		require.True(t, datum.TraceId == traceID)
	}
	_, extractedData, err = QueryExtractedDataPagination(db, &ypb.QueryMITMRuleExtractedDataRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 1},
		Filter:     &ypb.ExtractedDataFilter{TraceID: []string{traceID}},
	})
	require.NoError(t, err)
	require.True(t, len(extractedData) > 0)
	require.True(t, len(extractedData) == 1)
	for _, datum := range extractedData {
		require.True(t, datum.TraceId == traceID)
	}
}
