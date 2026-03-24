package aiskillloader

import (
	"strings"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// SearchSkillMetasByStructure performs the original loader-side structural search
// against skill name and description.
func SearchSkillMetasByStructure(query string, metas []*SkillMeta, limit int) []*SkillMeta {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if len(metas) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = len(metas)
	}

	queryLower := strings.ToLower(query)
	queryTokens := strings.Fields(queryLower)

	var matched []*SkillMeta
	for _, meta := range metas {
		if meta == nil || strings.TrimSpace(meta.Name) == "" {
			continue
		}
		searchText := strings.ToLower(meta.Name + " " + meta.Description)

		if strings.Contains(searchText, queryLower) {
			matched = append(matched, meta)
			if len(matched) >= limit {
				break
			}
			continue
		}

		if len(queryTokens) <= 1 {
			continue
		}

		meaningfulTokens := 0
		matchCount := 0
		for _, token := range queryTokens {
			if len(token) < 2 {
				continue
			}
			meaningfulTokens++
			if strings.Contains(searchText, token) {
				matchCount++
			}
		}
		if meaningfulTokens > 0 && matchCount > 0 && matchCount >= (meaningfulTokens+1)/2 {
			matched = append(matched, meta)
			if len(matched) >= limit {
				break
			}
		}
	}
	return matched
}

// SearchSkillMetasBM25 performs BM25 skill search against the current in-memory
// skill metadata set by materializing it into a temporary AIForge(skillmd) index.
func SearchSkillMetasBM25(query string, metas []*SkillMeta, limit int) ([]*SkillMeta, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(metas) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	memDB, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, utils.Wrap(err, "failed to create in-memory SQLite for skill BM25 search")
	}
	defer memDB.Close()

	if err := memDB.AutoMigrate(&schema.AIForge{}).Error; err != nil {
		return nil, utils.Wrap(err, "auto-migrate temporary ai_forges failed")
	}
	if err := yakit.EnsureAIForgeFTS5(memDB); err != nil {
		return nil, utils.Wrap(err, "ensure temporary ai_forges FTS5 failed")
	}

	metaMap := make(map[string]*SkillMeta, len(metas))
	for _, meta := range metas {
		if meta == nil || strings.TrimSpace(meta.Name) == "" {
			continue
		}
		metaMap[meta.Name] = meta
		if err := memDB.Create(skillMetaToSearchForge(meta)).Error; err != nil {
			return nil, utils.Wrapf(err, "insert temporary search skill %q failed", meta.Name)
		}
	}

	results, err := yakit.SearchAIForgeBM25(memDB, &yakit.AIForgeSearchFilter{
		ForgeTypes: []string{schema.FORGE_TYPE_SkillMD},
		Keywords:   strings.Fields(query),
	}, limit, 0)
	if err != nil {
		return nil, utils.Wrap(err, "skill BM25 search failed")
	}

	matched := make([]*SkillMeta, 0, len(results))
	for _, result := range results {
		if meta, ok := metaMap[result.ForgeName]; ok {
			matched = append(matched, meta)
		}
	}
	return matched, nil
}

func skillMetaToSearchForge(meta *SkillMeta) *schema.AIForge {
	return &schema.AIForge{
		ForgeName:    meta.Name,
		Description:  buildForgeDescriptionFromSkillMeta(meta),
		Tags:         metadataToForgeTags(meta.Metadata),
		InitPrompt:   meta.Body,
		ToolKeywords: buildKeywordsString(meta),
		ForgeType:    schema.FORGE_TYPE_SkillMD,
	}
}

func forgeToSkillMetaPreview(forge *schema.AIForge) *SkillMeta {
	if forge == nil {
		return nil
	}
	return &SkillMeta{
		Name:        forge.ForgeName,
		Description: forge.Description,
		Metadata:    forgeTagsToMetadata(forge.Tags),
		Body:        forge.InitPrompt,
	}
}

// LookupSkillMeta resolves metadata by name, using lazy loader capabilities when available.
func LookupSkillMeta(loader SkillLoader, name string) (*SkillMeta, error) {
	if loader == nil {
		return nil, utils.Error("skill loader is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, utils.Error("skill name is empty")
	}
	if provider, ok := loader.(SkillMetaLookup); ok {
		return provider.GetSkillMeta(name)
	}
	for _, meta := range loader.AllSkillMetas() {
		if meta != nil && meta.Name == name {
			return meta, nil
		}
	}
	return nil, utils.Errorf("skill %q not found", name)
}

// SearchSkillMetas delegates to a loader-native search when available and falls back
// to structural search over enumerated metadata.
func SearchSkillMetas(loader SkillLoader, query string, limit int) ([]*SkillMeta, error) {
	if loader == nil {
		return nil, utils.Error("skill loader is nil")
	}
	if provider, ok := loader.(SkillMetaSearcher); ok {
		return provider.SearchSkillMetas(query, limit)
	}
	return SearchSkillMetasByStructure(query, loader.AllSkillMetas(), limit), nil
}

// GetSkillSourceStats returns best-effort source statistics for a loader.
func GetSkillSourceStats(loader SkillLoader) SkillSourceStats {
	if loader == nil {
		return SkillSourceStats{}
	}
	if provider, ok := loader.(SkillStatsProvider); ok {
		return provider.GetSkillSourceStats()
	}
	return SkillSourceStats{LocalCount: len(loader.AllSkillMetas())}
}
