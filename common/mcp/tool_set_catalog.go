package mcp

import "sort"

// MCPToolSetTier classifies legacy MCP tool sets for default startup selection.
type MCPToolSetTier int

const (
	// ToolSetTierDefault — high-frequency pentest / audit workflows; enabled when no -t/--tool is given.
	ToolSetTierDefault MCPToolSetTier = iota
	// ToolSetTierOptional — specialized, long-running, or Yakit-UI-centric; enable with -t or --enable-all.
	ToolSetTierOptional
	// ToolSetTierInternal — meta/runtime hooks; not recommended for default MCP exposure.
	ToolSetTierInternal
)

// MCPToolSetCatalogEntry documents one registered legacy tool set.
type MCPToolSetCatalogEntry struct {
	Name    string
	Tier    MCPToolSetTier
	Summary string
}

// mcpToolSetCatalog is the authoritative inventory and default-startup classification.
// Order mirrors legacy registration order in MCPCommandUsage.
var mcpToolSetCatalog = []MCPToolSetCatalogEntry{
	// --- default: daily pentest / traffic / vuln workflows ---
	{Name: "codec", Tier: ToolSetTierDefault, Summary: "Encode/decode and fuzztag rendering; used in almost every payload workflow"},
	{Name: "cve", Tier: ToolSetTierDefault, Summary: "CVE vulnerability database lookup"},
	{Name: "httpflow", Tier: ToolSetTierDefault, Summary: "HTTP history query, tagging, and cleanup"},
	{Name: "port_scan", Tier: ToolSetTierDefault, Summary: "Port scan and port-result management (can be slow; still high-frequency)"},
	{Name: "reverse_shell", Tier: ToolSetTierDefault, Summary: "Generate reverse-shell commands; pairs with reverse_platform"},
	{Name: "reverse_platform", Tier: ToolSetTierDefault, Summary: "Global reverse, DNSLog, random port, bridge log — core OOB workflow"},
	{Name: "http_fuzzer", Tier: ToolSetTierDefault, Summary: "Web fuzzer requests and fuzzer tab creation"},
	{Name: "risk", Tier: ToolSetTierDefault, Summary: "Vulnerability/risk records from scans, MITM, plugins, and OOB"},
	{Name: "yso", Tier: ToolSetTierDefault, Summary: "Java deserialization (YSO) gadget/class payload generation"},
	{Name: "syntaxflow", Tier: ToolSetTierDefault, Summary: "SyntaxFlow rule management and code-audit scans"},
	{Name: "mitm", Tier: ToolSetTierDefault, Summary: "MITM proxy filters, replacer rules, cert download, start_mitm_v2"},
	{Name: "fingerprint", Tier: ToolSetTierDefault, Summary: "Service fingerprint query and CRUD"},

	// --- optional: specialized / heavy / UI-oriented ---
	{Name: "hybrid_scan", Tier: ToolSetTierOptional, Summary: "Combined hybrid scan; long-running, task-oriented"},
	{Name: "payload", Tier: ToolSetTierOptional, Summary: "Payload dictionary CRUD; Yakit dictionary UI workflow"},
	{Name: "yak_document", Tier: ToolSetTierOptional, Summary: "Yak API/library documentation lookup for script authors"},
	{Name: "yak_script", Tier: ToolSetTierOptional, Summary: "Yak script query/group/online sync; exec_yak_script hidden over SSE"},
	{Name: "brute", Tier: ToolSetTierOptional, Summary: "Credential brute force; scenario-specific"},
	{Name: "subdomain", Tier: ToolSetTierOptional, Summary: "Subdomain collection; recon-specific"},
	{Name: "crawler", Tier: ToolSetTierOptional, Summary: "Web crawler; recon-specific"},
	{Name: "ssa", Tier: ToolSetTierOptional, Summary: "SSA compile/query; overlaps syntaxflow for static analysis"},
	{Name: "project_database", Tier: ToolSetTierOptional, Summary: "Project database list/switch/create; session setup"},
	{Name: "global_hotpatch", Tier: ToolSetTierOptional, Summary: "Global MITM hotpatch templates; advanced MITM"},
	{Name: "system_proxy", Tier: ToolSetTierOptional, Summary: "OS system proxy get/set; edge-case environment setup"},

	// --- internal ---
	{Name: "dynamic", Tier: ToolSetTierInternal, Summary: "Runtime dynamic tool registration; meta hook"},
}

// MCPToolSetCatalog returns a copy of the tool-set catalog entries.
func MCPToolSetCatalog() []MCPToolSetCatalogEntry {
	out := make([]MCPToolSetCatalogEntry, len(mcpToolSetCatalog))
	copy(out, mcpToolSetCatalog)
	return out
}

// MCPToolSetTierOf returns the catalog tier for a tool set, or false if unknown.
func MCPToolSetTierOf(name string) (MCPToolSetTier, bool) {
	for _, entry := range mcpToolSetCatalog {
		if entry.Name == name {
			return entry.Tier, true
		}
	}
	return 0, false
}

// IsDefaultMCPToolSet reports whether name is enabled on default MCP startup.
func IsDefaultMCPToolSet(name string) bool {
	tier, ok := MCPToolSetTierOf(name)
	return ok && tier == ToolSetTierDefault
}

func toolSetNamesByTier(tier MCPToolSetTier) []string {
	names := make([]string, 0)
	for _, entry := range mcpToolSetCatalog {
		if entry.Tier == tier {
			names = append(names, entry.Name)
		}
	}
	return names
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

// DefaultMCPToolSets are enabled when no -t/--tool is given (CLI) or StartMcpServer
// is called without EnableAll and an empty Tool list (gRPC/Yakit).
// Derived from catalog entries marked ToolSetTierDefault.
var DefaultMCPToolSets = toolSetNamesByTier(ToolSetTierDefault)

// OptionalMCPToolSets are registered but not enabled by default; use -t or --enable-all.
var OptionalMCPToolSets = toolSetNamesByTier(ToolSetTierOptional)

// InternalMCPToolSets are not recommended for routine MCP exposure.
var InternalMCPToolSets = toolSetNamesByTier(ToolSetTierInternal)

// DefaultMCPResourceSets are enabled together with DefaultMCPToolSets.
var DefaultMCPResourceSets = []string{
	"codec",
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

// DefaultMCPToolCount returns how many legacy tools default startup exposes.
func DefaultMCPToolCount() int {
	n := 0
	for _, setName := range DefaultMCPToolSets {
		if set, ok := globalToolSets[setName]; ok {
			n += len(set.Tools)
		}
	}
	return n
}

// AllMCPToolSetNames returns every cataloged tool set name in registration order.
func AllMCPToolSetNames() []string {
	names := make([]string, len(mcpToolSetCatalog))
	for i, entry := range mcpToolSetCatalog {
		names[i] = entry.Name
	}
	return names
}
