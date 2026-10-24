package aicommon

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/interaction"
)

type UIArtifact struct {
	Document    interaction.ComponentDocument `json:"document"`
	TaskID      string                        `json:"taskId"`
	Revision    int                           `json:"revision"`
	Diagnostics []interaction.Diagnostic      `json:"diagnostics,omitempty"`
	Reported    bool                          `json:"reported,omitempty"`
	origin      *Config
	ctx         context.Context
}

func (c *Config) UICatalog() interaction.Catalog {
	s := c.UISession()
	s.mu.Lock()
	defer s.mu.Unlock()
	var catalog interaction.Catalog
	_ = json.Unmarshal(s.client.Catalog, &catalog)

	return catalog
}

func (c *Config) RenderUI(ctx context.Context, taskID string, doc interaction.ComponentDocument) (any, error) {
	if err := c.RestoreUIInteractions(); err != nil {
		return nil, err
	}
	catalog := c.UICatalog()
	if len(catalog.Components) == 0 {
		return nil, fmt.Errorf("client does not support generated UI; answer in plain text")
	}
	for _, node := range doc.Nodes {
		if node.Component == "ParameterForm" || node.Action != nil || node.Bindings != nil {
			return nil, fmt.Errorf("render_ui only displays data; input collection is not supported")
		}
	}
	s := c.UISession()
	s.mu.Lock()
	defer s.mu.Unlock()
	revision := 0
	if previous := s.artifacts[doc.ID]; previous != nil {
		revision = previous.Revision + 1
		if revision > 1 {
			return nil, fmt.Errorf("one correction has already been attempted; use plain text")
		}
	}
	normalized, diagnostics := catalog.Normalize(doc)
	if revision == 1 && len(diagnostics) > 0 {
		normalized.Nodes = nil
	}
	artifact := &UIArtifact{Document: normalized, TaskID: taskID, Revision: revision,
		Diagnostics: diagnostics, Reported: len(diagnostics) > 0, origin: c, ctx: ctx}
	s.artifacts[doc.ID] = artifact
	if _, err := c.EmitJSON("ui_component", "ui_component", artifact); err != nil {
		return nil, err
	}
	return map[string]any{
		"status": "rendered", "id": doc.ID, "diagnostics": diagnostics,
		"correctionAllowed": revision == 0 && len(diagnostics) > 0,
		"instruction":       "If diagnostics exist, correct ONCE using the SAME document and node IDs. Otherwise continue the task. Rendering only displays data; local inspection does not send user messages.",
	}, nil
}

func (c *Config) ReportUIDiagnostics(id string, diagnostics []interaction.Diagnostic) error {
	if err := c.RestoreUIInteractions(); err != nil {
		return err
	}
	if len(diagnostics) == 0 || len(diagnostics) > 32 {
		return fmt.Errorf("diagnostics must contain 1 to 32 entries")
	}
	s := c.UISession()
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact := s.artifacts[id]
	if artifact == nil {
		return fmt.Errorf("component block does not belong to this session")
	}
	if artifact.Reported || artifact.Revision > 0 {
		return nil
	}
	raw, _ := json.Marshal(diagnostics)
	if len(raw) > 8192 {
		return fmt.Errorf("diagnostics are too large")
	}
	artifact.Reported = true
	target := artifact.origin
	if target == nil {
		target = c
	}
	// Diagnostic data is feedback, never instructions or an automatic user reply.
	target.Timeline.PushText(target.AcquireId(), "[UI_VALIDATION] Block %s: %s\nCorrect at most once using the same IDs, or provide plain text.", id, string(raw))
	_, err := c.EmitJSON("ui_component", "ui_component", artifact)
	return err
}
