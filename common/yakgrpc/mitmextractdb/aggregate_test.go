package mitmextractdb

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQueryMITMExtractedAggregate(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	trace := uuid.NewString()
	token := uuid.NewString()
	host := "mitm-agg-" + token + ".test"
	rt := "run-" + token

	flow := &schema.HTTPFlow{
		HiddenIndex: trace,
		Host:        host,
		RuntimeId:   rt,
		IsHTTPS:     true,
		Url:         "https://" + host + "/p",
		Path:        "/p",
		Method:      "GET",
		SourceType:  schema.HTTPFlow_SourceType_MITM,
	}
	flow.SetRequest(fmt.Sprintf("GET /p HTTP/1.1\r\nHost: %s\r\n\r\n", host))
	require.NoError(t, yakit.InsertHTTPFlow(db, flow))
	defer db.Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})

	rule := "agg-rule-" + token
	data := "agg-data-" + token
	for i := 0; i < 2; i++ {
		err := yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "httpflow",
			TraceId:     trace,
			RuleVerbose: rule,
			Data:        data,
		})
		require.NoError(t, err)
	}
	defer db.Unscoped().Where("trace_id = ?", trace).Delete(&schema.ExtractedData{})

	p, rows, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:   &ypb.Paging{Page: 1, Limit: 10, OrderBy: "hit_count", Order: "desc"},
		HostContains: "mitm-agg-",
		RuntimeId:    rt,
		RuleVerbose:  []string{rule},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, p.TotalRecord, 1)

	var hit *ypb.MITMExtractedAggregateRow
	for _, r := range rows {
		if r.RuleVerbose == rule && r.DisplayData == data {
			hit = r
			break
		}
	}
	require.NotNil(t, hit, "expected aggregate row")
	require.GreaterOrEqual(t, hit.HitCount, int64(2))
	require.Contains(t, hit.SampleTraceIds, trace)
}

func TestQueryMITMExtractedAggregateRuleGroups(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	trace := uuid.NewString()
	token := uuid.NewString()
	host := "mitm-agg-rg-" + token + ".test"
	rt := "run-rg-" + token

	flow := &schema.HTTPFlow{
		HiddenIndex: trace,
		Host:        host,
		RuntimeId:   rt,
		IsHTTPS:     true,
		Url:         "https://" + host + "/p",
		Path:        "/p",
		Method:      "GET",
		SourceType:  schema.HTTPFlow_SourceType_MITM,
	}
	flow.SetRequest(fmt.Sprintf("GET /p HTTP/1.1\r\nHost: %s\r\n\r\n", host))
	require.NoError(t, yakit.InsertHTTPFlow(db, flow))
	defer db.Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})

	g1Rule := "G1" + mitmExtractRuleGroupSep + "rule-a-" + token
	g1Other := "G1" + mitmExtractRuleGroupSep + "rule-b-" + token
	plain := "plain-uncat-" + token
	data := "d-" + token
	for _, rv := range []string{g1Rule, g1Other, plain} {
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "httpflow",
			TraceId:     trace,
			RuleVerbose: rv,
			Data:        data,
		}))
	}
	defer db.Unscoped().Where("trace_id = ?", trace).Delete(&schema.ExtractedData{})

	base := &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:   &ypb.Paging{Page: 1, Limit: 20, OrderBy: "hit_count", Order: "desc"},
		HostContains: "mitm-agg-rg-",
		RuntimeId:    rt,
	}

	_, rowsAll, groups, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:                base.Pagination,
		HostContains:              base.HostContains,
		RuntimeId:                 base.RuntimeId,
		IncludeDistinctRuleGroups: true,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"G1"}, groups)
	require.GreaterOrEqual(t, len(rowsAll), 3)

	_, rowsG1, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:   base.Pagination,
		HostContains: base.HostContains,
		RuntimeId:    base.RuntimeId,
		RuleGroup:    "G1",
	})
	require.NoError(t, err)
	var seenG1 int
	for _, r := range rowsG1 {
		if r.RuleVerbose == g1Rule || r.RuleVerbose == g1Other {
			seenG1++
		}
		require.NotEqual(t, plain, r.RuleVerbose)
	}
	require.Equal(t, 2, seenG1)

	_, rowsUncat, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:             base.Pagination,
		HostContains:           base.HostContains,
		RuntimeId:              base.RuntimeId,
		OnlyUncategorizedRules: true,
	})
	require.NoError(t, err)
	require.Len(t, rowsUncat, 1)
	require.Equal(t, plain, rowsUncat[0].RuleVerbose)

	_, rowsGkw, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:       base.Pagination,
		HostContains:     base.HostContains,
		RuntimeId:        base.RuntimeId,
		RuleGroupKeyword: "G",
	})
	require.NoError(t, err)
	require.Len(t, rowsGkw, 2)
	for _, r := range rowsGkw {
		require.NotEqual(t, plain, r.RuleVerbose)
	}

	_, rowsDataKw, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:         base.Pagination,
		HostContains:       base.HostContains,
		RuntimeId:          base.RuntimeId,
		RuleVerboseKeyword: data,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rowsDataKw), 3)
}

