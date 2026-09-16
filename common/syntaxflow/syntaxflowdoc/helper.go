package syntaxflowdoc

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Category constants for SyntaxFlow documentation entries.
const (
	CategoryNativeCall = "native_call"
	CategoryOperator   = "operator"
	CategoryOpcode     = "opcode"
	CategoryDescKey    = "desc_key"
	CategoryBuiltinLib = "builtin_lib"
	CategorySyntax     = "syntax"
)

// DocEntry is one searchable SyntaxFlow language / library document item.
type DocEntry struct {
	Category    string   `json:"category"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description"`
	Example     string   `json:"example,omitempty"`
	Path        string   `json:"path,omitempty"` // source path for builtin libs
	Language    string   `json:"language,omitempty"`
}

// DocumentHelper is the in-memory SyntaxFlow documentation index (yakdoc analog).
type DocumentHelper struct {
	NativeCalls map[string]*DocEntry `json:"native_calls"`
	Operators   map[string]*DocEntry `json:"operators"`
	Opcodes     map[string]*DocEntry `json:"opcodes"`
	DescKeys    map[string]*DocEntry `json:"desc_keys"`
	BuiltinLibs map[string]*DocEntry `json:"builtin_libs"`
	Syntax      map[string]*DocEntry `json:"syntax"`
}

func NewEmptyDocumentHelper() *DocumentHelper {
	return &DocumentHelper{
		NativeCalls: make(map[string]*DocEntry),
		Operators:   make(map[string]*DocEntry),
		Opcodes:     make(map[string]*DocEntry),
		DescKeys:    make(map[string]*DocEntry),
		BuiltinLibs: make(map[string]*DocEntry),
		Syntax:      make(map[string]*DocEntry),
	}
}

// Counts returns per-category sizes for diagnostics.
func (h *DocumentHelper) Counts() map[string]int {
	if h == nil {
		return map[string]int{}
	}
	return map[string]int{
		CategoryNativeCall: len(h.NativeCalls),
		CategoryOperator:   len(h.Operators),
		CategoryOpcode:     len(h.Opcodes),
		CategoryDescKey:    len(h.DescKeys),
		CategoryBuiltinLib: len(h.BuiltinLibs),
		CategorySyntax:     len(h.Syntax),
	}
}

// Total returns total entry count.
func (h *DocumentHelper) Total() int {
	c := h.Counts()
	n := 0
	for _, v := range c {
		n += v
	}
	return n
}

// AllEntries flattens all categories for search indexing.
func (h *DocumentHelper) AllEntries() []*DocEntry {
	if h == nil {
		return nil
	}
	out := make([]*DocEntry, 0, h.Total())
	appendMap := func(m map[string]*DocEntry) {
		names := make([]string, 0, len(m))
		for name := range m {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if e := m[name]; e != nil {
				out = append(out, e)
			}
		}
	}
	appendMap(h.NativeCalls)
	appendMap(h.Operators)
	appendMap(h.Opcodes)
	appendMap(h.DescKeys)
	appendMap(h.BuiltinLibs)
	appendMap(h.Syntax)
	return out
}

// GetNativeCall returns a NativeCall doc by name (case-sensitive first, then case-insensitive).
func (h *DocumentHelper) GetNativeCall(name string) *DocEntry {
	if h == nil {
		return nil
	}
	if e, ok := h.NativeCalls[name]; ok {
		return e
	}
	lower := strings.ToLower(name)
	for k, e := range h.NativeCalls {
		if strings.ToLower(k) == lower {
			return e
		}
	}
	return nil
}

// GetBuiltinLib returns an include-library doc by name.
func (h *DocumentHelper) GetBuiltinLib(name string) *DocEntry {
	if h == nil {
		return nil
	}
	if e, ok := h.BuiltinLibs[name]; ok {
		return e
	}
	lower := strings.ToLower(name)
	for k, e := range h.BuiltinLibs {
		if strings.ToLower(k) == lower {
			return e
		}
	}
	return nil
}

// ListNativeCallNames returns sorted NativeCall names.
func (h *DocumentHelper) ListNativeCallNames() []string {
	if h == nil {
		return nil
	}
	names := make([]string, 0, len(h.NativeCalls))
	for name := range h.NativeCalls {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListBuiltinLibNames returns sorted include lib names.
func (h *DocumentHelper) ListBuiltinLibNames() []string {
	if h == nil {
		return nil
	}
	names := make([]string, 0, len(h.BuiltinLibs))
	for name := range h.BuiltinLibs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuildSelectionIndex builds a compact index for AI prompt injection.
func (h *DocumentHelper) BuildSelectionIndex() string {
	if h == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("[SyntaxFlowDoc] selection index\n")
	b.WriteString(fmt.Sprintf("- native_call: %d\n", len(h.NativeCalls)))
	b.WriteString(fmt.Sprintf("- operator/syntax: %d\n", len(h.Operators)+len(h.Syntax)))
	b.WriteString(fmt.Sprintf("- opcode filter: %d\n", len(h.Opcodes)))
	b.WriteString(fmt.Sprintf("- desc keys: %d\n", len(h.DescKeys)))
	b.WriteString(fmt.Sprintf("- builtin libs (include): %d\n", len(h.BuiltinLibs)))
	b.WriteString("Use syntaxflowdoc_search / syntaxflowdoc_native_call_details / syntaxflowdoc_builtin_lib_details to query.\n")
	return b.String()
}

// GetProjectPath returns yaklang repo root (same convention as yakdoc).
func GetProjectPath() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	abs, err := filepath.Abs(filepath.Join(filename, "../../../../"))
	if err != nil {
		return ""
	}
	return abs
}
