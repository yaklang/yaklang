package interaction

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func informationDocument(t *testing.T) ComponentDocument {
	t.Helper()
	raw, err := os.ReadFile("testdata/information.json")
	require.NoError(t, err)
	var doc ComponentDocument
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

func TestUIInformationContainersAndNestedSubmission(t *testing.T) {
	catalog := testCatalog(t)
	doc, diagnostics := catalog.Normalize(informationDocument(t))
	require.Empty(t, diagnostics)
	require.Len(t, doc.Nodes, 10)
	require.Equal(t, []string{"summary", "parameters"}, doc.Nodes[4].Children)
	require.Equal(t, []string{"request-text", "response-text"}, doc.Nodes[7].Children)
	text, err := doc.SubmissionText(Submission{NodeID: "parameters", Values: map[string]any{"scope": "API 网关", "note": "只读验证"}}, catalog)
	require.NoError(t, err)
	require.Contains(t, text, "只读验证")
	_, err = doc.SubmissionText(Submission{NodeID: "parameters", Values: map[string]any{"scope": "未知服务"}}, catalog)
	require.Error(t, err)
}

func TestUIInformationTabReferenceValidation(t *testing.T) {
	for _, key := range []string{"id", "content"} {
		t.Run("duplicate "+key, func(t *testing.T) {
			doc := informationDocument(t)
			items, _ := uiObjects(doc.Nodes[4].Props["items"])
			items[1][key] = items[0][key]
			normalized, diagnostics := testCatalog(t).Normalize(doc)
			require.NotEmpty(t, diagnostics)
			for _, node := range normalized.Nodes {
				require.NotEqual(t, "views", node.ID)
			}
		})
	}
	for _, target := range []string{"missing", "views", "root"} {
		t.Run("invalid reference "+target, func(t *testing.T) {
			doc := informationDocument(t)
			items, _ := uiObjects(doc.Nodes[4].Props["items"])
			items[0]["content"] = target
			normalized, diagnostics := testCatalog(t).Normalize(doc)
			require.NotEmpty(t, diagnostics)
			for _, node := range normalized.Nodes {
				if node.ID == "views" {
					require.Equal(t, []string{"parameters"}, node.Children)
				}
			}
		})
	}
	doc := informationDocument(t)
	doc.Nodes[4].Props["defaultTab"] = "missing"
	doc.Nodes[4].Children = []string{"source"}
	normalized, diagnostics := testCatalog(t).Normalize(doc)
	require.Len(t, diagnostics, 2)
	require.NotContains(t, normalized.Nodes[4].Props, "defaultTab")
	require.Equal(t, []string{"summary", "parameters"}, normalized.Nodes[4].Children)
}

func TestUIInformationRejectsExecutableAndUndeclaredProps(t *testing.T) {
	doc := informationDocument(t)
	doc.Nodes[1].Props["severity"] = "invalid"
	doc.Nodes[1].Props["visible"] = false
	doc.Nodes[1].Props["html"] = "<script>"
	doc.Nodes[2].Action = &ComponentAction{Name: "submit_to_chat", Args: map[string]any{}}
	doc.Nodes[3].Props["content"] = map[string]any{"component": "ParameterForm"}
	normalized, diagnostics := testCatalog(t).Normalize(doc)
	require.NotEmpty(t, diagnostics)
	for _, node := range normalized.Nodes {
		require.NotEqual(t, "source", node.ID, "popover content must be text")
		if node.ID == "risk" {
			require.NotContains(t, node.Props, "html")
			require.NotContains(t, node.Props, "visible")
		}
		if node.ID == "term" {
			require.Nil(t, node.Action)
		}
	}
}
