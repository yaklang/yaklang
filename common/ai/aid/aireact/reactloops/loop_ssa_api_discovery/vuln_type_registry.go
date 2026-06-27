package loop_ssa_api_discovery

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed vuln_type_registry.json
var vulnTypeRegistryJSON []byte

// VulnTypeDef mirrors vuln_batch_scan.yak vulnTypeRegistry entries (metadata only).
type VulnTypeDef struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Scope        string `json:"scope"`
	PassiveCheck bool   `json:"passive_check"`
}

var vulnTypeRegistry []VulnTypeDef

func init() {
	_ = json.Unmarshal(vulnTypeRegistryJSON, &vulnTypeRegistry)
}

// AllVulnTypeDefs returns the full registry (aligned with embedded vuln_batch_scan.yak).
func AllVulnTypeDefs() []VulnTypeDef {
	out := make([]VulnTypeDef, len(vulnTypeRegistry))
	copy(out, vulnTypeRegistry)
	return out
}

// AllVulnTypeIDs returns every vuln_type id required for deep-mining finalize gate.
func AllVulnTypeIDs() []string {
	ids := make([]string, 0, len(vulnTypeRegistry))
	for _, d := range vulnTypeRegistry {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	return ids
}

// VulnTypeDefByID looks up a registry entry.
func VulnTypeDefByID(id string) (VulnTypeDef, bool) {
	id = strings.TrimSpace(id)
	for _, d := range vulnTypeRegistry {
		if d.ID == id {
			return d, true
		}
	}
	return VulnTypeDef{}, false
}

// FormatVulnTypeRegistryForPrompt returns markdown list for agent playbooks.
func FormatVulnTypeRegistryForPrompt() string {
	var b strings.Builder
	b.WriteString("| vuln_type | name | scope | passive |\n|---|---|---|---|\n")
	for _, d := range vulnTypeRegistry {
		passive := "no"
		if d.PassiveCheck {
			passive = "yes"
		}
		b.WriteString("| `")
		b.WriteString(d.ID)
		b.WriteString("` | ")
		b.WriteString(d.Name)
		b.WriteString(" | ")
		b.WriteString(d.Scope)
		b.WriteString(" | ")
		b.WriteString(passive)
		b.WriteString(" |\n")
	}
	return b.String()
}

// ApplicableVulnTypeIDsForEndpoint returns registry ids that apply to method/path (all ids still required at finalize with skip_reason if N/A).
func ApplicableVulnTypeIDsForEndpoint(method, pathPattern string) []string {
	method = strings.ToUpper(strings.TrimSpace(method))
	path := strings.ToLower(strings.TrimSpace(pathPattern))
	isGET := method == "" || method == "GET" || method == "HEAD"
	isLogin := strings.Contains(path, "login") || strings.Contains(path, "signin") || strings.Contains(path, "dologin")
	isRoot := path == "" || path == "/"

	var out []string
	for _, d := range vulnTypeRegistry {
		if vulnTypeAppliesToEndpoint(d, isGET, isLogin, isRoot) {
			out = append(out, d.ID)
		}
	}
	return out
}

func vulnTypeAppliesToEndpoint(d VulnTypeDef, isGET, isLogin, isRoot bool) bool {
	switch strings.TrimSpace(d.Scope) {
	case "all":
		return true
	case "no_get":
		return !isGET
	case "login_only":
		return isLogin
	case "root_only":
		return isRoot
	default:
		return true
	}
}
