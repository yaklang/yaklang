package reactloops

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const toolInformationGainStateKey = "tool_information_gain_state"

var (
	dynamicUUIDPattern      = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f-]{27,}\b`)
	dynamicTimestampPattern = regexp.MustCompile(`\b20\d{2}[-/:T][0-9T:.+\-/Z]+\b`)
	dynamicLongBlobPattern  = regexp.MustCompile(`(?i)(data:[^,;]+;base64,)?[a-z0-9+/=_-]{96,}`)
	dynamicNumberPattern    = regexp.MustCompile(`\b\d{3,}\b`)
	spacePattern            = regexp.MustCompile(`\s+`)
)

type toolInformationGainState struct {
	ToolName    string
	Signature   string
	Consecutive int
}

type ToolInformationGainObservation struct {
	Consecutive   int
	ShouldSwitch  bool
	NewlyDetected bool
	Hint          string
}

// HasLowInformationGainSignal reports whether the latest tool path has already
// produced three semantically equivalent results. It lets expensive control
// plane calls react to evidence instead of waking up on a fixed iteration.
func (r *ReActLoop) HasLowInformationGainSignal() bool {
	if r == nil {
		return false
	}
	state, _ := r.GetVariable(toolInformationGainStateKey).(*toolInformationGainState)
	return state != nil && state.Consecutive >= 3
}

func (r *ReActLoop) HasNewLowInformationGainSignal() bool {
	if r == nil {
		return false
	}
	state, _ := r.GetVariable(toolInformationGainStateKey).(*toolInformationGainState)
	return state != nil && state.Consecutive == 3
}

func normalizeToolResultForInformationGain(result *aitool.ToolResult) string {
	if result == nil {
		return "nil"
	}
	text := strings.ToLower(utils.InterfaceToString(result.Data) + " " + result.Error)
	text = dynamicLongBlobPattern.ReplaceAllString(text, "<blob>")
	text = dynamicUUIDPattern.ReplaceAllString(text, "<uuid>")
	text = dynamicTimestampPattern.ReplaceAllString(text, "<time>")
	text = dynamicNumberPattern.ReplaceAllString(text, "<number>")
	text = spacePattern.ReplaceAllString(text, " ")
	return fmt.Sprintf("success=%t %s", result.Success, strings.TrimSpace(text))
}

// ObserveToolInformationGain detects three consecutive semantically equivalent
// results from the same tool, even when responses contain changing UUIDs,
// timestamps, counters, or base64 captcha payloads.
func (r *ReActLoop) ObserveToolInformationGain(toolName string, result *aitool.ToolResult) ToolInformationGainObservation {
	if r == nil {
		return ToolInformationGainObservation{}
	}
	normalized := normalizeToolResultForInformationGain(result)
	sum := sha256.Sum256([]byte(normalized))
	signature := fmt.Sprintf("%x", sum[:8])
	state, _ := r.GetVariable(toolInformationGainStateKey).(*toolInformationGainState)
	if state == nil || state.ToolName != toolName || state.Signature != signature {
		state = &toolInformationGainState{ToolName: toolName, Signature: signature, Consecutive: 1}
	} else {
		state.Consecutive++
	}
	r.Set(toolInformationGainStateKey, state)
	observation := ToolInformationGainObservation{Consecutive: state.Consecutive}
	if state.Consecutive >= 3 {
		observation.ShouldSwitch = true
		observation.NewlyDetected = state.Consecutive == 3
		observation.Hint = fmt.Sprintf("[Low information gain] %s returned the same semantic result %d times. Stop retrying this path and switch attack surface. For web security, batch public paths with batch_do_http_request, then inspect OpenAPI/Swagger, CORS or unauthenticated endpoints, and static JS-discovered routes.", toolName, state.Consecutive)
	}
	return observation
}
