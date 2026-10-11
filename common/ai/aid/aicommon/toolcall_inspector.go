package aicommon

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

var _ asyncexecmanager.Inspector = (*ToolCaller)(nil)

// Inspector is stable before, during and after this caller's execution.
// It observes the original bounded output buffers, including cleanup output.
func (t *ToolCaller) Inspector() asyncexecmanager.Inspector {
	if t == nil {
		return nil
	}
	return t
}

func (t *ToolCaller) Inspect(ctx context.Context) (asyncexecmanager.Observation, error) {
	if ctx == nil {
		return asyncexecmanager.Observation{}, errors.New("inspection context is required")
	}
	if err := ctx.Err(); err != nil {
		return asyncexecmanager.Observation{}, err
	}
	if t == nil {
		return asyncexecmanager.Observation{}, errors.New("tool caller is nil")
	}
	t.inspectionMu.RLock()
	stdout, stderr := t.inspectionStdout, t.inspectionStderr
	result := append(json.RawMessage(nil), t.inspectionResult...)
	err := t.inspectionError
	t.inspectionMu.RUnlock()
	observation := asyncexecmanager.Observation{Result: result}
	if stdout != nil {
		observation.Stdout = string(stdout.Snapshot())
		observation.Truncated = stdout.Len() > toolOutputSnapshotMaxBytes
	}
	if stderr != nil {
		observation.Stderr = string(stderr.Snapshot())
		observation.Truncated = observation.Truncated || stderr.Len() > toolOutputSnapshotMaxBytes
	}
	return observation, err
}

func (t *ToolCaller) bindInspectionOutput(stdout, stderr *toolOutputBuffer) {
	t.inspectionMu.Lock()
	defer t.inspectionMu.Unlock()
	t.inspectionStdout, t.inspectionStderr = stdout, stderr
	t.inspectionResult, t.inspectionError = nil, nil
}

// Publish at the return boundary, after result normalization and cleanup. Freeze
// once so recipients of the mutable ToolResult cannot alter later observations.
// Result is the canonical ToolResult envelope without invocation parameters.
// Legacy cancellation can return while a callback still owns/mutates params;
// reading those maps here would introduce a race into the default call path.
func (t *ToolCaller) publishInspectionResult(result *aitool.ToolResult) {
	var raw json.RawMessage
	var err error
	if result != nil {
		envelope := *result
		envelope.Param = nil
		raw, err = json.Marshal(envelope)
	}
	t.inspectionMu.Lock()
	defer t.inspectionMu.Unlock()
	t.inspectionResult, t.inspectionError = raw, err
}

// WithToolCaller_WaitForCallbackOnCancel opts resources into joining plugin work
// before pipes/artifacts are closed. Cancellation remains cooperative; legacy
// callers retain their prompt cancellation response by default.
func WithToolCaller_WaitForCallbackOnCancel(enabled bool) ToolCallerOption {
	return func(t *ToolCaller) { t.waitForCallbackOnCancel = enabled }
}
