package format

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

const yakdocMaxSimilarMembers = 15

var externFieldErrorRe = regexp.MustCompile(`Extern(?:Lib|Type) \[([^\]]+)\] don't has \[([^\]]+)\](?:, maybe you meant ([^?]+) \?)?`)

type externFieldErrorInfo struct {
	LibName      string
	WrongKey     string
	SuggestedKey string
}

func parseExternFieldError(message string) (*externFieldErrorInfo, bool) {
	m := externFieldErrorRe.FindStringSubmatch(message)
	if len(m) < 3 {
		return nil, false
	}
	info := &externFieldErrorInfo{
		LibName:  strings.TrimSpace(m[1]),
		WrongKey: strings.TrimSpace(m[2]),
	}
	if len(m) >= 4 {
		info.SuggestedKey = strings.TrimSpace(m[3])
	}
	return info, true
}

func rankSimilarNames(names []string, target string, max int) []string {
	if target == "" || len(names) == 0 {
		return nil
	}
	type scored struct {
		name  string
		score float64
	}
	scores := make([]scored, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		scores = append(scores, scored{
			name:  name,
			score: utils.CalcSimilarity([]byte(target), []byte(name)),
		})
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})
	if max <= 0 {
		max = yakdocMaxSimilarMembers
	}
	result := make([]string, 0, max)
	seen := make(map[string]struct{})
	for _, item := range scores {
		if len(result) >= max {
			break
		}
		if _, ok := seen[item.name]; ok {
			continue
		}
		seen[item.name] = struct{}{}
		result = append(result, item.name)
	}
	return result
}

// EnrichExternFieldError auto-attaches yakdoc context for ExternLib/ExternType member errors.
func EnrichExternFieldError(errorMessage string) string {
	info, ok := parseExternFieldError(errorMessage)
	if !ok || info.LibName == "" {
		return ""
	}

	funcs, vars := doc.LibMemberNames(info.LibName)
	if len(funcs) == 0 && len(vars) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("【已自动附加 YakDocument】系统已查询标准库文档，请直接基于下列 API 修改代码，禁止继续猜测。\n\n")
	buf.WriteString(fmt.Sprintf("库 %s 不存在成员 %s", info.LibName, info.WrongKey))
	if info.SuggestedKey != "" {
		buf.WriteString(fmt.Sprintf("；编译器建议：%s", info.SuggestedKey))
	}
	buf.WriteString("\n\n")

	similarFuncs := rankSimilarNames(funcs, info.WrongKey, yakdocMaxSimilarMembers)
	if info.SuggestedKey != "" {
		similarFuncs = prependUnique(similarFuncs, info.SuggestedKey)
	}
	if len(similarFuncs) > 0 {
		buf.WriteString(fmt.Sprintf("## %s 相近函数 (top %d)\n", info.LibName, len(similarFuncs)))
		for _, name := range similarFuncs {
			marker := ""
			if name == info.SuggestedKey {
				marker = " ← 推荐"
			}
			buf.WriteString(fmt.Sprintf("- %s%s\n", name, marker))
		}
		buf.WriteString("\n")
	}

	similarVars := rankSimilarNames(vars, info.WrongKey, 8)
	if len(similarVars) > 0 {
		buf.WriteString(fmt.Sprintf("## %s 相近变量 (top %d)\n", info.LibName, len(similarVars)))
		for _, name := range similarVars {
			buf.WriteString(fmt.Sprintf("- %s\n", name))
		}
		buf.WriteString("\n")
	}

	if info.SuggestedKey != "" {
		if fn := doc.GetFunctionDecl(info.LibName, info.SuggestedKey); fn != nil {
			buf.WriteString("## 建议函数详情\n")
			buf.WriteString(fn.String())
			buf.WriteString("\n")
		} else if inst := doc.GetDocumentInstance(info.LibName, info.SuggestedKey); inst != nil {
			buf.WriteString("## 建议变量详情\n")
			buf.WriteString(inst.String())
			buf.WriteString("\n")
		}
	}

	return strings.TrimSpace(buf.String())
}

func prependUnique(items []string, head string) []string {
	if head == "" {
		return items
	}
	out := []string{head}
	for _, item := range items {
		if item != head {
			out = append(out, item)
		}
	}
	return out
}

func safeGlobMatch(text, pattern string) bool {
	defer func() {
		_ = recover()
	}()
	return utils.MatchAnyOfGlob(text, pattern)
}
