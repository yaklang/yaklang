//go:build hids && linux

package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

const (
	FileEvidenceScanKindSingleFile = "single_file_scan"
	FileEvidenceScanKindDirectory  = "directory_scan"
)

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
		return nil, nil
	}
	m.mu.Lock()
	instance := m.instance
	m.mu.Unlock()
	if instance == nil {
		return nil, fmt.Errorf("hids runtime is not active")
	}
	result, err := instance.scanFileEvidence(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("scan hids file evidence: %w", err)
	}
	return result, nil
}

func (i *Instance) scanFileEvidence(
	ctx context.Context,
	input FileEvidenceScanInput,
) (map[string]any, error) {
	if i == nil || i.pipeline == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	done := make(chan scanFileEvidenceResult, 1)
	go func() {
		result, err := i.pipeline.executeEvidenceRequest(model.Event{}, buildActiveFileEvidenceRequest(input))
		done <- scanFileEvidenceResult{result: result, err: err}
	}()
	select {
	case <-scanCtx.Done():
		return nil, scanCtx.Err()
	case result := <-done:
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
