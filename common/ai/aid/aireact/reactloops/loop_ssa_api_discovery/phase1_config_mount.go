package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

var (
	reAddPathPrefix     = regexp.MustCompile(`(?i)addPathPrefix\s*\(\s*"([^"]+)"`)
	reAddPathPatterns   = regexp.MustCompile(`(?i)addPathPatterns\s*\(\s*"([^"]+)"`)
	rePathPrefixConst   = regexp.MustCompile(`(?i)(?:CONTEXT|ADMIN|API|WEB)[_A-Z]*(?:PATH|PREFIX|URI)\s*=\s*"([^"]+)"`)
	reRegistryMapping   = regexp.MustCompile(`(?i)registry\.add(?:ViewController|ResourceHandler)(?:Mappings)?\s*\(\s*"([^"]+)"`)
	reSpringMappingPath = regexp.MustCompile(`@(?:Request|Get|Post|Put|Delete)Mapping\s*\(\s*(?:(?:value|path)\s*=\s*)?"([^"]+)"`)
)

const (
	worklistCategoryRoutingConfig = "routing_config"
	worklistCategoryAuthEntry     = "auth_entry"
	worklistCategoryAPIHandler    = "api_handler"
	worklistCategoryBuildConfig   = "build_config"
)

// extractMountPrefixesFromJava scans WebMvcConfigurer / routing config sources for mount prefixes.
func extractMountPrefixesFromJava(content, ref string) []RoutingFact {
	var facts []RoutingFact
	seen := map[string]struct{}{}
	add := func(prefix, kind, hint string) {
		prefix = normURLPath(prefix)
		if prefix == "" || prefix == "/" {
			return
		}
		if _, ok := seen[prefix]; ok {
			return
		}
		seen[prefix] = struct{}{}
		facts = append(facts, RoutingFact{
			Kind:        kind,
			MountPrefix: prefix,
			Ref:         ref,
			Hint:        hint,
			Confidence:  0.9,
		})
	}
	for _, m := range reAddPathPrefix.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			add(m[1], "webmvc_add_path_prefix", "addPathPrefix")
		}
	}
	for _, m := range reAddPathPatterns.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			p := strings.TrimSuffix(strings.TrimSpace(m[1]), "/**")
			p = strings.TrimSuffix(p, "/*")
			add(p, "webmvc_path_pattern", "addPathPatterns")
		}
	}
	for _, m := range rePathPrefixConst.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			add(m[1], "path_constant", "context/admin/api path constant")
		}
	}
	for _, m := range reRegistryMapping.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			add(m[1], "view_controller_mapping", "registry mapping")
		}
	}
	for _, m := range reSpringMappingPath.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 && strings.Count(m[1], "/") <= 2 {
			add(m[1], "class_request_mapping", "@RequestMapping path")
		}
	}
	return facts
}

func readMountFactsForRelPath(rt *Runtime, relPath string) []RoutingFact {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil
	}
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" {
		return nil
	}
	abs := filepath.Join(rt.Session.CodeRootPath, filepath.FromSlash(relPath))
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	return extractMountPrefixesFromJava(string(data), relPath)
}

func batchHasRoutingConfig(batch []WorklistSeedItem) bool {
	for _, item := range batch {
		if isRoutingConfigCategory(item.Category, item.Reason) {
			return true
		}
	}
	return false
}

