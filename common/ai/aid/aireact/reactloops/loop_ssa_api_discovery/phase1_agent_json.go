package loop_ssa_api_discovery

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var aiTaggedJSONEndRe = regexp.MustCompile(`<\|[A-Za-z0-9_]+_END[^|]*\|>`)

// stripAITaggedJSONPayload removes stream/tag wrappers from agent JSON params.
func stripAITaggedJSONPayload(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < 3; i++ {
		if idx := strings.Index(s, "<|"); idx >= 0 {
			if end := strings.Index(s[idx:], "|>"); end >= 0 {
				s = strings.TrimSpace(s[idx+end+2:])
			}
		}
		s = aiTaggedJSONEndRe.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if unq, err := strconv.Unquote(s); err == nil {
			s = strings.TrimSpace(unq)
		}
	}
	return s
}

func parseAgentJSONObject(raw string, dest any) error {
	raw = stripAITaggedJSONPayload(raw)
	if err := json.Unmarshal([]byte(raw), dest); err == nil {
		return nil
	}
	// Retry after unescaping common double-encoding from tool params.
	if unq, err := strconv.Unquote(`"` + strings.ReplaceAll(raw, `"`, `\"`) + `"`); err == nil {
		if err2 := json.Unmarshal([]byte(unq), dest); err2 == nil {
			return nil
		}
	}
	return json.Unmarshal([]byte(raw), dest)
}
