package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/utils"
)

// AuditDir returns the audit artifact directory under the session workdir.
func AuditDir(state *model.AuditState) string {
	if state.WorkDir != "" {
		return filepath.Join(state.WorkDir, "audit")
	}
	return filepath.Join(os.TempDir(), "code_audit_"+state.ProjectName)
}

// NewSubTask creates an isolated subtask for a pipeline phase loop.
func NewSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

// FormatLoopEndReason stringifies why a ReAct loop iteration ended without a terminal action.
func FormatLoopEndReason(reason any) string {
	if reason == nil {
		return "loop ended without calling the terminal action"
	}
	switch v := reason.(type) {
	case error:
		if v != nil {
			return v.Error()
		}
	case fmt.Stringer:
		return v.String()
	default:
		text := strings.TrimSpace(utils.InterfaceToString(v))
		if text != "" {
			return text
		}
	}
	return "loop ended without calling the terminal action"
}
