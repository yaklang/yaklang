package aicommon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// decodeToolParamObject accepts one complete JSON object, optionally fenced,
// followed by complete parameter AITAG blocks. Unlike the general Action
// extractor, it must not search nested objects or later examples for a call.
func decodeToolParamObject(raw string, paramNames []string) (aitool.InvokeParams, []toolParamAITagBlock, error) {
	remaining := trimToolParamFence(strings.TrimSpace(raw))
	decoder := json.NewDecoder(strings.NewReader(remaining))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, nil, fmt.Errorf("expected one complete parameter JSON object")
	}
	root := make(aitool.InvokeParams)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return nil, nil, fmt.Errorf("invalid parameter object: %w", err)
		}
		key, ok := token.(string)
		if !ok {
			return nil, nil, fmt.Errorf("invalid parameter object key")
		}
		if _, exists := root[key]; exists {
			return nil, nil, fmt.Errorf("duplicate parameter envelope key %q", key)
		}
		var value any
		if err = decoder.Decode(&value); err != nil {
			return nil, nil, fmt.Errorf("invalid parameter field %q: %w", key, err)
		}
		root[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, nil, fmt.Errorf("parameter JSON object is incomplete")
	}
	var blocks []toolParamAITagBlock
	seenBlocks := make(map[string]bool)
	remaining = strings.TrimSpace(remaining[decoder.InputOffset():])
	for remaining != "" {
		remaining = trimToolParamFence(remaining)
		if remaining == "" {
			break
		}
		// Match a complete pair against declared names: both parameter names
		// and nonces may contain underscores, so the opening tag alone can be
		// ambiguous when one declared name is a prefix of another.
		var matchedName, nonce string
		var contentStart, contentEnd, blockEnd int
		for _, name := range paramNames {
			prefix := "<|TOOL_PARAM_" + name + "_"
			if !strings.HasPrefix(remaining, prefix) {
				continue
			}
			end := strings.Index(remaining[len(prefix):], "|>")
			if end < 0 {
				continue
			}
			candidateNonce := remaining[len(prefix) : len(prefix)+end]
			if candidateNonce == "" || strings.ContainsAny(candidateNonce, "\r\n<>") {
				continue
			}
			start := len(prefix) + end + 2
			endTag := "<|TOOL_PARAM_" + name + "_END_" + candidateNonce + "|>"
			end = strings.Index(remaining[start:], endTag)
			if end < 0 {
				continue
			}
			if matchedName != "" {
				return nil, nil, fmt.Errorf("ambiguous parameter AITAG; use the current nonce and one matching end tag")
			}
			matchedName = name
			nonce = candidateNonce
			contentStart, contentEnd, blockEnd = start, start+end, start+end+len(endTag)
		}
		if matchedName == "" {
			return nil, nil, fmt.Errorf("expected a single parameter object followed only by complete declared parameter AITAG blocks")
		}
		if seenBlocks[matchedName] {
			return nil, nil, fmt.Errorf("multiple AITAG blocks for parameter %q", matchedName)
		}
		seenBlocks[matchedName] = true
		blocks = append(blocks, toolParamAITagBlock{ParamName: matchedName, Nonce: nonce, Content: normalizeAITAGBlockContent(remaining[contentStart:contentEnd])})
		remaining = strings.TrimSpace(remaining[blockEnd:])
	}
	return root, blocks, nil
}

func trimToolParamFence(raw string) string {
	for strings.HasPrefix(raw, "```") {
		line, rest, _ := strings.Cut(raw, "\n")
		switch strings.TrimSpace(line) {
		case "```", "```json", "```text", "```jsonschema":
			raw = strings.TrimSpace(rest)
		default:
			return raw
		}
	}
	return raw
}

func isToolParamEnvelopeKey(key string) bool {
	switch key {
	case "@action", "action", "tool", "params", "identifier", "call_expectations":
		return true
	default:
		return false
	}
}

