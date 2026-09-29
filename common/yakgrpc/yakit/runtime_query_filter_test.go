package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQueryHTTPFlowSupportsScopedLiteralEvidenceFilters(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	flows := []*schema.HTTPFlow{
		{RuntimeId: "runtime-a", Url: "https://example.test/literal-path", Method: "GET", StatusCode: 200, Request: "token%_literal", Response: "proof-a"},
		{RuntimeId: "runtime-b", Url: "https://example.test/literal-path", Method: "GET", StatusCode: 200, Request: "token%_literal", Response: "proof-b"},
		{RuntimeId: "runtime-a", Url: "https://example.test/literal-path", Method: "GET", StatusCode: 200, Request: "tokenXXliteral", Response: "proof-a"},
		{RuntimeId: "outside", Url: "https://example.test/literal-path", Method: "GET", StatusCode: 200, Request: "token%_literal", Response: "proof-a"},
	}
	for _, flow := range flows {
		require.NoError(t, db.Create(flow).Error)
	}

	page, got, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
		RuntimeIDs:       []string{"runtime-a", "runtime-b"},
		Methods:          "GET",
		StatusCode:       "200",
		IncludeInUrl:     []string{"literal-path"},
		RequestContains:  []string{"token%_literal"},
		ResponseContains: []string{"proof-a"},
		Full:             true,
		Pagination:       &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "desc"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, page.TotalRecord)
	require.Len(t, got, 1)
	require.Equal(t, flows[0].ID, got[0].ID)

	_, got, err = QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
		RuntimeIDs: []string{"runtime-a", "runtime-b"},
		IncludeId:  []int64{int64(flows[3].ID)},
		Full:       true,
		SkipTotal:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 1, OrderBy: "id", Order: "desc"},
	})
	require.NoError(t, err)
	require.Empty(t, got)
}
