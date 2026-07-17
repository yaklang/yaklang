package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// BuildForkTaskID builds a stable sub-agent task id from the parent task and job identifier.
func BuildForkTaskID(parentTask aicommon.AIStatefulTask, job SubAgentJob) string {
	parentID := "sub-agent"
	if parentTask != nil && parentTask.GetId() != "" {
		parentID = parentTask.GetId()
	}
	segment := SanitizeIDSegment(job.Identifier)
	if segment == "" {
		segment = fmt.Sprintf("job-%d", job.Order)
	}
	return fmt.Sprintf("%s-sub-%s-%s", parentID, segment, utils.RandStringBytes(4))
}

// SanitizeIDSegment normalizes a job identifier for use in task ids.
func SanitizeIDSegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' || r == '/' {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}
