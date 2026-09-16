package syntaxflowdoc

import (
	"fmt"
	"strings"
)

// FormatNativeCallDetails formats one or more NativeCall docs.
func FormatNativeCallDetails(entries []*DocEntry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[SyntaxFlowDoc] %d native_call detail(s)\n\n", len(entries)))
	for i, e := range entries {
		if e == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. <%s>\n", i+1, e.Name))
		if e.Description != "" {
			b.WriteString("   desc: " + e.Description + "\n")
		}
		if e.Example != "" {
			b.WriteString("   example: " + e.Example + "\n")
		}
	}
	return b.String()
}

// FormatBuiltinLibDetails formats include-library docs.
func FormatBuiltinLibDetails(entries []*DocEntry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[SyntaxFlowDoc] %d builtin_lib detail(s)\n\n", len(entries)))
	for i, e := range entries {
		if e == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, e.Name))
		if e.Language != "" {
			b.WriteString("   language: " + e.Language + "\n")
		}
		if e.Path != "" {
			b.WriteString("   path: " + e.Path + "\n")
		}
		if e.Description != "" {
			b.WriteString("   desc: " + e.Description + "\n")
		}
		if e.Example != "" {
			b.WriteString("   example: " + e.Example + "\n")
		}
	}
	return b.String()
}

// FormatNameList formats a simple numbered name list.
func FormatNameList(kind string, names []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[SyntaxFlowDoc] %d %s\n\n", len(names), kind))
	for i, name := range names {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, name))
	}
	return b.String()
}

// ResolveNativeCalls looks up NativeCall docs by name; missing names are reported.
func ResolveNativeCalls(h *DocumentHelper, names []string) (found []*DocEntry, missing []string) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if e := h.GetNativeCall(name); e != nil {
			found = append(found, e)
		} else {
			missing = append(missing, name)
		}
	}
	return found, missing
}

// ResolveBuiltinLibs looks up include libs by name.
func ResolveBuiltinLibs(h *DocumentHelper, names []string) (found []*DocEntry, missing []string) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if e := h.GetBuiltinLib(name); e != nil {
			found = append(found, e)
		} else {
			missing = append(missing, name)
		}
	}
	return found, missing
}
