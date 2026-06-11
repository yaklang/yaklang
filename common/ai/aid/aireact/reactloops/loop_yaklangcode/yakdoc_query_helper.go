package loop_yaklangcode

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

const (
	yakdocMaxNameListItems   = 200
	yakdocDefaultSearchLimit = 20
	yakdocMaxSimilarMembers  = 15
)

var externFieldErrorRe = regexp.MustCompile(`Extern(?:Lib|Type) \[([^\]]+)\] don't has \[([^\]]+)\](?:, maybe you meant ([^?]+) \?)?`)

func displayLibName(libName string) string {
	if strings.TrimSpace(libName) == "" {
		return "GLOBAL"
	}
	return libName
}

// QueryAllLibraryNames returns sorted standard library names.
func QueryAllLibraryNames() ([]string, error) {
	helper := doc.GetDefaultDocumentHelper()
	if helper == nil || len(helper.Libs) == 0 {
		return nil, utils.Error("yak document helper is empty")
	}
	names := lo.Keys(helper.Libs)
	sort.Strings(names)
	return names, nil
}

// FormatAllLibraryNames formats library names for AI consumption.
func FormatAllLibraryNames(names []string) string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("[YakDocument] %d standard libraries\n\n", len(names)))
	for i, name := range names {
		buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, name))
	}
	return buf.String()
}

// QueryLibraryDetails returns function and variable name lists for each library.
// Empty libNames queries GLOBAL builtins once.
func QueryLibraryDetails(libNames []string) (map[string]libraryDetail, error) {
	if len(libNames) == 0 {
		libNames = []string{""}
	}
	results := make(map[string]libraryDetail, len(libNames))
	for _, name := range libNames {
		funcs := lo.Keys(doc.GetDocumentFunctions(name))
		vars := lo.Keys(doc.GetDocumentInstances(name))
		sort.Strings(funcs)
		sort.Strings(vars)
		if len(funcs) == 0 && len(vars) == 0 && strings.TrimSpace(name) != "" {
			return nil, utils.Errorf("library[%s] not found; use yakdoc_get_all_library_names to list libraries", name)
		}
		key := displayLibName(name)
		results[key] = libraryDetail{
			LibName:   name,
			Functions: funcs,
			Variables: vars,
		}
	}
	return results, nil
}

type libraryDetail struct {
	LibName   string
	Functions []string
	Variables []string
}

func FormatLibraryDetails(details map[string]libraryDetail) string {
	var buf strings.Builder
	buf.WriteString("[YakDocument] Library details\n\n")
	keys := lo.Keys(details)
	sort.Strings(keys)
	for _, key := range keys {
		item := details[key]
		buf.WriteString(fmt.Sprintf("## Library: %s\n", key))
		buf.WriteString(formatNameList("Functions", item.Functions))
		buf.WriteString(formatNameList("Variables", item.Variables))
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

func formatNameList(label string, names []string) string {
	total := len(names)
	truncated := names
	suffix := ""
	if total > yakdocMaxNameListItems {
		truncated = names[:yakdocMaxNameListItems]
		suffix = fmt.Sprintf("\n... (%d more; use yakdoc_function_details / yakdoc_variable_details)\n", total-yakdocMaxNameListItems)
	}
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("%s (%d):\n", label, total))
	for _, name := range truncated {
		buf.WriteString(fmt.Sprintf("  - %s\n", name))
	}
	buf.WriteString(suffix)
	return buf.String()
}

// QueryFunctionDetails returns function documentation entries.
func QueryFunctionDetails(libName string, funcNames []string) (map[string]*yakdoc.FuncDecl, error) {
	if len(funcNames) == 0 {
		return nil, utils.Error("missing argument: function")
	}
	results := make(map[string]*yakdoc.FuncDecl, len(funcNames))
	for _, funcName := range funcNames {
		f := doc.GetDocumentFunction(libName, funcName)
		if f == nil {
			available := lo.Keys(doc.GetDocumentFunctions(libName))
			sort.Strings(available)
			return nil, utils.Errorf(
				"function[%s.%s] not found; use yakdoc_library_details to list available function names (found: %v)",
				displayLibName(libName), funcName, truncateList(available, 20),
			)
		}
		results[funcName] = f
	}
	return results, nil
}

func FormatFunctionDetails(results map[string]*yakdoc.FuncDecl) string {
	var buf strings.Builder
	buf.WriteString("[YakDocument] Function details\n\n")
	keys := lo.Keys(results)
	sort.Strings(keys)
	for _, name := range keys {
		buf.WriteString(results[name].String())
		buf.WriteString("\n\n")
	}
	return strings.TrimSpace(buf.String())
}

// QueryVariableDetails returns variable/instance documentation entries.
func QueryVariableDetails(libName string, varNames []string) (map[string]*yakdoc.LibInstance, error) {
	if len(varNames) == 0 {
		return nil, utils.Error("missing argument: variable")
	}
	results := make(map[string]*yakdoc.LibInstance, len(varNames))
	for _, varName := range varNames {
		i := doc.GetDocumentInstance(libName, varName)
		if i == nil {
			available := lo.Keys(doc.GetDocumentInstances(libName))
			sort.Strings(available)
			return nil, utils.Errorf(
				"variable[%s.%s] not found; use yakdoc_library_details to list available variable names (found: %v)",
				displayLibName(libName), varName, truncateList(available, 20),
			)
		}
		results[varName] = i
	}
	return results, nil
}

func FormatVariableDetails(results map[string]*yakdoc.LibInstance) string {
	var buf strings.Builder
	buf.WriteString("[YakDocument] Variable details\n\n")
	keys := lo.Keys(results)
	sort.Strings(keys)
	for _, name := range keys {
		buf.WriteString(results[name].String())
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

func truncateList(items []string, max int) []string {
	if len(items) <= max {
		return items
	}
	return items[:max]
}

// SearchYakDocument searches yakdoc by keywords (function names, descriptions, library names).
func SearchYakDocument(query string, limit int, library string) ([]*doc.DocumentSearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, utils.Error("missing argument: query")
	}
	if limit <= 0 {
		limit = yakdocDefaultSearchLimit
	}
	hits := doc.SearchDocument(query, limit, library)
	if len(hits) == 0 {
		return nil, utils.Errorf("no yakdoc matches for query[%q]; try different keywords or yakdoc_get_all_library_names", query)
	}
	return hits, nil
}

// FormatSearchResults formats fuzzy search hits for AI consumption.
func FormatSearchResults(query string, hits []*doc.DocumentSearchHit) string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("[YakDocument Search] query=%q, %d hits\n\n", query, len(hits)))
	for i, hit := range hits {
		buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, doc.FormatSearchHit(hit)))
		if i < len(hits)-1 {
			buf.WriteString("\n")
		}
	}
	buf.WriteString("\n\n提示：对最相关的条目使用 yakdoc_function_details / yakdoc_variable_details 获取完整签名。")
	return buf.String()
}

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
