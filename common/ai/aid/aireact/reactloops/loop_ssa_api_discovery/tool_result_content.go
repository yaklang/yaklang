package loop_ssa_api_discovery

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// toolResultTextContent returns human-readable tool output for parsing (HTTP login probe, etc.).
// Prefer ToolExecutionResult.Stdout over utils.InterfaceToString, which JSON-serializes the struct
// and breaks line-oriented parsers (e.g. Cookie: headers after redirect follow).
func toolResultTextContent(result *aitool.ToolResult) string {
	if result == nil {
		return ""
	}
	switch data := result.Data.(type) {
	case *aitool.ToolExecutionResult:
		return joinToolExecutionText(data)
	case aitool.ToolExecutionResult:
		return joinToolExecutionText(&data)
	default:
		raw := utils.InterfaceToString(result.Data)
		if exec := parseToolExecutionResultJSON(raw); exec != nil {
			if text := joinToolExecutionText(exec); text != "" {
				return text
			}
		}
		return raw
	}
}

func joinToolExecutionText(exec *aitool.ToolExecutionResult) string {
	if exec == nil {
		return ""
	}
	var parts []string
	if s := strings.TrimSpace(exec.Stdout); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(exec.Stderr); s != "" {
		parts = append(parts, s)
	}
	if exec.Result != nil {
		if s := strings.TrimSpace(utils.InterfaceToString(exec.Result)); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}

func parseToolExecutionResultJSON(raw string) *aitool.ToolExecutionResult {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '{' {
		return nil
	}
	var exec aitool.ToolExecutionResult
	if json.Unmarshal([]byte(raw), &exec) != nil {
		return nil
	}
	if exec.Stdout == "" && exec.Stderr == "" && exec.Result == nil {
		return nil
	}
	return &exec
}