func isRoutingConfigCategory(category, reason string) bool {
	category = strings.TrimSpace(category)
	if category == worklistCategoryRoutingConfig || category == worklistCategoryBuildConfig {
		return true
	}
	lower := strings.ToLower(reason + " " + category)
	for _, tok := range []string{"webmvcconfigurer", "security configuration", "routing/security config", "web.xml", "application"} {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

func stageHasMountPrefixFacts(out *CodeReadingStageOutput) bool {
	if out == nil {
		return false
	}
	for _, rf := range out.RoutingFacts {
		if mp := strings.TrimSpace(rf.MountPrefix); mp != "" && mp != "/" {
			return true
		}
	}
	return false
}

// bootstrapConfigMountFromBatch programmatically reads routing config files and returns stage output with mount prefixes.
func bootstrapConfigMountFromBatch(rt *Runtime, stage int, batch []WorklistSeedItem) *CodeReadingStageOutput {
	out := &CodeReadingStageOutput{
		Stage:              stage,
		ReadFilesCompleted: []string{},
		APIFragments:       []APIFragment{},
		RoutingFacts:       []RoutingFact{},
		NextWorklist:       []WorklistSeedItem{},
	}
	if !batchHasRoutingConfig(batch) {
		return nil
	}
	seenPrefix := map[string]struct{}{}
	for _, item := range batch {
		if !isRoutingConfigCategory(item.Category, item.Reason) {
			continue
		}
		rel := normalizePlanFileRef(rt, item.RelPath)
		if rel != "" {
			out.ReadFilesCompleted = append(out.ReadFilesCompleted, rel)
		}
		for _, rf := range readMountFactsForRelPath(rt, rel) {
			if _, ok := seenPrefix[rf.MountPrefix]; ok {
				continue
			}
			seenPrefix[rf.MountPrefix] = struct{}{}
			out.RoutingFacts = append(out.RoutingFacts, rf)
		}
	}
	if len(out.RoutingFacts) == 0 {
		return out
	}
	return out
}

func validateConfigStageMountRequired(stage int, out *CodeReadingStageOutput, batch []WorklistSeedItem) error {
	if !batchHasRoutingConfig(batch) {
		return nil
	}
	if stageHasMountPrefixFacts(out) {
		return nil
	}
	return utils.Errorf("stage %d: routing_config batch must produce routing_facts with mount_prefix before controller reading", stage)
}

func isAuthEntryPath(rel string) bool {
	lower := strings.ToLower(filepath.ToSlash(rel))
	for _, tok := range []string{"/login", "/auth", "/signin", "/sign-in", "/session", "login", "authenticate"} {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

func isBackendCodeRelPath(rel string) bool {
	cat := classifyFileCategory(rel, nil)
	if cat == fileCategoryResource {
		return false
	}
	if shouldSkipPathForCodeHarvest(rel) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(rel))
	if ext != ".java" && ext != ".go" && ext != ".py" && ext != ".php" && ext != ".kt" {
		if cat != fileCategoryConfig && cat != fileCategoryCode {
			return false
		}
	}
	return true
}

func profileHasMountEvidence(p *ProjectProfileV1) bool {
	if p != nil {
		if cp := strings.TrimSpace(p.ContextPath); cp != "" && cp != "unknown" && cp != "/" {
			return true
		}
	}
	return false
}

func batchMaxPriority(batch []WorklistSeedItem) int {
	maxP := 0
	for _, item := range batch {
		p := item.Priority
		if p == 0 {
			p = 99
		}
		if p > maxP {
			maxP = p
		}
	}
	return maxP
}

func filterWorklistByMaxPriority(items []WorklistSeedItem, maxPriority int) []WorklistSeedItem {
	var out []WorklistSeedItem
	for _, item := range items {
		p := item.Priority
		if p == 0 {
			p = 99
		}
		if p <= maxPriority {
			out = append(out, item)
		}
	}
	return out
}

func mergeStageOutputWithProgrammatic(out *CodeReadingStageOutput, prog *CodeReadingStageOutput) *CodeReadingStageOutput {
	if out == nil {
		return prog
	}
	if prog == nil {
		return out
	}
	seenFile := map[string]struct{}{}
	for _, f := range out.ReadFilesCompleted {
		seenFile[f] = struct{}{}
	}
	for _, f := range prog.ReadFilesCompleted {
		if _, ok := seenFile[f]; !ok {
			out.ReadFilesCompleted = append(out.ReadFilesCompleted, f)
			seenFile[f] = struct{}{}
		}
	}
	seenPrefix := map[string]struct{}{}
	for _, rf := range out.RoutingFacts {
		seenPrefix[rf.MountPrefix] = struct{}{}
	}
	for _, rf := range prog.RoutingFacts {
		if _, ok := seenPrefix[rf.MountPrefix]; ok {
			continue
		}
		seenPrefix[rf.MountPrefix] = struct{}{}
		out.RoutingFacts = append(out.RoutingFacts, rf)
	}
	if strings.TrimSpace(out.AuthNotes) == "" {
		out.AuthNotes = prog.AuthNotes
	}
	return out
}
