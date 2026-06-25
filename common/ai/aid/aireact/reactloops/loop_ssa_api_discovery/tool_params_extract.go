package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// extractLoopActionToolParams pulls Yak-tool-ready params from a loop action.
// Models often nest under "params": {...} or mix metadata keys that must not reach the tool.
func extractLoopActionToolParams(action *aicommon.Action) aitool.InvokeParams {
	if action == nil {
		return aitool.InvokeParams{}
	}
	all := action.GetParams()
	if len(all) == 0 {
		return aitool.InvokeParams{}
	}
	if p, ok := all["params"].(map[string]any); ok && len(p) > 0 {
		return aitool.InvokeParams(p)
	}
	skip := map[string]struct{}{
		aicommon.ActionMagicKey:    {},
		"tool":                     {},
		"params":                   {},
		"call_expectations":        {},
		"identifier":               {},
		"human_readable_thought": {},
	}
	out := make(aitool.InvokeParams)
	for k, v := range all {
		if _, bad := skip[k]; bad {
			continue
		}
		out[k] = v
	}
	return out
}

func coercePathsValue(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return ""
		}
		if strings.HasPrefix(s, "[") {
			var arr []string
			if err := json.Unmarshal([]byte(s), &arr); err == nil {
				return strings.Join(trimNonEmptyLines(arr), "\n")
			}
			var anyArr []any
			if err := json.Unmarshal([]byte(s), &anyArr); err == nil {
				var lines []string
				for _, x := range anyArr {
					line := strings.TrimSpace(utils.InterfaceToString(x))
					if line != "" {
						lines = append(lines, line)
					}
				}
				return strings.Join(lines, "\n")
			}
		}
		return s
	case []any:
		var lines []string
		for _, x := range t {
			line := strings.TrimSpace(utils.InterfaceToString(x))
			if line != "" {
				lines = append(lines, line)
			}
		}
		return strings.Join(lines, "\n")
	case []string:
		return strings.Join(trimNonEmptyLines(t), "\n")
	default:
		return strings.TrimSpace(utils.InterfaceToString(t))
	}
}

