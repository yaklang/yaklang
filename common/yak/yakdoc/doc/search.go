package doc

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils/fuzzy"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

const defaultSearchLimit = 20
const maxSearchLimit = 64

// DocumentSearchHit is a ranked yakdoc search result.
type DocumentSearchHit struct {
	LibName     string
	Name        string
	Kind        string // "function" or "variable"
	DeclSnippet string
	DocSnippet  string
}

var (
	searchIndexOnce sync.Once
	fuzzySearchMap  map[string]*DocumentSearchHit
	fuzzySearchKeys []string
)

func ensureSearchIndex() {
	searchIndexOnce.Do(func() {
		fuzzySearchMap = make(map[string]*DocumentSearchHit)
		fuzzySearchKeys = make([]string, 0)

		helper := GetDefaultDocumentHelper()
		if helper == nil {
			return
		}

		for _, lib := range helper.Libs {
			if lib == nil {
				continue
			}
			for _, fn := range lib.Functions {
				if fn == nil {
					continue
				}
				addSearchHit(lib.Name, fn.MethodName, "function", fn.Decl, fn.Document)
			}
			for _, inst := range lib.Instances {
				if inst == nil {
					continue
				}
				decl := fmt.Sprintf("%s %s", inst.Type, inst.InstanceName)
				addSearchHit(lib.Name, inst.InstanceName, "variable", decl, inst.ValueStr)
			}
		}

		for name, fn := range helper.Functions {
			if fn == nil {
				continue
			}
			addSearchHit("GLOBAL", name, "function", fn.Decl, fn.Document)
		}
		for name, inst := range helper.Instances {
			if inst == nil {
				continue
			}
			decl := fmt.Sprintf("%s %s", inst.Type, inst.InstanceName)
			addSearchHit("GLOBAL", name, "variable", decl, inst.ValueStr)
		}
	})
}

func addSearchHit(libName, name, kind, decl, docText string) {
	decl = strings.TrimSpace(decl)
	docText = strings.TrimSpace(docText)
	key := strings.ToLower(fmt.Sprintf("%s.%s|%s %s", libName, name, decl, docText))
	if _, exists := fuzzySearchMap[key]; exists {
		return
	}
	hit := &DocumentSearchHit{
		LibName:     libName,
		Name:        name,
		Kind:        kind,
		DeclSnippet: truncateSnippet(decl, 160),
		DocSnippet:  truncateSnippet(docText, 200),
	}
	fuzzySearchMap[key] = hit
	fuzzySearchKeys = append(fuzzySearchKeys, key)
}

func truncateSnippet(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// SearchDocument performs fuzzy search across yakdoc libraries, function names, and descriptions.
func SearchDocument(query string, limit int, libraryFilter string) []*DocumentSearchHit {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	ensureSearchIndex()
	if len(fuzzySearchKeys) == 0 {
		return nil
	}

	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	keys := fuzzySearchKeys
	if libFilter := strings.TrimSpace(libraryFilter); libFilter != "" {
		prefix := strings.ToLower(libFilter) + "."
		filtered := make([]string, 0)
		for _, key := range keys {
			if strings.HasPrefix(key, prefix) {
				filtered = append(filtered, key)
			}
		}
		keys = filtered
	}
	if len(keys) == 0 {
		return nil
	}

	fuzzyResults := fuzzy.RankFindEx(
		strings.ToLower(query),
		keys,
		func(s1, s2 string) float64 {
			var counter float64
			var distance float64
			for _, word := range strings.Fields(s1) {
				if word == "" {
					continue
				}
				if strings.Contains(s2, word) {
					counter++
					distance += fuzzy.LevenshteinDistance(word, s2)
				}
			}
			if counter > 0 {
				return distance / counter
			}
			return math.MaxFloat64
		},
	)
	sort.Sort(fuzzyResults)

	results := make([]*DocumentSearchHit, 0, limit)
	seen := make(map[string]struct{})
	for i := 0; i < len(fuzzyResults) && len(results) < limit; i++ {
		if fuzzyResults[i].Distance == math.MaxFloat64 {
			continue
		}
		hit := fuzzySearchMap[fuzzyResults[i].Target]
		if hit == nil {
			continue
		}
		dedupeKey := hit.LibName + "." + hit.Name
		if _, ok := seen[dedupeKey]; ok {
			continue
		}
		seen[dedupeKey] = struct{}{}
		results = append(results, hit)
	}
	return results
}

// FormatSearchHit returns a one-line summary for a search hit.
func FormatSearchHit(hit *DocumentSearchHit) string {
	if hit == nil {
		return ""
	}
	line := fmt.Sprintf("%s.%s (%s)", hit.LibName, hit.Name, hit.Kind)
	if hit.DeclSnippet != "" {
		line += " — " + hit.DeclSnippet
	}
	if hit.DocSnippet != "" {
		line += "\n  " + hit.DocSnippet
	}
	return line
}

// LibMemberNames returns sorted function and variable names for a library.
func LibMemberNames(libName string) (functions []string, variables []string) {
	funcs := GetDocumentFunctions(libName)
	vars := GetDocumentInstances(libName)
	for name := range funcs {
		functions = append(functions, name)
	}
	for name := range vars {
		variables = append(variables, name)
	}
	sort.Strings(functions)
	sort.Strings(variables)
	return functions, variables
}

// GetFunctionDecl is a convenience wrapper returning nil when not found.
func GetFunctionDecl(libName, funcName string) *yakdoc.FuncDecl {
	return GetDocumentFunction(libName, funcName)
}
