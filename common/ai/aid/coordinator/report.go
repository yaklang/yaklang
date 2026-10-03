package coordinator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ReportDraft struct {
	Title           string `json:"title,omitempty"`
	Path            string `json:"path,omitempty"`
	Document        string `json:"document,omitempty"`
	Submitted       bool   `json:"submitted,omitempty"`
	UserRevision    uint64 `json:"user_revision,omitempty"`
	MessageSequence uint64 `json:"message_sequence,omitempty"`
}

func (c *Controller) reportReadyLocked() error {
	if err := c.canFinishLocked(); err != nil {
		return err
	}
	if c.editing {
		return fmt.Errorf("plan edit is in progress")
	}
	for _, m := range c.state.Inbox {
		if m.NeedsDecision && m.Sequence > c.state.CheckedThrough {
			return fmt.Errorf("unresolved %s message %s", m.Type, m.ID)
		}
	}
	return nil
}

func (c *Controller) ReportReady() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reportReadyLocked() == nil
}

func atomicReportWrite(path, document string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".report-*.md")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.WriteString(document)
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Controller) CreateReport(ctx context.Context, title, document string) (map[string]any, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(document) == "" {
		return nil, fmt.Errorf("report title and document must be nonempty")
	}
	c.editMu.Lock()
	defer c.editMu.Unlock()
	c.mu.Lock()
	if err := c.reportReadyLocked(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if c.state.Report.Path != "" {
		c.mu.Unlock()
		return nil, fmt.Errorf("report already exists; use modify_report")
	}
	if c.resultConfig == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("report needs an attached session")
	}
	path := filepath.Join(c.patchDir, "..", "coordinator-report.md")
	if c.patchDir == "" {
		c.mu.Unlock()
		return nil, fmt.Errorf("report artifact directory is not configured")
	}
	user, messages := c.state.UserRevision, c.state.NextMessage
	c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := atomicReportWrite(path, document); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.state.Report = ReportDraft{Title: title, Path: filepath.Clean(path), Document: document, UserRevision: user, MessageSequence: messages}
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	return map[string]any{"status": "created", "report_path": filepath.Clean(path)}, nil
}

func (c *Controller) ModifyReport(ctx context.Context, params map[string]any) (PlanEditReceipt, error) {
	r := PlanEditReceipt{Status: "updated"}
	if len(params) != 1 {
		return r, fmt.Errorf("provide exactly one of document/document_patch")
	}
	var document, patch string
	for key, value := range params {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return r, fmt.Errorf("report edit must be a nonempty string")
		}
		switch key {
		case "document":
			document = text
		case "document_patch":
			patch = text
		default:
			return r, fmt.Errorf("unknown report parameter %q", key)
		}
	}
	c.editMu.Lock()
	defer c.editMu.Unlock()
	c.mu.Lock()
	if err := c.reportReadyLocked(); err != nil {
		c.mu.Unlock()
		return r, err
	}
	draft := c.state.Report
	user, messages := c.state.UserRevision, c.state.NextMessage
	c.mu.Unlock()
	if draft.Path == "" {
		return r, fmt.Errorf("create_report first")
	}
	if patch != "" {
		var err error
		r.PatchArtifact, err = saveDocumentPatch(c.patchDir, patch)
		if err != nil {
			return r, err
		}
		document, err = applyTextPatch(draft.Document, patch, "coordinator-report.md")
		if err != nil {
			return r, err
		}
	}
	if document == draft.Document {
		r.Status = "unchanged"
		// Explicit revalidation may keep the prose while adopting the requirements
		// observed at this boundary. It never includes messages arriving later.
		c.mu.Lock()
		draft.UserRevision, draft.MessageSequence, draft.Submitted = user, messages, false
		c.state.Report = draft
		c.notifyLocked()
		c.mu.Unlock()
		c.publish()
		return r, nil
	}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if err := atomicReportWrite(draft.Path, document); err != nil {
		return r, err
	}
	c.mu.Lock()
	draft.Document = document
	draft.Submitted = false
	draft.UserRevision = user
	draft.MessageSequence = messages
	c.state.Report = draft
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	r.Components = []string{"report"}
	return r, nil
}

