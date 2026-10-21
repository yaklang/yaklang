package interaction

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func visualizationDocument(t *testing.T) ComponentDocument {
	t.Helper()
	raw, err := os.ReadFile("testdata/visualizations.json")
	require.NoError(t, err)
	var doc ComponentDocument
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

func TestUIVisualizationsNormalizeAndSelectAuthoritativeRows(t *testing.T) {
	catalog := testCatalog(t)
	doc, diagnostics := catalog.Normalize(visualizationDocument(t))
	require.Empty(t, diagnostics)
	require.Len(t, doc.Nodes, 5)
	receipt := Submission{NodeID: "details", Values: map[string]any{
		"selectedRowIds": []any{"admin", "gateway"}, "rows": []any{map[string]any{"values": "forged"}},
	}}
	canonical, err := doc.NormalizeSubmission(receipt)
	require.NoError(t, err)
	require.Equal(t, []any{"gateway", "admin"}, canonical.Values["selectedRowIds"])
	rows := canonical.Values["rows"].([]any)
	require.Equal(t, float64(12), rows[0].(map[string]any)["values"].(map[string]any)["count"])
	text, err := doc.SubmissionText(receipt, catalog)
	require.NoError(t, err)
	require.Contains(t, text, "API 网关")
	require.Contains(t, text, "问题数量：12")
	require.NotContains(t, text, "用户服务")
	require.NotContains(t, text, "forged")
	for _, ids := range [][]any{{}, {"missing"}, {"gateway", "gateway"}, {1}} {
		_, err := doc.NormalizeSubmission(Submission{NodeID: "details", Values: map[string]any{"selectedRowIds": ids}})
		require.Error(t, err)
	}
	receipt.Values["extra"] = true
	_, err = doc.NormalizeSubmission(receipt)
	require.Error(t, err)
	delete(receipt.Values, "extra")
	doc.Nodes[4].Props["selection"] = "single"
	_, err = doc.NormalizeSubmission(receipt)
	require.Error(t, err)
	doc.Nodes[4].Props["selection"] = "none"
	_, err = doc.SubmissionText(receipt, catalog)
	require.Error(t, err)
	doc.Nodes[4].Props["selection"] = "multiple"
	doc.Nodes[4].Action = nil
	_, err = doc.SubmissionText(receipt, catalog)
	require.Error(t, err)
}

func TestUIVisualizationCrossFieldValidation(t *testing.T) {
	tests := map[string]func(*ComponentDocument){
		"mismatched bars": func(doc *ComponentDocument) {
			doc.Nodes[2].Props["series"].([]any)[0].(map[string]any)["values"] = []any{float64(1)}
		},
		"duplicate categories": func(doc *ComponentDocument) {
			categories := doc.Nodes[2].Props["categories"].([]any)
			categories[1].(map[string]any)["id"] = "gateway"
		},
		"duplicate series": func(doc *ComponentDocument) {
			series := doc.Nodes[2].Props["series"].([]any)
			doc.Nodes[2].Props["series"] = append(series, series[0])
		},
		"unordered x": func(doc *ComponentDocument) {
			points := doc.Nodes[3].Props["series"].([]any)[0].(map[string]any)["points"].([]any)
			points[1].(map[string]any)["x"] = float64(0)
		},
		"invalid time": func(doc *ComponentDocument) {
			points := doc.Nodes[3].Props["series"].([]any)[0].(map[string]any)["points"].([]any)
			points[0].(map[string]any)["x"] = -1e20
		},
		"duplicate row": func(doc *ComponentDocument) {
			rows := doc.Nodes[4].Props["rows"].([]any)
			rows[1].(map[string]any)["id"] = "gateway"
		},
		"duplicate column": func(doc *ComponentDocument) {
			columns := doc.Nodes[4].Props["columns"].([]any)
			columns[1].(map[string]any)["key"] = "module"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			doc := visualizationDocument(t)
			mutate(&doc)
			result, diagnostics := testCatalog(t).Normalize(doc)
			require.NotEmpty(t, diagnostics)
			require.Len(t, result.Nodes, 4, "invalid component should preserve valid siblings")
		})
	}
}

func TestUIDataTableRepairsCellsAndDisablesInvalidActions(t *testing.T) {
	doc := visualizationDocument(t)
	table := &doc.Nodes[4]
	row := table.Props["rows"].([]any)[0].(map[string]any)["values"].(map[string]any)
	row["count"], row["unknown"] = "bad", "remove"
	table.Props["initialSort"] = map[string]any{"key": "unknown"}
	table.Props["selection"] = "none"
	result, diagnostics := testCatalog(t).Normalize(doc)
	require.Len(t, result.Nodes, 5)
	require.Len(t, diagnostics, 4)
	values := result.Nodes[4].Props["rows"].([]any)[0].(map[string]any)["values"].(map[string]any)
	require.Nil(t, values["count"])
	require.NotContains(t, values, "unknown")
	require.NotContains(t, result.Nodes[4].Props, "initialSort")
	require.Nil(t, result.Nodes[4].Action)
	doc = visualizationDocument(t)
	doc.RequiredComponents = append(doc.RequiredComponents, "FutureChart")
	result, diagnostics = testCatalog(t).Normalize(doc)
	require.Empty(t, result.Nodes)
	require.NotEmpty(t, result.Fallback)
	require.NotEmpty(t, diagnostics)
}

func TestUIVisualizationsPermitMissingAndNegativeValues(t *testing.T) {
	doc := visualizationDocument(t)
	doc.Nodes[2].Props["series"].([]any)[0].(map[string]any)["values"] = []any{float64(-5), nil, float64(0)}
	points := doc.Nodes[3].Props["series"].([]any)[0].(map[string]any)["points"].([]any)
	points[1].(map[string]any)["y"] = nil
	result, diagnostics := testCatalog(t).Normalize(doc)
	require.Empty(t, diagnostics)
	require.Len(t, result.Nodes, 5)
	require.Nil(t, result.Nodes[2].Props["series"].([]any)[0].(map[string]any)["values"].([]any)[1])
}