// normalizeFixedToolParamEnvelope is exclusive to R2: the selected tool is
// authoritative. Recovery changes the envelope, never the selected tool or
// a business value. Reserved business names must be nested inside params.
func normalizeFixedToolParamEnvelope(root aitool.InvokeParams, tool *aitool.Tool) (aitool.InvokeParams, error) {
	properties := tool.Params()
	params, wrapped := root["params"]
	if wrapped {
		_, hasAction := actionMarker(root)
		_, hasTool := root["tool"]
		if properties.Have("params") && !hasAction && !hasTool {
			return nil, fmt.Errorf("ambiguous business parameter named params; use an explicit call-tool envelope")
		}
		if marker, exists := actionMarker(root); exists && marker != "call-tool" {
			return nil, fmt.Errorf("invalid action for fixed tool %q: requested=%q, expected call-tool", tool.Name, marker)
		}
		if requested, exists := root["tool"]; exists && requested != tool.Name {
			return nil, fmt.Errorf("parameter tool identity does not match selected tool %q", tool.Name)
		}
		object, ok := params.(map[string]any)
		if !ok || object == nil {
			return nil, fmt.Errorf("params for fixed tool %q must be an object", tool.Name)
		}
		for key := range root {
			if !isToolParamEnvelopeKey(key) && properties.Have(key) {
				return nil, fmt.Errorf("tool parameter %q is outside params; put all tool parameters inside params", key)
			}
		}
		// Preserve legacy diagnostic metadata in the envelope/raw response.
		// Unknown outer fields are never forwarded as tool parameters.
		for _, key := range []string{"identifier", "call_expectations"} {
			if value, exists := root[key]; exists {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("parameter envelope field %q must be a string", key)
				}
			}
		}
	} else {
		for key := range root {
			if isToolParamEnvelopeKey(key) || !properties.Have(key) {
				return nil, fmt.Errorf("ambiguous bare parameter field %q; use the call-tool params envelope", key)
			}
		}
		// Even a parameterless tool must actually return {}, rather than having
		// an empty response inferred as an empty set of arguments.
		params = map[string]any(root)
		root = make(aitool.InvokeParams)
	}
	root[ActionMagicKey] = "call-tool"
	root["tool"] = tool.Name
	root["params"] = params
	return root, nil
}

type fixedToolParamResponse struct {
	Envelope aitool.InvokeParams
	Raw      string
	Blocks   []toolParamAITagBlock
}

func extractFixedToolParamResponse(ctx context.Context, stream io.Reader, tool *aitool.Tool, opts ...ActionMakerOption) (*fixedToolParamResponse, error) {
	if tool == nil || tool.Tool == nil || tool.Name == "" {
		return nil, fmt.Errorf("parameter generation requires a selected tool")
	}
	var raw bytes.Buffer
	action, err := ExtractActionFromStream(ctx, io.TeeReader(stream, &raw), "call-tool", opts...)
	if err != nil {
		return nil, err
	}
	if err = action.WaitParseResult(ctx); err != nil {
		return nil, err
	}
	action.WaitStream(ctx)
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	response := raw.String()
	root, blocks, err := decodeToolParamObject(response, tool.Params().Keys())
	if err != nil {
		return nil, err
	}
	canonical, err := normalizeFixedToolParamEnvelope(root, tool)
	if err != nil {
		return nil, err
	}
	return &fixedToolParamResponse{Envelope: canonical, Raw: response, Blocks: blocks}, nil
}

// Only syntactic blocks after the JSON object can supply parameters. Tag-like
// text inside a JSON string remains literal data, including the current nonce.
func mergeFixedToolParamBlocks(params aitool.InvokeParams, blocks []toolParamAITagBlock, meta *ToolParamsPromptMeta) error {
	if len(blocks) == 0 {
		return nil
	}
	if meta == nil || meta.Nonce == "" {
		return fmt.Errorf("parameter AITAG requires the current prompt nonce")
	}
	allowed := make(map[string]bool, len(meta.ParamNames))
	for _, name := range meta.ParamNames {
		allowed[name] = true
	}
	exact := 0
	for _, block := range blocks {
		if !allowed[block.ParamName] {
			return fmt.Errorf("parameter AITAG %q was not offered in this prompt", block.ParamName)
		}
		if block.Nonce == meta.Nonce {
			params[block.ParamName] = block.Content
			exact++
		}
	}
	// Preserve the existing single stale-nonce fallback, without allowing it
	// to overwrite a non-empty JSON proposal or mix with exact-nonce blocks.
	if exact == 0 && len(blocks) == 1 {
		block := blocks[0]
		if block.Content != "" && params.GetString(block.ParamName) == "" {
			params[block.ParamName] = block.Content
		}
	}
	return nil
}

// Only exact JSON boolean literals in a declared boolean field are coerced.
// String fields, unions, numbers and objects retain their original values.
func normalizeToolParamBooleans(tool *aitool.Tool, params aitool.InvokeParams) {
	properties := tool.Params()
	for key, value := range params {
		text, ok := value.(string)
		if !ok || (text != "true" && text != "false") {
			continue
		}
		property, exists := properties.Get(key)
		if exists && utils.MapGetString(utils.InterfaceToGeneralMap(property), "type") == "boolean" {
			params[key] = text == "true"
		}
	}
}