func (c *Controller) SubmitReport(observedUser, observedMessage uint64) (ReportDraft, error) {
	c.editMu.Lock()
	defer c.editMu.Unlock()
	c.mu.Lock()
	if err := c.reportReadyLocked(); err != nil {
		c.mu.Unlock()
		return ReportDraft{}, err
	}
	r := c.state.Report
	if r.Path == "" || strings.TrimSpace(r.Document) == "" {
		c.mu.Unlock()
		return r, fmt.Errorf("create a complete report first")
	}
	if observedUser != c.state.UserRevision || observedMessage != c.state.NextMessage {
		c.mu.Unlock()
		return r, fmt.Errorf("new messages or user requirements arrived during submission")
	}
	if r.UserRevision != c.state.UserRevision || r.MessageSequence != c.state.NextMessage {
		c.mu.Unlock()
		return r, fmt.Errorf("report predates user requirements; revise it first")
	}
	if r.Submitted {
		c.mu.Unlock()
		return r, fmt.Errorf("report already submitted")
	}
	// Include objective incomplete/failure facts even if the authored prose omitted them.
	var facts strings.Builder
	history := clone(c.state.History)
	if history == nil {
		history = make(map[string][]Attempt)
	}
	for id, a := range c.state.Attempts {
		if a.ID == 0 && a.State == Cancelled {
			history[id] = append(history[id], a)
		}
	}
	keys := make([]string, 0, len(history))
	for id := range history {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		attempts := history[id]
		for _, a := range attempts {
			if a.State == Failed || a.State == Rejected || a.State == Cancelled {
				fmt.Fprintf(&facts, "\n- %s / attempt %d: %s；%s；%s", a.Task.ID, a.ID, a.State, a.Result.Error, a.ReviewReason)
			}
		}
	}
	c.mu.Unlock()
	const resolutionMarker = "<!-- coordinator-resolution-facts -->"
	document := r.Document
	if at := strings.Index(document, resolutionMarker); at >= 0 {
		document = strings.TrimRight(document[:at], "\r\n")
	}
	if facts.Len() > 0 {
		document += "\n\n" + resolutionMarker + "\n## 系统记录的失败、重试与未完成范围\n" + facts.String() + "\n"
	}
	if document != r.Document {
		r.Document = document
		// Slow file I/O does not block discoveries, user input or worker settlement.
		if err := atomicReportWrite(r.Path, r.Document); err != nil {
			return r, err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.reportReadyLocked(); err != nil {
		return r, err
	}
	if observedUser != c.state.UserRevision || observedMessage != c.state.NextMessage {
		return r, fmt.Errorf("new messages arrived while writing the report")
	}
	r.Submitted = true
	r.MessageSequence = c.state.NextMessage
	c.state.Report = r
	c.notifyLocked()
	return r, nil
}

func (c *Controller) FinalizeReport() error {
	c.mu.Lock()
	if err := c.reportReadyLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if !c.state.Report.Submitted || c.state.Report.UserRevision != c.state.UserRevision || c.state.Report.MessageSequence != c.state.NextMessage {
		c.mu.Unlock()
		return fmt.Errorf("report is not current")
	}
	c.state.Finished = true
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	// Publishing the compatible completion snapshot may synchronously deliver a
	// new user event. Recheck that boundary before granting the loop exit.
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.state.Finished || !c.state.Report.Submitted || c.state.Report.UserRevision != c.state.UserRevision || c.state.Report.MessageSequence != c.state.NextMessage {
		return fmt.Errorf("new work arrived while publishing completion")
	}
	return c.reportReadyLocked()
}
