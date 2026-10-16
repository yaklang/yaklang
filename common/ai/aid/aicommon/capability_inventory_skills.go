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

// BuildInventorySkillsFromManager lists registry metadata and the manager's
// visible Timeline skills, including their current body token counts.
func BuildInventorySkillsFromManager(mgr *aiskillloader.SkillsContextManager) []CapabilityInventoryNamedItem {
	if mgr == nil {
		return nil
	}
	items := BuildInventorySkillsFromLoaderWithEstimator(mgr.GetLoader(), nil, mgr.TokenEstimator())
	index := make(map[string]int, len(items))
	for i, item := range items {
		index[item.Name] = i
	}
	for _, skill := range mgr.GetCurrentSelectedSkills() {
		if skill == nil || skill.SkillMeta == nil || strings.TrimSpace(skill.Name) == "" {
			continue
		}
		item := skillMetaToInventoryItem(skill.SkillMeta, CapabilityInventorySkillLoadLoaded)
		item.TokenCount = skill.TokenCount
		if i, found := index[item.Name]; found {
			items[i] = item
		} else {
			index[item.Name] = len(items)
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}
