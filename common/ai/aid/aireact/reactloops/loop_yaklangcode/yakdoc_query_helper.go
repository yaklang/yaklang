package loop_yaklangcode

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

const yakdocMaxNameListItems = 200

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
