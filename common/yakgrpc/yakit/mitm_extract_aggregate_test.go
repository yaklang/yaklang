package yakit

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
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
	require.NoError(t, InsertHTTPFlow(db, flow))
	defer db.Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})

	rule := "agg-rule-" + token
	data := "agg-data-" + token
	for i := 0; i < 2; i++ {
		err := CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
			SourceType:  "httpflow",
			TraceId:     trace,
			RuleVerbose: rule,
			Data:        data,
		})
		require.NoError(t, err)
	}
	defer db.Unscoped().Where("trace_id = ?", trace).Delete(&schema.ExtractedData{})

	p, rows, _, err := QueryMITMExtractedAggregate(db, &ypb.QueryMITMExtractedAggregateRequest{
		Pagination:  &ypb.Paging{Page: 1, Limit: 10, OrderBy: "hit_count", Order: "desc"},
		HostContains:  "mitm-agg-",
		RuntimeId:     rt,
		RuleVerbose:   []string{rule},
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
	require.NoError(t, InsertHTTPFlow(db, flow))
	defer db.Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})

	g1Rule := "G1" + mitmExtractRuleGroupSep + "rule-a-" + token
	g1Other := "G1" + mitmExtractRuleGroupSep + "rule-b-" + token
	plain := "plain-uncat-" + token
	data := "d-" + token
	for _, rv := range []string{g1Rule, g1Other, plain} {
		require.NoError(t, CreateOrUpdateExtractedData(db, 0, &schema.ExtractedData{
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
		HostContains:                base.HostContains,
		RuntimeId:                   base.RuntimeId,
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
}