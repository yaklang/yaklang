package loop_intent

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// BuildCapabilityCatalog collects all available tools, forges, skills, and focus modes,
// formats each as a single-line entry for AI consumption.
// Format: [type:identifier]: verbose_name - description. keywords: kw1,kw2
func BuildCapabilityCatalog(r aicommon.AIInvokeRuntime) string {
	var sb strings.Builder

	db := consts.GetGormProfileDatabase()
	if db != nil {
		reactloops.GenerateYakToolsCatalog(&sb)

		forges, err := yakit.GetAllAIForge(db)
		if err != nil {
			log.Warnf("capability catalog: failed to load forges: %v", err)
		} else {
			for _, f := range forges {
				if f == nil || !schema.IsRunnableForgeType(f.ForgeType) {
					continue
				}
				name := f.ForgeVerboseName
				if name == "" {
					name = f.ForgeName
				}
				desc := utils.ShrinkString(f.Description, 120)
				line := fmt.Sprintf("[forge:%s]: %s - %s", f.ForgeName, name, desc)
				if f.ToolKeywords != "" {
					line += fmt.Sprintf(". keywords: %s", utils.ShrinkString(f.ToolKeywords, 80))
				}
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}

		// Include enabled MCP tools that have a cached description so the LLM
		// catalog-match pass can discover them via semantic matching.
		// Use [mcp-tool:] prefix to distinguish MCP tools from built-in tools.
		if reactloops.IsMCPServersAllowed(r) {
			mcpToolConfigs, mcpErr := yakit.GetAllEnabledMCPServerToolConfigs(db)
			if mcpErr != nil {
				log.Warnf("capability catalog: failed to load MCP tool configs: %v", mcpErr)
			} else {
				for _, t := range mcpToolConfigs {
					if t.Description == "" {
						continue
					}
					fullName := fmt.Sprintf("mcp_%s_%s", t.ServerName, t.ToolName)
					desc := utils.ShrinkString(t.Description, 120)
					sb.WriteString(fmt.Sprintf("[mcp-tool:%s]: %s - %s\n", fullName, fullName, desc))
				}
			}
		}
	}

	type skillLoaderProvider interface {
		GetSkillLoader() aiskillloader.SkillLoader
	}
	cfg := r.GetConfig()
	if provider, ok := cfg.(skillLoaderProvider); ok {
		skillLoader := provider.GetSkillLoader()
		if skillLoader != nil && skillLoader.HasSkills() {
			for _, meta := range skillLoader.AllSkillMetas() {
				desc := utils.ShrinkString(meta.Description, 120)
				sb.WriteString(fmt.Sprintf("[skill:%s]: %s - %s\n", meta.Name, meta.Name, desc))
			}
		}
	}

	for _, meta := range reactloops.GetAllLoopMetadata() {
		if meta.IsHidden {
			continue
		}
		desc := utils.ShrinkString(meta.Description, 120)
		sb.WriteString(fmt.Sprintf("[focus_mode:%s]: %s - %s\n", meta.Name, meta.Name, desc))
	}

	return sb.String()
}

// MatchExplicitIdentifiersFromCatalog recognizes capability identifiers that
// the user wrote verbatim. Semantic matching belongs to the bounded BM25
// query_capabilities action; sending the entire catalog through one or more
// LiteForge calls made a short request create 20-40KB lightweight spikes.
func MatchExplicitIdentifiersFromCatalog(catalog string, userQuery string) []string {
	if catalog == "" || userQuery == "" {
		return nil
	}
	query := strings.ToLower(userQuery)
	seen := make(map[string]struct{})
	var matched []string
	for _, line := range strings.Split(catalog, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		end := strings.IndexByte(line, ']')
		colon := strings.IndexByte(line, ':')
		if end <= colon+1 || colon < 1 {
			continue
		}
		identifier := strings.TrimSpace(line[colon+1 : end])
		if identifier == "" || !containsExplicitIdentifier(query, strings.ToLower(identifier)) {
			continue
		}
		if _, ok := seen[identifier]; ok {
			continue
		}
		seen[identifier] = struct{}{}
		matched = append(matched, identifier)
	}
	return matched
}

func containsExplicitIdentifier(query, identifier string) bool {
	for from := 0; from < len(query); {
		idx := strings.Index(query[from:], identifier)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(identifier)
		leftOK := start == 0 || !isIdentifierByte(query[start-1])
		rightOK := end == len(query) || !isIdentifierByte(query[end])
		if leftOK && rightOK {
			return true
		}
		from = start + 1
	}
	return false
}

func isIdentifierByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

// VerifyIdentifiers filters a list of identifier names through ResolveIdentifier,
// removing any that don't correspond to real tools/forges/skills/focus modes.
func VerifyIdentifiers(loop *reactloops.ReActLoop, identifiers []string) []string {
	var verified []string
	for _, id := range identifiers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		resolved := loop.ResolveIdentifier(id)
		if resolved.IsUnknown() {
			log.Infof("capability catalog: identifier %q not resolved, skipping", id)
			continue
		}
		verified = append(verified, id)
	}
	return verified
}
