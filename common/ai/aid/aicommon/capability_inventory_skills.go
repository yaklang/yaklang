package aicommon

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
)

func skillMetaToInventoryItem(meta *aiskillloader.SkillMeta, loadState string) CapabilityInventoryNamedItem {
	item := CapabilityInventoryNamedItem{Category: "skill", SkillLoadState: loadState}
	if meta == nil {
		return item
	}
	name := strings.TrimSpace(meta.Name)
	item.Name = name
	item.VerboseName = name
	item.Description = meta.Description
	return item
}

// BuildInventorySkillsFromLoader builds capability_inventory skill entries from
// prompt-registry listing (token budget) plus fully loaded skills. Loaded skills
// are always included even if they fall beyond the registry token budget.
func BuildInventorySkillsFromLoader(loader aiskillloader.SkillLoader, loadedNames map[string]struct{}) []CapabilityInventoryNamedItem {
	return BuildInventorySkillsFromLoaderWithEstimator(loader, loadedNames, nil)
}

// BuildInventorySkillsFromLoaderWithEstimator uses the same registry token budget as SKILLS_CONTEXT.
func BuildInventorySkillsFromLoaderWithEstimator(
	loader aiskillloader.SkillLoader,
	loadedNames map[string]struct{},
	tokenEstimator func(string) int,
) []CapabilityInventoryNamedItem {
	if loader == nil || !loader.HasSkills() {
		return nil
	}
	if loadedNames == nil {
		loadedNames = map[string]struct{}{}
	}

	registryListed, _ := aiskillloader.SelectSkillMetasForPromptRegistry(loader.AllSkillMetas(), tokenEstimator)

	byName := make(map[string]CapabilityInventoryNamedItem)
	for _, meta := range registryListed {
		if meta == nil || strings.TrimSpace(meta.Name) == "" {
			continue
		}
		state := CapabilityInventorySkillLoadMetadata
		if _, loaded := loadedNames[meta.Name]; loaded {
			state = CapabilityInventorySkillLoadLoaded
		}
		byName[meta.Name] = skillMetaToInventoryItem(meta, state)
	}

	result := make([]CapabilityInventoryNamedItem, 0, len(byName)+len(loadedNames))
	for name := range loadedNames {
		if _, ok := byName[name]; ok {
			continue
		}
		meta, err := lookupSkillMeta(loader, name)
		if err != nil || meta == nil {
			continue
		}
		byName[name] = skillMetaToInventoryItem(meta, CapabilityInventorySkillLoadLoaded)
	}

	for _, item := range byName {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func lookupSkillMeta(loader aiskillloader.SkillLoader, name string) (*aiskillloader.SkillMeta, error) {
	if lookup, ok := loader.(aiskillloader.SkillMetaLookup); ok {
		return lookup.GetSkillMeta(name)
	}
	for _, meta := range loader.AllSkillMetas() {
		if meta != nil && meta.Name == name {
			return meta, nil
		}
	}
	return nil, nil
}

// BuildInventorySkillsFromManager mirrors prompt SKILLS_CONTEXT: registry metas
// as metadata plus fully loaded skills as loaded.
func BuildInventorySkillsFromManager(mgr *aiskillloader.SkillsContextManager) []CapabilityInventoryNamedItem {
	if mgr == nil {
		return nil
	}
	loadedNames := make(map[string]struct{})
	for _, meta := range mgr.GetCurrentSelectedSkills() {
		if meta == nil || strings.TrimSpace(meta.Name) == "" {
			continue
		}
		loadedNames[meta.Name] = struct{}{}
	}
	return BuildInventorySkillsFromLoaderWithEstimator(mgr.GetLoader(), loadedNames, mgr.TokenEstimator())
}
