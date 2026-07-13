package mcpcatalog

// Tier classifies legacy MCP tool sets for default startup selection.
type Tier int

const (
	// TierDefault — high-frequency pentest / audit workflows; enabled when no -t/--tool is given.
	TierDefault Tier = iota
	// TierOptional — specialized, long-running, or Yakit-UI-centric; enable with -t or --enable-all.
	TierOptional
	// TierInternal — meta/runtime hooks; not recommended for default MCP exposure.
	TierInternal
)

// Entry documents one registered legacy tool set.
type Entry struct {
	Name    string
	Tier    Tier
	Summary string
}

// catalog is the authoritative inventory and default-startup classification.
// Order mirrors legacy registration order in MCP command usage.
var catalog = []Entry{
	// --- default: daily pentest / traffic / vuln workflows ---
	{Name: "codec", Tier: TierDefault, Summary: "Encode/decode and fuzztag rendering; used in almost every payload workflow"},
	{Name: "cve", Tier: TierDefault, Summary: "CVE vulnerability database lookup"},
	{Name: "httpflow", Tier: TierDefault, Summary: "HTTP history query, tagging, and cleanup"},
	{Name: "port_scan", Tier: TierDefault, Summary: "Port scan and port-result management (can be slow; still high-frequency)"},
	{Name: "reverse_shell", Tier: TierDefault, Summary: "Generate reverse-shell commands; pairs with reverse_platform"},
	{Name: "reverse_platform", Tier: TierDefault, Summary: "Global reverse, DNSLog, random port, bridge log — core OOB workflow"},
	{Name: "http_fuzzer", Tier: TierDefault, Summary: "Web fuzzer requests and fuzzer tab creation"},
	{Name: "risk", Tier: TierDefault, Summary: "Vulnerability/risk records from scans, MITM, plugins, and OOB"},
	{Name: "yso", Tier: TierDefault, Summary: "Java deserialization (YSO) gadget/class payload generation"},
	{Name: "syntaxflow", Tier: TierDefault, Summary: "SyntaxFlow rule management and code-audit scans"},
	{Name: "mitm", Tier: TierDefault, Summary: "MITM proxy filters, replacer rules, cert download, start_mitm_v2"},
	{Name: "fingerprint", Tier: TierDefault, Summary: "Service fingerprint query and CRUD"},

	// --- optional: specialized / heavy / UI-oriented ---
	{Name: "hybrid_scan", Tier: TierOptional, Summary: "Combined hybrid scan; long-running, task-oriented"},
	{Name: "payload", Tier: TierOptional, Summary: "Payload dictionary CRUD; Yakit dictionary UI workflow"},
	{Name: "yak_document", Tier: TierOptional, Summary: "Yak API/library documentation lookup for script authors"},
	{Name: "yak_script", Tier: TierOptional, Summary: "Yak script query/group/online sync; exec_yak_script hidden over SSE"},
	{Name: "brute", Tier: TierOptional, Summary: "Credential brute force; scenario-specific"},
	{Name: "subdomain", Tier: TierOptional, Summary: "Subdomain collection; recon-specific"},
	{Name: "crawler", Tier: TierOptional, Summary: "Web crawler; recon-specific"},
	{Name: "ssa", Tier: TierOptional, Summary: "SSA compile/query; overlaps syntaxflow for static analysis"},
	{Name: "project_database", Tier: TierOptional, Summary: "Project database list/switch/create; session setup"},
	{Name: "global_hotpatch", Tier: TierOptional, Summary: "Global MITM hotpatch templates; advanced MITM"},
	{Name: "system_proxy", Tier: TierOptional, Summary: "OS system proxy get/set; edge-case environment setup"},

	// --- internal ---
	{Name: "dynamic", Tier: TierInternal, Summary: "Runtime dynamic tool registration; meta hook"},
}

// Catalog returns a copy of the tool-set catalog entries.
func Catalog() []Entry {
	out := make([]Entry, len(catalog))
	copy(out, catalog)
	return out
}

// TierOf returns the catalog tier for a tool set, or false if unknown.
func TierOf(name string) (Tier, bool) {
	for _, entry := range catalog {
		if entry.Name == name {
			return entry.Tier, true
		}
	}
	return 0, false
}

// IsDefaultToolSet reports whether name is enabled on factory-default MCP startup.
func IsDefaultToolSet(name string) bool {
	tier, ok := TierOf(name)
	return ok && tier == TierDefault
}

func toolSetNamesByTier(tier Tier) []string {
	names := make([]string, 0)
	for _, entry := range catalog {
		if entry.Tier == tier {
			names = append(names, entry.Name)
		}
	}
	return names
}

// DefaultToolSetNames are factory defaults from the built-in catalog tier.
func DefaultToolSetNames() []string {
	return toolSetNamesByTier(TierDefault)
}

// DefaultResourceSetNames are factory default resource sets.
func DefaultResourceSetNames() []string {
	return []string{"codec"}
}

// OptionalToolSetNames are registered but not enabled by factory default.
func OptionalToolSetNames() []string {
	return toolSetNamesByTier(TierOptional)
}

// InternalToolSetNames are not recommended for routine MCP exposure.
func InternalToolSetNames() []string {
	return toolSetNamesByTier(TierInternal)
}

// TierName returns the wire/string tier label for API responses.
func TierName(tier Tier) string {
	switch tier {
	case TierDefault:
		return "default"
	case TierOptional:
		return "optional"
	case TierInternal:
		return "internal"
	default:
		return "unknown"
	}
}

// EntryByName returns catalog metadata for a tool set.
func EntryByName(name string) (Entry, bool) {
	for _, entry := range catalog {
		if entry.Name == name {
			return entry, true
		}
	}
	return Entry{}, false
}

// AllToolSetNames returns every cataloged tool set name in registration order.
func AllToolSetNames() []string {
	names := make([]string, len(catalog))
	for i, entry := range catalog {
		names[i] = entry.Name
	}
	return names
}