func TestQueryMITMExtractedAggregateHttpFlowFilter(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := uuid.NewString()
	rt := "run-hff-" + token
	host := "mitm-agg-hff-" + token + ".test"
	traceA := uuid.NewString()
	traceB := uuid.NewString()

	flowA := &schema.HTTPFlow{
		HiddenIndex: traceA,
		Host:        host,
		RuntimeId:   rt,
		IsHTTPS:     true,
		Url:         "https://" + host + "/path-a-" + token,
		Path:        "/path-a-" + token,
		Method:      "GET",
		SourceType:  schema.HTTPFlow_SourceType_MITM,
	}
	flowA.SetRequest(fmt.Sprintf("GET /path-a-%s HTTP/1.1\r\nHost: %s\r\n\r\n", token, host))
	require.NoError(t, yakit.InsertHTTPFlow(db, flowA))
	defer db.Unscoped().Where("id = ?", flowA.ID).Delete(&schema.HTTPFlow{})

	flowB := &schema.HTTPFlow{
		HiddenIndex: traceB,
		Host:        host,
		RuntimeId:   rt,
		IsHTTPS:     true,
		Url:         "https://" + host + "/path-b-" + token,
		Path:        "/path-b-" + token,
		Method:      "POST",
		SourceType:  schema.HTTPFlow_SourceType_MITM,
	}
	flowB.SetRequest(fmt.Sprintf("POST /path-b-%s HTTP/1.1\r\nHost: %s\r\n\r\n", token, host))
	require.NoError(t, yakit.InsertHTTPFlow(db, flowB))
	defer db.Unscoped().Where("id = ?", flowB.ID).Delete(&schema.HTTPFlow{})

	rule := "hff-rule-" + token
	require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
		SourceType:  "httpflow",
		TraceId:     traceA,
		RuleVerbose: rule,
		Data:        "data-a",
	}))
	require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
		SourceType:  "httpflow",
		TraceId:     traceB,
		RuleVerbose: rule,
		Data:        "data-b",
	}))
	defer db.Unscoped().Where("trace_id IN (?)", []string{traceA, traceB}).Delete(&schema.ExtractedData{})

	base := &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:   &ypb.Paging{Page: 1, Limit: 20, OrderBy: "hit_count", Order: "desc"},
		HostContains: "mitm-agg-hff-",
		RuntimeId:    rt,
	}

	pAll, _, _, err := QueryMITMExtractedAggregate(db, base)
	require.NoError(t, err)
	require.GreaterOrEqual(t, pAll.TotalRecord, 2)

	pF, rowsF, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:     base.Pagination,
		HostContains:   base.HostContains,
		RuntimeId:      base.RuntimeId,
		HttpFlowFilter: &ypb.QueryHTTPFlowRequest{Methods: "GET"},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, pF.TotalRecord, 1)
	var seenA, seenB bool
	for _, r := range rowsF {
		if r.RuleVerbose == rule && r.DisplayData == "data-a" {
			seenA = true
		}
		if r.RuleVerbose == rule && r.DisplayData == "data-b" {
			seenB = true
		}
	}
	require.True(t, seenA, "GET filter should keep path-a extraction")
	require.False(t, seenB, "GET filter should drop POST path-b extraction")
}

