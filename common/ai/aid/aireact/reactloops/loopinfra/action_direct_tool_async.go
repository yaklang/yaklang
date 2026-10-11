package loopinfra

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const actionStateDirectToolAsync = "async_directly_call_tool"

// This optional scheduling hint tolerates common model formatting mistakes.
// Unknown values keep the synchronous default instead of retrying the tool call.
// Only explicit affirmative values opt in; objects and other numbers do not.
func normalizeDirectToolAsync(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "t", "1", "yes", "on":
			return true
		}
	case float64:
		return value == 1
	case int:
		return value == 1
	case int64:
		return value == 1
	case json.Number:
		number, err := value.Float64()
		return err == nil && number == 1
	}
	return false
}

func directToolActionAsync(action *aicommon.Action) bool {
	// Read only the canonical root (or next_action wrapper), never flattened
	// nested tool parameters or sibling batch members.
	raw, _ := lookupCanonicalActionParam(action, actionStateDirectToolAsync)
	return normalizeDirectToolAsync(raw)
}
