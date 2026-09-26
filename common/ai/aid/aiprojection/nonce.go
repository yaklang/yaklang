package aiprojection

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// This value identifies control tags created by this process. It is not a
// provider credential or a tool authorization decision. Timeline owns its
// persistence and rebinds saved envelopes when restoring another process's data.
var projectionNonce = newProjectionNonce()

func newProjectionNonce() string {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic("cannot initialize projection nonce")
	}
	return hex.EncodeToString(bytes[:])
}

// Nonce is for internal timeline snapshot metadata only. Never expose it in
// model inputs, user events or ordinary debug dumps.
func Nonce() string { return projectionNonce }

// RedactNonce is for human-readable diagnostics, never for replay/persistence.
func RedactNonce(text string) string {
	return strings.ReplaceAll(text, projectionNonce, "REDACTED_PROJECTION_NONCE")
}

// CreateTag authenticates only the envelope, not tags occurring in content.
// label remains the section/tool/bucket name; it is separate from our nonce.
func CreateTag(tag, label, content string) string {
	open, end := tag, tag+"_END"
	if label != "" {
		open += "_" + label
		end += "_" + label
	}
	return "<|" + open + "_" + projectionNonce + "|>\n" + content + "\n<|" + end + "_" + projectionNonce + "|>"
}

// CreateTemplate marks control tags in a TRUSTED template before substituting
// user/tool/model data. Never call this on an already assembled prompt.
func CreateTemplate(template string) string {
	return rewriteTagTokens(template, func(token string) string {
		if isProjectionToken(token) && !strings.HasSuffix(token, "_"+projectionNonce) {
			return token + "_" + projectionNonce
		}
		return token
	})
}

func isProjectionToken(token string) bool {
	for _, name := range []string{
		"PROMPT_SECTION", "AI_CACHE_SYSTEM", "AI_CACHE_FROZEN", "AI_CACHE_SEMI2", "AI_CACHE_SEMI",
		"TIMELINE", "FUNCTION_CALL_ACTION_SCHEMA", "FUNCTION_CALL_TOOL_PARAM_SCHEMA", "FUNCTION_CALL_ACTION_RESPONSE",
	} {
		if token == name || strings.HasPrefix(token, name+"_") {
			return true
		}
	}
	return false
}

// rewriteTagTokens never consumes a second opening delimiter as part of a
// malformed first token. Thus a truncated example cannot swallow a real tag.
func rewriteTagTokens(text string, rewrite func(string) string) string {
	var out strings.Builder
	for len(text) > 0 {
		start := strings.Index(text, "<|")
		if start < 0 {
			out.WriteString(text)
			break
		}
		out.WriteString(text[:start])
		text = text[start+2:]
		end := strings.Index(text, "|>")
		nested := strings.Index(text, "<|")
		if end < 0 || (nested >= 0 && nested < end) {
			out.WriteString("<|")
			continue
		}
		out.WriteString("<|" + rewrite(text[:end]) + "|>")
		text = text[end+2:]
	}
	return out.String()
}

// A reversible, deterministic escape keeps untrusted '<|' out of the legacy
// parsers, including malformed tags. Escape the escape byte first so arbitrary
// text (even text containing this sentinel) round-trips without collisions.
const literalEscape = "\ue000"

func prepareProjection(text, nonce string) (string, bool) {
	text = strings.ReplaceAll(text, literalEscape, literalEscape+"E")
	var out strings.Builder
	found := false
	for len(text) > 0 {
		start := strings.Index(text, "<|")
		if start < 0 {
			out.WriteString(text)
			break
		}
		out.WriteString(text[:start])
		text = text[start+2:]
		end := strings.Index(text, "|>")
		nested := strings.Index(text, "<|")
		if end >= 0 && (nested < 0 || nested > end) {
			token := text[:end]
			suffix := "_" + nonce
			if nonce != "" && strings.HasSuffix(token, suffix) && isProjectionToken(strings.TrimSuffix(token, suffix)) {
				out.WriteString("<|" + strings.TrimSuffix(token, suffix) + "|>")
				text = text[end+2:]
				found = true
				continue
			}
		}
		out.WriteString(literalEscape + "L")
	}
	return out.String(), found
}

func restoreProjectionText(text string) string {
	return RedactNonce(strings.NewReplacer(literalEscape+"L", "<|", literalEscape+"E", literalEscape).Replace(text))
}

func restoreProjectionValue(value any) any {
	switch v := value.(type) {
	case string:
		return restoreProjectionText(v)
	case []*aispec.ChatContent:
		for _, part := range v {
			if part != nil {
				part.Text = restoreProjectionText(part.Text)
			}
		}
	case map[string]any:
		restored := make(map[string]any, len(v))
		for key, item := range v {
			restored[restoreProjectionText(key)] = restoreProjectionValue(item)
		}
		return restored
	case []any:
		for i, item := range v {
			v[i] = restoreProjectionValue(item)
		}
	}
	return value
}

