package interaction

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

func TestUIPromptCatalogBudgets(t *testing.T) {
	catalog := testCatalog(t)
	index := catalog.PromptIndex()
	require.LessOrEqual(t, ytoken.CalcTokenCount(index), UIIndexTokenBudget)
	require.NotContains(t, index, "description")
	require.NotContains(t, index, "schema")
	for _, entry := range catalog.Components {
		require.Contains(t, index, entry.Name)
		content, names, err := catalog.LoadPromptDefinitions([]string{entry.Name, entry.Name})
		require.NoError(t, err)
		require.Equal(t, []string{entry.Name}, names)
		require.Contains(t, content, promptJSON(entry), "return the real definition, including actions and bindings")
		require.Equal(t, 1, strings.Count(content, promptJSON(entry)), "duplicates must not inflate context")
		require.LessOrEqual(t, ytoken.CalcTokenCount(content), UISchemaTokenBudget)
	}
	chart, _, err := catalog.LoadPromptDefinitions([]string{"BarChart"})
	require.NoError(t, err)
	require.NotContains(t, chart, `"name":"DataTable"`)
	t.Logf("catalog=%d, name index=%d, BarChart loaded view=%d Qwen BPE tokens", ytoken.CalcTokenCount(promptJSON(catalog)), ytoken.CalcTokenCount(index), ytoken.CalcTokenCount(chart))
}

func TestUIPromptDiscoveryRemainsBoundedWithLargeCatalog(t *testing.T) {
	catalog := Catalog{Version: "future"}
	for i := 0; i < MaxCatalogComponents; i++ {
		catalog.Components = append(catalog.Components, ComponentDefinition{
			Name: fmt.Sprintf("Widget%03d", i), Description: strings.Repeat("数据对比说明 ", 100),
			Schema: map[string]any{"type": "object", "properties": map[string]any{"secret": map[string]any{"type": "string"}}},
		})
	}
	require.NoError(t, catalog.Validate())
	require.Contains(t, catalog.PromptIndex(), "search_ui_components")
	require.NotContains(t, catalog.PromptIndex(), "Widget")
	require.LessOrEqual(t, ytoken.CalcTokenCount(catalog.PromptIndex()), UIIndexTokenBudget)
	seen := map[string]bool{}
	offset := 0
	for {
		page, err := catalog.Search("", offset)
		require.NoError(t, err)
		require.Equal(t, MaxCatalogComponents, page.Total)
		require.NotEmpty(t, page.Matches, "pagination must always advance")
		require.LessOrEqual(t, len(page.Matches), UILoadMaxNames)
		require.LessOrEqual(t, ytoken.CalcTokenCount(promptJSON(page)), UISearchTokenBudget)
		require.NotContains(t, promptJSON(page), "secret")
		for _, entry := range page.Matches {
			require.False(t, seen[entry.Name])
			require.True(t, utf8.ValidString(entry.Description))
			seen[entry.Name] = true
		}
		if page.Next == nil {
			break
		}
		require.Greater(t, *page.Next, offset)
		offset = *page.Next
	}
	require.Len(t, seen, MaxCatalogComponents)
	page, err := catalog.Search("widget511", 0)
	require.NoError(t, err)
	require.Len(t, page.Matches, 1)
	page, err = catalog.Search("数据对比", MaxCatalogComponents)
	require.NoError(t, err)
	require.Equal(t, MaxCatalogComponents, page.Total)
	require.Empty(t, page.Matches)
	_, err = catalog.Search("", -1)
	require.Error(t, err)
	_, err = catalog.Search(strings.Repeat("x", 129), 0)
	require.Error(t, err)
	catalog.Components = append(catalog.Components, ComponentDefinition{Name: "Extra", Schema: map[string]any{"type": "object"}})
	require.Error(t, catalog.Validate())
}

func TestUIPromptLoadRejectsUnknownOrOversizedDefinitions(t *testing.T) {
	catalog := testCatalog(t)
	for _, names := range [][]string{{"unknown"}, {"BarChart", "unknown"}, make([]string, UILoadMaxNames+1)} {
		content, _, err := catalog.LoadPromptDefinitions(names)
		require.Error(t, err)
		require.Empty(t, content)
	}
	catalog.Components[0].Schema["description"] = strings.Repeat("oversized schema definition ", UISchemaTokenBudget)
	content, _, err := catalog.LoadPromptDefinitions([]string{catalog.Components[0].Name})
	require.ErrorContains(t, err, "load a smaller set")
	require.Empty(t, content, "never truncate a schema into an invalid contract")
	content, names, err := catalog.LoadPromptDefinitions(nil)
	require.NoError(t, err)
	require.Empty(t, content)
	require.Empty(t, names)
}
