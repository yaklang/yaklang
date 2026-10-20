package aicommon

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type UIArtifact struct {
	Document    interaction.ComponentDocument     `json:"document"`
	TaskID      string                            `json:"taskId"`
	Revision    int                               `json:"revision"`
	Diagnostics []interaction.Diagnostic          `json:"diagnostics,omitempty"`
	Reported    bool                              `json:"reported,omitempty"`
	Catalog     interaction.Catalog               `json:"catalog"`
	Submissions map[string]interaction.Submission `json:"submissions,omitempty"`
	origin      *Config
	ctx         context.Context
}

func (c *Config) UICatalog() interaction.Catalog {
	s := c.UISession()
	s.mu.Lock()
	defer s.mu.Unlock()
	var catalog interaction.Catalog
	_ = json.Unmarshal(s.client.Catalog, &catalog)
	if !c.GetAllowUserInteraction() {
		display := catalog.Components[:0]
		for _, entry := range catalog.Components {
			if entry.Name != "ParameterForm" && len(entry.Actions) == 0 && len(entry.Bindings) == 0 {
				display = append(display, entry)
			}
		}
		catalog.Components = display
	}
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
	if !c.GetAllowUserInteraction() {
		for _, node := range doc.Nodes {
			if node.Component == "ParameterForm" || node.Action != nil || len(node.Bindings) > 0 {
				return nil, fmt.Errorf("user interaction is disabled by session settings; do not substitute render_ui forms for questions")
			}
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
		Diagnostics: diagnostics, Reported: len(diagnostics) > 0, Catalog: catalog, origin: c, ctx: ctx}
	if previous := s.artifacts[doc.ID]; previous != nil {
		artifact.Submissions = previous.Submissions
	}
	s.artifacts[doc.ID] = artifact
	if _, err := c.EmitJSON("ui_component", "ui_component", artifact); err != nil {
		return nil, err
	}
	return map[string]any{
		"status": "rendered", "id": doc.ID, "diagnostics": diagnostics,
		"correctionAllowed": revision == 0 && len(diagnostics) > 0,
		"instruction":       "If diagnostics exist, correct ONCE using the SAME document and node IDs. Otherwise continue the task. Rendering does not wait for input. Only a later ordinary user message conveys submitted form values.",
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

func (c *Config) acceptComponentAnswerLocked(s *UISession, event *ypb.AIInputEvent, receipt interaction.Submission, deliver func(string, string, *Config) (string, error)) error {
	artifact := s.artifacts[receipt.RequestID]
	if artifact == nil {
		return fmt.Errorf("component block does not belong to this session")
	}
	if previous, exists := artifact.Submissions[receipt.NodeID]; exists {
		// A different transport ID must not turn the same form into another user
		// message. Publish authoritative state even when rejecting edited values.
		if _, err := c.EmitJSON("ui_component", "ui_component", artifact); err != nil {
			return err
		}
		before, _ := json.Marshal(previous.Values)
		after, _ := json.Marshal(receipt.Values)
		if string(before) != string(after) {
			return fmt.Errorf("form already submitted; use a new form to provide different values")
		}
		c.EmitSyncJSON("ui_receipt", "ui_receipt", map[string]any{"id": receipt.ID, "ok": true}, event.SyncID)
		return nil
	}
	if !c.GetAllowUserInteraction() {
		return fmt.Errorf("user interaction is disabled by session settings")
	}
	text, err := artifact.Document.SubmissionText(receipt, artifact.Catalog)
	if err != nil {
		return err
	}
	origin := artifact.origin
	if artifact.ctx == nil || artifact.ctx.Err() != nil {
		origin = nil
	}
	taskID, err := deliver(text, artifact.TaskID, origin)
	if err != nil {
		return err
	}
	message := UIUserMessage{ID: receipt.ID, TaskID: taskID, Text: text, Metadata: receipt}
	s.receipts[receipt.ID] = message
	if artifact.Submissions == nil {
		artifact.Submissions = make(map[string]interaction.Submission)
	}
	artifact.Submissions[receipt.NodeID] = receipt
	if _, err := c.EmitJSON("user_message", "user_message", message); err != nil {
		return err
	}
	if _, err := c.EmitJSON("ui_component", "ui_component", artifact); err != nil {
		return err
	}
	c.EmitSyncJSON("ui_receipt", "ui_receipt", map[string]any{"id": receipt.ID, "ok": true}, event.SyncID)
	return nil
}