func TestQueryMITMExtractedAggregateRuleGroupKeywordEdges(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	trace := uuid.NewString()
	token := uuid.NewString()
	host := "mitm-agg-rgkw-" + token + ".test"
	rt := "run-rgkw-" + token

	flow := &schema.HTTPFlow{
		HiddenIndex: trace,
		Host:        host,
		RuntimeId:   rt,
		IsHTTPS:     true,
		Url:         "https://" + host + "/p",
		Path:        "/p",
		Method:      "GET",
		SourceType:  schema.HTTPFlow_SourceType_MITM,
	}
	flow.SetRequest(fmt.Sprintf("GET /p HTTP/1.1\r\nHost: %s\r\n\r\n", host))
	require.NoError(t, yakit.InsertHTTPFlow(db, flow))
	defer db.Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})

	g1Rule := "G1" + mitmExtractRuleGroupSep + "rule-a-" + token
	g1Other := "G1" + mitmExtractRuleGroupSep + "rule-b-" + token
	plain := "plain-uncat-" + token
	data := "d-rgkw-" + token
	for _, rv := range []string{g1Rule, g1Other, plain} {
		require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "httpflow",
			TraceId:     trace,
			RuleVerbose: rv,
			Data:        data,
		}))
	}
	defer db.Unscoped().Where("trace_id = ?", trace).Delete(&schema.ExtractedData{})

	base := &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:   &ypb.Paging{Page: 1, Limit: 20, OrderBy: "hit_count", Order: "desc"},
		HostContains: "mitm-agg-rgkw-",
		RuntimeId:    rt,
	}

	t.Run("exactRuleGroupOverridesKeyword", func(t *testing.T) {
		_, rows, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
			Pagination:       base.Pagination,
			HostContains:     base.HostContains,
			RuntimeId:        base.RuntimeId,
			RuleGroup:        "G1",
			RuleGroupKeyword: "nomatchzz",
		})
		require.NoError(t, err)
		require.Len(t, rows, 2)
	})

	t.Run("distinctRuleGroupsIgnoresRuleGroupKeyword", func(t *testing.T) {
		_, _, groups, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
			Pagination:                base.Pagination,
			HostContains:              base.HostContains,
			RuntimeId:                 base.RuntimeId,
			RuleGroupKeyword:          "G",
			IncludeDistinctRuleGroups: true,
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"G1"}, groups)
	})

	t.Run("ruleGroupKeywordDoesNotMatchUncategorizedByRuleName", func(t *testing.T) {
		_, rows, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
			Pagination:       base.Pagination,
			HostContains:     base.HostContains,
			RuntimeId:        base.RuntimeId,
			RuleGroupKeyword: plain,
		})
		require.NoError(t, err)
		require.Len(t, rows, 0)
	})
}

func TestQueryMITMExtractedAggregateRuleVerboseKeywordByColumn(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	trace := uuid.NewString()
	token := uuid.NewString()
	host := "mitm-agg-kwcol-" + token + ".test"
	rt := "run-kwcol-" + token

	flow := &schema.HTTPFlow{
		HiddenIndex: trace,
		Host:        host,
		RuntimeId:   rt,
		IsHTTPS:     true,
		Url:         "https://" + host + "/p",
		Path:        "/p",
		Method:      "GET",
		SourceType:  schema.HTTPFlow_SourceType_MITM,
	}
	flow.SetRequest(fmt.Sprintf("GET /p HTTP/1.1\r\nHost: %s\r\n\r\n", host))
	require.NoError(t, yakit.InsertHTTPFlow(db, flow))
	defer db.Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})

	rule := "kwcol-rule-" + token
	dataVal := "kwcol-data-" + token
	require.NoError(t, yakit.CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
		SourceType:  "httpflow",
		TraceId:     trace,
		RuleVerbose: rule,
		Data:        dataVal,
	}))
	defer db.Unscoped().Where("trace_id = ?", trace).Delete(&schema.ExtractedData{})

	base := &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:   &ypb.Paging{Page: 1, Limit: 10, OrderBy: "hit_count", Order: "desc"},
		HostContains: "mitm-agg-kwcol-",
		RuntimeId:    rt,
	}

	t.Run("matchRuleVerbose", func(t *testing.T) {
		_, rows, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
			Pagination:         base.Pagination,
			HostContains:       base.HostContains,
			RuntimeId:          base.RuntimeId,
			RuleVerboseKeyword: "kwcol-rule",
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, rule, rows[0].RuleVerbose)
	})

	t.Run("matchData", func(t *testing.T) {
		_, rows, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
			Pagination:         base.Pagination,
			HostContains:       base.HostContains,
			RuntimeId:          base.RuntimeId,
			RuleVerboseKeyword: "kwcol-data",
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, dataVal, rows[0].DisplayData)
	})
}