func trimNonEmptyLines(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func pickStringParam(p aitool.InvokeParams, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k]; ok {
			s := strings.TrimSpace(utils.InterfaceToString(v))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

// augmentBatchDoHTTPParams maps legacy keys, fills base-url from discovery session, and normalizes paths for batch_do_http_request.yak.
func augmentBatchDoHTTPParams(rt *Runtime, raw aitool.InvokeParams) aitool.InvokeParams {
	out := make(aitool.InvokeParams, len(raw)+4)
	for k, v := range raw {
		out[k] = v
	}

	paths := coercePathsValue(out["paths"])
	if paths == "" {
		paths = coercePathsValue(out["path"])
	}
	if paths == "" {
		paths = coercePathsValue(out["requests"])
		delete(out, "requests")
	}
	if paths == "" {
		paths = coercePathsValue(out["urls"])
		delete(out, "urls")
	}
	if paths != "" {
		out["paths"] = paths
	}
	delete(out, "requests")
	delete(out, "urls")
	delete(out, "path")

	base := pickStringParam(out, "base-url", "base_url", "baseUrl")
	if base == "" && rt != nil && rt.Session != nil {
		base = EffectiveTargetBaseURL(rt.Session)
	}
	if base != "" {
		out["base-url"] = base
	}
	delete(out, "base_url")
	delete(out, "baseUrl")

	if _, ok := out["concurrent"]; !ok {
		out["concurrent"] = 4
	}
	if _, ok := out["timeout"]; !ok {
		out["timeout"] = 15
	}
	return out
}

func batchDoHTTPParamsMissingMessage(p aitool.InvokeParams) string {
	paths := strings.TrimSpace(utils.InterfaceToString(p["paths"]))
	base := pickStringParam(p, "base-url", "base_url", "baseUrl")
	if paths == "" {
		return "缺少 paths：每行一条相对路径（如 /login）或一行完整 URL；不要使用不存在的 requests 键。"
	}
	if !strings.Contains(paths, "://") && base == "" {
		return "相对 paths 需要 base-url（可由会话自动填充）；若 discovery_get_status 中 target_base_url 为空请先 discovery_set_target。"
	}
	return ""
}

// augmentDoHTTPParams normalizes common AI param aliases and infers Content-Type for POST bodies.
// Returns optional feedback lines describing auto-corrections (for agent visibility).
func augmentDoHTTPParams(raw aitool.InvokeParams) (aitool.InvokeParams, []string) {
	out := make(aitool.InvokeParams, len(raw)+4)
	for k, v := range raw {
		out[k] = v
	}
	var notes []string

	aliasKeys := []struct{ from, to string }{
		{"content_type", "content-type"},
		{"contentType", "content-type"},
		{"post_params", "post-params"},
		{"postParams", "post-params"},
		{"query_params", "query-params"},
		{"queryParams", "query-params"},
		{"redirect_times", "redirect-times"},
		{"redirectTimes", "redirect-times"},
		{"show_request", "show-request"},
		{"showRequest", "show-request"},
	}
	for _, a := range aliasKeys {
		if v, ok := out[a.from]; ok {
			if _, exists := out[a.to]; !exists || strings.TrimSpace(utils.InterfaceToString(out[a.to])) == "" {
				out[a.to] = v
				if a.from != a.to {
					notes = append(notes, fmt.Sprintf("normalized param %q -> %q", a.from, a.to))
				}
			}
			delete(out, a.from)
		}
	}

	method := strings.ToUpper(strings.TrimSpace(pickStringParam(out, "method")))
	if method == "" {
		method = "GET"
	}
	if method != "POST" && method != "PUT" && method != "PATCH" {
		return out, notes
	}

	ct := strings.TrimSpace(pickStringParam(out, "content-type"))
	postParams := strings.TrimSpace(pickStringParam(out, "post-params"))
	body := strings.TrimSpace(pickStringParam(out, "body"))

	if postParams != "" && ct == "" {
		out["content-type"] = "application/x-www-form-urlencoded"
		notes = append(notes, "inferred content-type=application/x-www-form-urlencoded (post-params present)")
		ct = "application/x-www-form-urlencoded"
	}

	if ct == "" && body != "" {
		if inferred := inferContentTypeFromBody(body); inferred != "" {
			out["content-type"] = inferred
			notes = append(notes, "inferred content-type="+inferred+" from request body shape")
			ct = inferred
		}
	}

	if ct == "" && looksLikeFormBody(body) && postParams == "" {
		out["post-params"] = body
		delete(out, "body")
		out["content-type"] = "application/x-www-form-urlencoded"
		notes = append(notes, "moved form-like body -> post-params and set content-type=application/x-www-form-urlencoded")
	}

	return out, notes
}

func inferContentTypeFromBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
		return "application/json"
	}
	if looksLikeFormBody(body) {
		return "application/x-www-form-urlencoded"
	}
	return ""
}

func looksLikeFormBody(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
		return false
	}
	if !strings.Contains(body, "=") {
		return false
	}
	// k=v&k2=v2 style
	for _, part := range strings.Split(body, "&") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Index(part, "=") <= 0 {
			return false
		}
	}
	return true
}

func formatDoHTTPParamNormalizationHint(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## do_http_request param normalization\n")
	b.WriteString("The following were auto-corrected before sending (use CLI names next time):\n")
	for _, n := range notes {
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteString("\n")
	}
	b.WriteString("- Yak CLI key is **`content-type`** (hyphen), not `content_type`.\n")
	b.WriteString("- Form login: `content-type=application/x-www-form-urlencoded` + **`post-params`** (preferred) or `body`.\n")
	b.WriteString("- JSON login: `content-type=application/json` + **`body`** with JSON text.\n")
	b.WriteString("- Use **`show-request=yes`** once to verify `Content-Type` and body appear in the request packet.\n")
	return b.String()
}
