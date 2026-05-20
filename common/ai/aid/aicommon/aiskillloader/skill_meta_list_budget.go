package aiskillloader

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/ytoken"
)

// AvailableSkillsRegistryHeader is the registry listing header in SKILLS_CONTEXT.
const AvailableSkillsRegistryHeader = "== Available Skills (use loading_skills action to load) ==\n"

// AvailableSkillsOverflowHint formats the tail line when registry listing hits the token budget.
func AvailableSkillsOverflowHint(omitted int) string {
	return fmt.Sprintf("  ... and %d more skills. Use search_capabilities to find specific skills.\n", omitted)
}

// FormatAvailableSkillRegistryLine formats one registry skill line for prompt / budget accounting.
func FormatAvailableSkillRegistryLine(meta *SkillMeta) string {
	if meta == nil {
		return ""
	}
	return fmt.Sprintf("  - %s: %s\n", meta.Name, meta.Description)
}

// MeasureStringTokens returns token count for rendered text.
func MeasureStringTokens(rendered string, tokenEstimator func(string) int) int {
	if tokenEstimator != nil {
		return tokenEstimator(rendered)
	}
	return ytoken.CalcTokenCount(rendered)
}

// SelectSkillMetasByTokenBudget returns the prefix of registry skill metas that fit within
// maxTokens when rendered as Available Skills lines (optional sectionHeader included in budget).
// omittedCount is the number of remaining skills after listed (same semantics as appendAvailableSkillsSection).
func SelectSkillMetasByTokenBudget(
	metas []*SkillMeta,
	maxTokens int,
	tokenEstimator func(string) int,
	sectionHeader string,
) (listed []*SkillMeta, omittedCount int) {
	if maxTokens <= 0 {
		maxTokens = MetadataListMaxTokens
	}

	sorted := sortSkillMetasByName(metas)
	if len(sorted) == 0 {
		return nil, 0
	}

	currentTokens := MeasureStringTokens(sectionHeader, tokenEstimator)
	listed = make([]*SkillMeta, 0, len(sorted))

	for _, meta := range sorted {
		if meta == nil || strings.TrimSpace(meta.Name) == "" {
			continue
		}
		line := FormatAvailableSkillRegistryLine(meta)
		lineTokens := MeasureStringTokens(line, tokenEstimator)
		if currentTokens+lineTokens > maxTokens {
			omittedCount = len(sorted) - len(listed)
			break
		}
		currentTokens += lineTokens
		listed = append(listed, meta)
	}
	return listed, omittedCount
}

// SelectSkillMetasForPromptRegistry applies the same budget as SKILLS_CONTEXT "Available Skills".
func SelectSkillMetasForPromptRegistry(metas []*SkillMeta, tokenEstimator func(string) int) ([]*SkillMeta, int) {
	return SelectSkillMetasByTokenBudget(metas, MetadataListMaxTokens, tokenEstimator, AvailableSkillsRegistryHeader)
}
