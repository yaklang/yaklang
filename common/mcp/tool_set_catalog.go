package mcp

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/mcpcatalog"
)

// MCPToolSetTier classifies legacy MCP tool sets for default startup selection.
type MCPToolSetTier = mcpcatalog.Tier

const (
	ToolSetTierDefault  = mcpcatalog.TierDefault
	ToolSetTierOptional = mcpcatalog.TierOptional
	ToolSetTierInternal = mcpcatalog.TierInternal
)

// MCPToolSetCatalogEntry documents one registered legacy tool set.
type MCPToolSetCatalogEntry struct {
	Name    string
	Tier    MCPToolSetTier
	Summary string
}

func toMCPToolSetCatalogEntry(entry mcpcatalog.Entry) MCPToolSetCatalogEntry {
	return MCPToolSetCatalogEntry{
		Name:    entry.Name,
		Tier:    entry.Tier,
		Summary: entry.Summary,
	}
}

// MCPToolSetCatalog returns a copy of the tool-set catalog entries.
func MCPToolSetCatalog() []MCPToolSetCatalogEntry {
	entries := mcpcatalog.Catalog()
	out := make([]MCPToolSetCatalogEntry, len(entries))
	for i, entry := range entries {
		out[i] = toMCPToolSetCatalogEntry(entry)
	}
	return out
}

// MCPToolSetTierOf returns the catalog tier for a tool set, or false if unknown.
func MCPToolSetTierOf(name string) (MCPToolSetTier, bool) {
	return mcpcatalog.TierOf(name)
}

// IsDefaultMCPToolSet reports whether name is enabled on default MCP startup.
func IsDefaultMCPToolSet(name string) bool {
	return mcpcatalog.IsDefaultToolSet(name)
}

// IsDefaultBuiltinTool reports whether a legacy builtin tool belongs to a default-tier tool set.
func IsDefaultBuiltinTool(toolName string) bool {
	setName, ok := BuiltinToolSetOf(toolName)
	if !ok {
		return false
	}
	return IsDefaultMCPToolSet(setName)
}

// BuiltinToolSetOf returns the tool set that owns a legacy builtin tool name.
func BuiltinToolSetOf(toolName string) (string, bool) {
	for setName, set := range globalToolSets {
		if _, ok := set.Tools[toolName]; ok {
			return setName, true
		}
	}
	return "", false
}

// CatalogDefaultMCPToolSets are factory defaults from the built-in catalog tier.
var CatalogDefaultMCPToolSets = mcpcatalog.DefaultToolSetNames()

// DefaultMCPToolSets is an alias of CatalogDefaultMCPToolSets for legacy callers/tests.
// Runtime startup should use yakit.EffectiveDefaultMCPToolSets(profileDB) when available.
var DefaultMCPToolSets = CatalogDefaultMCPToolSets

// CatalogDefaultMCPResourceSets are factory default resource sets.
var CatalogDefaultMCPResourceSets = mcpcatalog.DefaultResourceSetNames()

// DefaultMCPResourceSets is an alias of CatalogDefaultMCPResourceSets.
var DefaultMCPResourceSets = CatalogDefaultMCPResourceSets

// OptionalMCPToolSets are registered but not enabled by default; use -t or --enable-all.
var OptionalMCPToolSets = mcpcatalog.OptionalToolSetNames()

// InternalMCPToolSets are not recommended for routine MCP exposure.
var InternalMCPToolSets = mcpcatalog.InternalToolSetNames()

// TierName returns the wire/string tier label for API responses.
func TierName(tier MCPToolSetTier) string {
	return mcpcatalog.TierName(tier)
}

// CatalogEntryByName returns catalog metadata for a tool set.
func CatalogEntryByName(name string) (MCPToolSetCatalogEntry, bool) {
	entry, ok := mcpcatalog.EntryByName(name)
	if !ok {
		return MCPToolSetCatalogEntry{}, false
	}
	return toMCPToolSetCatalogEntry(entry), true
}

// ValidateToolSetNames returns an error if any name is unknown.
func ValidateToolSetNames(names []string) error {
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := globalToolSets[name]; !ok {
			return fmt.Errorf("undefined tool set: %s", name)
		}
	}
	return nil
}

// ToolNamesInSet returns sorted tool names registered under a tool set.
func ToolNamesInSet(setName string) []string {
	set, ok := globalToolSets[setName]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(set.Tools))
	for name := range set.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultMCPToolCount returns how many legacy tools catalog factory defaults expose.
func DefaultMCPToolCount() int {
	return MCPToolCountForSets(CatalogDefaultMCPToolSets)
}

// MCPToolCountForSets returns how many legacy tools the given tool sets expose.
func MCPToolCountForSets(setNames []string) int {
	n := 0
	for _, setName := range setNames {
		if set, ok := globalToolSets[setName]; ok {
			n += len(set.Tools)
		}
	}
	return n
}

// AllMCPToolSetNames returns every cataloged tool set name in registration order.
func AllMCPToolSetNames() []string {
	return mcpcatalog.AllToolSetNames()
}