func restoreProjectionResult(result *aispec.ChatBaseHijackResult) {
	for i := range result.Messages {
		m := &result.Messages[i]
		m.Content = restoreProjectionValue(m.Content)
		m.ReasoningContent = restoreProjectionText(m.ReasoningContent)
		for _, call := range m.ToolCalls {
			if call != nil {
				call.Function.Arguments = restoreProjectionText(call.Function.Arguments)
				call.Description = restoreProjectionText(call.Description)
			}
		}
	}
	for i := range result.Tools {
		t := &result.Tools[i]
		t.Function.Description = restoreProjectionText(t.Function.Description)
		t.Function.Parameters = restoreProjectionValue(t.Function.Parameters)
	}
}

func CreateActionSchema(tool aispec.Tool) (string, error) {
	return createSchema(actionSchemaTagName, tool)
}

func CreateToolParamSchema(tool aispec.Tool) (string, error) {
	return createSchema(toolParamSchemaTagName, tool)
}

func createSchema(tag string, tool aispec.Tool) (string, error) {
	if tool.Type != "function" || !validActionSchemaName(tool.Function.Name) {
		return "", fmt.Errorf("invalid projection tool definition")
	}
	encoded, err := json.Marshal(tool)
	if err != nil {
		return "", err
	}
	var canonical aispec.Tool
	if err := json.Unmarshal(encoded, &canonical); err != nil {
		return "", err
	}
	params, ok := canonical.Function.Parameters.(map[string]any)
	if !ok || params["type"] != "object" {
		return "", fmt.Errorf("projection tool parameters must be an object schema")
	}
	return CreateTag(tag, tool.Function.Name, string(encoded)), nil
}

func CreateActionResponse(payload json.RawMessage) (string, error) {
	messages, err := decodeActionResponseMessages(string(payload))
	if err != nil {
		return "", err
	}
	// Marshal again to escape tag-looking strings inside JSON values.
	encoded, err := json.Marshal(messages)
	if err != nil {
		return "", err
	}
	return CreateTag(actionResponseTagName, "", string(encoded)), nil
}

// RebindReplayNonce is a pure envelope operation, not a persistence API.
// Timeline calls it only for dedicated PromptText records. Payload is never
// searched/replaced, and a missing nonce only permits known legacy envelopes.
func RebindReplayNonce(text, oldNonce string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "<|") {
		return text, fmt.Errorf("invalid replay envelope")
	}
	end := strings.Index(trimmed, "|>")
	if end < 0 {
		return text, fmt.Errorf("incomplete replay envelope")
	}
	open := trimmed[2:end]
	if oldNonce != "" {
		if !strings.HasSuffix(open, "_"+oldNonce) {
			return text, fmt.Errorf("replay nonce mismatch")
		}
		open = strings.TrimSuffix(open, "_"+oldNonce)
	}
	var tag, label string
	if open == actionResponseTagName {
		tag = open
	} else {
		for _, name := range []string{timelineModelThinkingReplayTagName, legacyTimelineModelThinkingReplayTagName} {
			if strings.HasPrefix(open, name+"_") {
				tag, label = name, strings.TrimPrefix(open, name+"_")
				break
			}
		}
	}
	if tag == "" {
		return text, fmt.Errorf("unsupported replay envelope")
	}
	if label != "" && !isReasoningReplayNonce(label) {
		return text, fmt.Errorf("invalid reasoning replay label")
	}
	close := "<|" + tag + "_END"
	if label != "" {
		close += "_" + label
	}
	if oldNonce != "" {
		close += "_" + oldNonce
	}
	close += "|>"
	if !strings.HasSuffix(trimmed, close) {
		return text, fmt.Errorf("incomplete replay closing tag")
	}
	payload := strings.TrimSpace(trimmed[end+2 : len(trimmed)-len(close)])
	if tag == actionResponseTagName {
		if _, err := decodeActionResponseMessages(payload); err != nil {
			return text, err
		}
	} else if _, ok := decodeReasoningReplayPayload(payload, tag == legacyTimelineModelThinkingReplayTagName); !ok {
		return text, fmt.Errorf("invalid reasoning replay payload")
	}
	if oldNonce == projectionNonce {
		return text, nil
	}
	// Only the two envelope tokens change; all payload bytes remain untouched.
	start := strings.Index(text, "<|")
	closeAt := strings.LastIndex(text, close)
	newOpen := "<|" + open + "_" + projectionNonce + "|>"
	newClose := strings.TrimSuffix(close, "|>")
	if oldNonce != "" {
		newClose = strings.TrimSuffix(newClose, "_"+oldNonce)
	}
	newClose += "_" + projectionNonce + "|>"
	return text[:start] + newOpen + text[start+end+2:closeAt] + newClose + text[closeAt+len(close):], nil
}
