//go:build hids && linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

const (
	FileEvidenceScanKindSingleFile = "single_file_scan"
	FileEvidenceScanKindDirectory  = "directory_scan"
)

var ErrFileEvidenceScanBusy = errors.New("a file evidence scan is already in progress")

type FileEvidenceScanInput struct {
	Kind       string
	Path       string
	Recursive  bool
	MaxDepth   int
	MaxEntries int
}

func (m *Manager) ScanFileEvidence(
	ctx context.Context,
	input FileEvidenceScanInput,
) (map[string]any, error) {
	if m == nil {
		return nil, fmt.Errorf("hids runtime is not active")
	}
	m.mu.Lock()
	instance := m.instance
	m.mu.Unlock()
	if instance == nil || instance.pipeline == nil {
		return nil, fmt.Errorf("hids runtime is not active")
	}
	result, err := m.scanFileEvidenceWithExecutor(ctx, input, instance.pipeline.executeEvidenceRequestContext)
	if err != nil {
		return nil, fmt.Errorf("scan hids file evidence: %w", err)
	}
	return result, nil
}

func (m *Manager) scanFileEvidenceWithExecutor(
	ctx context.Context,
	input FileEvidenceScanInput,
	execute func(context.Context, model.Event, map[string]any) (map[string]any, error),
) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !m.fileScanMu.TryLock() {
		return nil, ErrFileEvidenceScanBusy
	}
	scanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	done := make(chan scanFileEvidenceResult, 1)
	go func() {
		// Keep the slot until real execution stops. A filesystem syscall may not
		// be preemptible by ctx, but a timeout must not spawn unbounded successors.
		// The manager owns this slot so replacing its runtime keeps the bound.
		defer m.fileScanMu.Unlock()
		result, err := execute(scanCtx, model.Event{}, buildActiveFileEvidenceRequest(input))
		done <- scanFileEvidenceResult{result: result, err: err}
	}()
	select {
	case <-scanCtx.Done():
		return nil, scanCtx.Err()
	case result := <-done:
		if err := scanCtx.Err(); err != nil {
			return nil, err
		}
		return result.result, result.err
	}
}

type scanFileEvidenceResult struct {
	result map[string]any
	err    error
}

func buildActiveFileEvidenceRequest(input FileEvidenceScanInput) map[string]any {
	kind := strings.TrimSpace(input.Kind)
	request := map[string]any{
		"kind":   kind,
		"target": strings.TrimSpace(input.Path),
		"reason": "manual_file_evidence_scan",
		"metadata": map[string]any{
			"path": strings.TrimSpace(input.Path),
		},
	}
	if kind == FileEvidenceScanKindDirectory {
		request["recursive"] = input.Recursive
		request["max_depth"] = input.MaxDepth
		request["max_entries"] = input.MaxEntries
	}
	return request
}
