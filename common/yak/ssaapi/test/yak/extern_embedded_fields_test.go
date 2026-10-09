package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type httpHistoryEmbeddedItem struct {
	*schema.HTTPFlow
	DatabaseID int64
}

type httpHistoryEmbeddedPage struct {
	Items []*httpHistoryEmbeddedItem
}

func TestExternEmbeddedHTTPFlowFields(t *testing.T) {
	// Reproduce the HTTP history tool's access through slice -> pointer ->
	// embedded HTTPFlow -> embedded gorm.Model, without expanding the Yak path.
	prog, err := ssaapi.Parse(`
page = db.QueryHTTPFlows()
id = page.Items[0].ID
explicitID = page.Items[0].HTTPFlow.Model.ID
url = page.Items[0].Url
databaseID = page.Items[0].DatabaseID
`, ssaapi.WithExternLib("db", map[string]any{
		"QueryHTTPFlows": func() *httpHistoryEmbeddedPage { return nil },
	}))
	require.NoError(t, err)
	require.Empty(t, prog.GetErrors(), prog.GetErrors().String())
	for name, kind := range map[string]ssa.TypeKind{
		"id": ssa.NumberTypeKind, "explicitID": ssa.NumberTypeKind,
		"url": ssa.StringTypeKind, "databaseID": ssa.NumberTypeKind,
	} {
		values := prog.Ref(name)
		require.Len(t, values, 1, name)
		require.Equal(t, kind, values[0].GetTypeKind(), name)
	}

	// A genuinely missing field must still produce a static diagnostic.
	invalid, err := ssaapi.Parse(`id = item.Missing`, ssaapi.WithExternValue(map[string]any{
		"item": &httpHistoryEmbeddedItem{},
	}))
	require.NoError(t, err)
	require.Contains(t, invalid.GetErrors().String(), "Missing")
}
