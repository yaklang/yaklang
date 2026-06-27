// JSON 宽松提取（Markdown 围栏、action 别名键、jsonextractor 多候选），对齐 aispec / infosec 类的增强思路。
//
// ReAct 主循环与 VerifyUserSatisfaction 在 reactloops、aicommon、aireact 中解析 @action。
// 在「仅修改本 package」的前提下无法挂载到 exec.go 的解析链（子包无法改写 ReActLoop 未导出字段，
// 且本包不能 import aireact，否则会与 reactinit 形成 import cycle）。运行时使用需在引擎侧对
// ssa_api_discovery* 循环调用 ExtractValidActionFromReaderWithLooseFallback。
package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

// ExtractValidActionFromReaderWithLooseFallback 先走标准 ExtractValidActionFromStream，失败则对整段文本做宽松 JSON 提取。
// allowedNames 须与 ValidCheck 期望一致（一般为各 alias 后接 strictName，与 reactloops exec 中 append(alias, "object") 类似）。
func ExtractValidActionFromReaderWithLooseFallback(ctx context.Context, reader io.Reader, strictName string, allowedNames []string, opts ...aicommon.ActionMakerOption) (*aicommon.Action, error) {
	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	raw := string(b)
	act, err := aicommon.ExtractValidActionFromStream(ctx, strings.NewReader(raw), strictName, opts...)
	if err == nil {
		return act, nil
	}
	loose, err2 := extractValidActionFromLooseBlob(raw, allowedNames)
	if err2 == nil && loose != nil {
		return loose, nil
	}
	return nil, err
}

func normalizeLooseModelJSONBlob(blob string) string {
	s := strings.TrimSpace(blob)
	s = stripMarkdownJSONCodeFence(s)
	if strings.Contains(s, "，") {
		s = strings.ReplaceAll(s, "，", ",")
	}
	return strings.TrimSpace(s)
}

func stripMarkdownJSONCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	rest := strings.TrimPrefix(s, "```")
	if nl := strings.Index(rest, "\n"); nl >= 0 {
		rest = rest[nl+1:]
	} else {
		rest = strings.TrimLeft(rest, "`")
	}
	if idx := strings.LastIndex(rest, "```"); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.TrimSpace(rest)
}

func canonicalizeAtActionKey(m map[string]any) {
	if m == nil {
		return
	}
	if _, ok := m["@action"]; ok {
		return
	}
	for _, alt := range []string{"action", "Action", "tool_action"} {
		if v, ok := m[alt]; ok {
			m["@action"] = v
			return
		}
	}
}

func normalizeActionFieldString(v any) string {
	s := strings.TrimSpace(fmt.Sprint(v))
	s = strings.Trim(s, `"'`+"`")
	s = strings.Trim(s, "\u201c\u201d\u2018\u2019")
	return strings.TrimSpace(s)
}

func collectJSONCandidatesFromBlob(normalized string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(j string) {
		j = strings.TrimSpace(j)
		if j == "" || seen[j] {
			return
		}
		seen[j] = true
		out = append(out, j)
	}

	std, raw := jsonextractor.ExtractJSONWithRaw(normalized)
	for _, j := range std {
		add(j)
	}
	for _, j := range raw {
		add(j)
		add(string(jsonextractor.FixJson([]byte(j))))
	}
	for _, pair := range jsonextractor.ExtractObjectIndexes(normalized) {
		add(normalized[pair[0]:pair[1]])
	}
	return out
}

func actionFromUnmarshalledMap(expectName string, m map[string]any) *aicommon.Action {
	ac := aicommon.NewSimpleAction(expectName, nil)
	for k, v := range m {
		ac.ForceSet(k, v)
	}
	return ac
}

func parseActionFromJSONObjectJSON(allowedNames []string, jsonBytes []byte) (*aicommon.Action, error) {
	var m map[string]any
	if err := json.Unmarshal(jsonBytes, &m); err != nil {
		fixed := jsonextractor.FixJson(jsonBytes)
		if err2 := json.Unmarshal(fixed, &m); err2 != nil {
			return nil, err
		}
	}
	canonicalizeAtActionKey(m)
	raw, ok := m["@action"]
	if !ok {
		return nil, utils.Error("missing @action")
	}
	name := normalizeActionFieldString(raw)
	if name == "" || !utils.StringArrayContains(allowedNames, name) {
		return nil, utils.Errorf("action %q not in allowed set", name)
	}
	return actionFromUnmarshalledMap(name, m), nil
}

func extractValidActionFromLooseBlob(blob string, allowedNames []string) (*aicommon.Action, error) {
	if strings.TrimSpace(blob) == "" {
		return nil, utils.Error("empty blob")
	}
	norm := normalizeLooseModelJSONBlob(blob)
	if norm == "" {
		return nil, utils.Error("normalized blob empty")
	}
	var lastErr error
	for _, cand := range collectJSONCandidatesFromBlob(norm) {
		act, err := parseActionFromJSONObjectJSON(allowedNames, []byte(cand))
		if err != nil {
			lastErr = err
			continue
		}
		if act != nil && act.ValidCheck(allowedNames...) {
			return act, nil
		}
	}
	if lastErr != nil {
		return nil, utils.Errorf("loose extract: %v", lastErr)
	}
	return nil, utils.Error("loose extract: no candidate matched")
}
