package coordinator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

// Delivery is business output, separate from the scheduler's execution proof.
// The host grants completion only after both are saved at the observed boundary.
type Delivery struct {
	Content     string
	ContentType string
	Summary     string
}

type ResultDelivery func(context.Context, *Session, Snapshot) (Delivery, error)
type deliveryOption struct{ handler ResultDelivery }

func WithResultDelivery(handler ResultDelivery) aicommon.ConfigOption {
	return aicommon.WithAppendOtherOption(deliveryOption{handler: handler})
}

var errDeliveryChanged = errors.New("new work arrived during result delivery")

func (s *Session) HasResultDelivery() bool { return s.delivery != nil }

func hasDelivery(c *Controller) bool {
	if c == nil {
		return false
	}
	s, ok := c.host.(*Session)
	return ok && s.HasResultDelivery()
}

// deliverResult runs outside the controller lock. A stale output cannot mark
// work complete; the next model boundary must process the new input first.
func (s *Session) deliverResult(ctx context.Context) (bool, error) {
	if !s.HasResultDelivery() || !s.controller.ReportReady() {
		return false, nil
	}
	basis := s.Snapshot()
	if !basis.Report.Submitted {
		output, err := s.delivery(ctx, s, basis)
		if err != nil {
			return false, err
		}
		report, err := s.controller.saveDelivery(ctx, basis, output)
		if errors.Is(err, errDeliveryChanged) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		s.controller.publish()
		s.EmitPinFilename(report.Path)
		s.EmitJSON(schema.EVENT_TYPE_REPORT_FINISH, "report-finish", map[string]any{
			"report_path": report.Path, "title": report.Title, "summary_markdown": output.Summary,
		})
	}
	if err := s.controller.FinalizeReport(); err != nil {
		return false, nil
	}
	return true, nil
}

func (c *Controller) saveDelivery(ctx context.Context, basis Snapshot, output Delivery) (ReportDraft, error) {
	c.editMu.Lock()
	defer c.editMu.Unlock()
	check := func() error {
		if err := c.reportReadyLocked(); err != nil {
			return errDeliveryChanged
		}
		if c.state.UserRevision != basis.UserRevision || c.state.NextMessage != basis.NextMessage {
			return errDeliveryChanged
		}
		return ctx.Err()
	}
	c.mu.Lock()
	err := check()
	dir := filepath.Clean(filepath.Join(c.patchDir, ".."))
	c.mu.Unlock()
	if err != nil {
		return ReportDraft{}, err
	}
	if c.patchDir == "" {
		return ReportDraft{}, fmt.Errorf("delivery artifact directory is not configured")
	}
	ext := ".txt"
	if output.ContentType == "application/json" {
		ext = ".json"
	}
	stem := fmt.Sprintf("forge-result-%d-%d", basis.UserRevision, basis.NextMessage)
	resultPath := filepath.Join(dir, stem+ext)
	if err := atomicReportWrite(resultPath, output.Content); err != nil {
		return ReportDraft{}, err
	}
	var proof strings.Builder
	fmt.Fprintf(&proof, "# Forge execution proof\n\n%s\n\nBusiness result: %s\n", output.Summary, resultPath)
	for _, task := range basis.Plan.Tasks {
		a := basis.Attempts[task.ID]
		fmt.Fprintf(&proof, "\n- %s [%s] / attempt %d: %s\n  %s\n", task.Name, a.State, a.ID, a.Result.Summary, a.ReviewReason)
		if len(a.Result.EvidenceIDs) > 0 {
			fmt.Fprintf(&proof, "  Evidence: %s\n", strings.Join(a.Result.EvidenceIDs, ", "))
		}
		if len(a.Result.Artifacts) > 0 {
			fmt.Fprintf(&proof, "  Artifacts: %s\n", strings.Join(a.Result.Artifacts, ", "))
		}
	}
	if facts := reportResolutionFacts(basis); facts != "" {
		fmt.Fprintf(&proof, "\n\n## Failures, retries and incomplete scope\n%s\n", facts)
	}
	report := ReportDraft{Title: "Forge execution proof", Path: filepath.Join(dir, stem+"-execution.md"),
		Document: proof.String(), Submitted: true, UserRevision: basis.UserRevision, MessageSequence: basis.NextMessage,
		DeliveryPath: resultPath, DeliveryContentType: output.ContentType}
	if err := atomicReportWrite(report.Path, report.Document); err != nil {
		return report, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := check(); err != nil {
		return report, err
	}
	c.state.Report = report
	c.notifyLocked()
	return report, nil
}

func deliverForLoop(l *reactloops.ReActLoop, task aicommon.AIStatefulTask) (bool, error) {
	session, ok := controller(l).host.(*Session)
	if !ok || !session.HasResultDelivery() || planningOnly(l) {
		return false, nil
	}
	return session.deliverResult(task.GetContext())
}
