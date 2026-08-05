package aireact

import (
	"fmt"
	"path"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
)

// RecommendedSkill describes a product-recommended built-in skill exposed to
// clients. Name is the value clients pass through AIStartParams.EnabledCapabilities.
type RecommendedSkill struct {
	Name            string
	DisplayNameZhCN string
	Description     string
}

// recommendedBuiltinSkills is intentionally a fixed product list rather than
// an enumeration of every built-in or user-installed skill. Additions here are
// therefore explicit UI recommendations.
var recommendedBuiltinSkills = []struct {
	name string
}{
	{name: "pentest-task-design"},
	{name: "code-review"},
}

// GetRecommendedBuiltinSkills resolves the fixed recommendations against the
// embedded SKILL.md files. Reading metadata from the embedded source keeps the
// gRPC description aligned with the exact skill ReAct will load.
func GetRecommendedBuiltinSkills() ([]RecommendedSkill, error) {
	builtinFS := GetBuiltinSkillsFS()
	if builtinFS == nil {
		return nil, fmt.Errorf("built-in skills filesystem is not initialized")
	}

	result := make([]RecommendedSkill, 0, len(recommendedBuiltinSkills))
	for _, definition := range recommendedBuiltinSkills {
		skillPath := path.Join("skills", definition.name, "SKILL.md")
		content, err := builtinFS.ReadFile(skillPath)
		if err != nil {
			return nil, fmt.Errorf("read recommended built-in skill %q: %w", definition.name, err)
		}
		meta, err := aiskillloader.ParseSkillMeta(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse recommended built-in skill %q: %w", definition.name, err)
		}
		if meta.Name != definition.name {
			return nil, fmt.Errorf(
				"recommended built-in skill name mismatch: registry=%q, metadata=%q",
				definition.name,
				meta.Name,
			)
		}
		displayNameZhCN := meta.GetDisplayName(aiskillloader.SkillLocaleZhCN)
		if displayNameZhCN == "" {
			return nil, fmt.Errorf(
				"recommended built-in skill %q is missing metadata.%s",
				definition.name,
				aiskillloader.SkillMetadataDisplayNameZhCN,
			)
		}
		result = append(result, RecommendedSkill{
			Name:            meta.Name,
			DisplayNameZhCN: displayNameZhCN,
			Description:     meta.Description,
		})
	}

	return result, nil
}
