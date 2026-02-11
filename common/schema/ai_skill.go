package schema

import (
	"github.com/jinzhu/gorm"
)

// AISkill represents a registered AI skill in the profile database.
// Skills are discovered from SKILL.md files and indexed for BM25 search.
type AISkill struct {
	gorm.Model

	// Name is the unique identifier for this skill (from SKILL.md frontmatter).
	Name string `json:"name" gorm:"unique_index"`

	// Description describes what the skill does (from SKILL.md frontmatter).
	Description string `json:"description" gorm:"type:text;index"`

	// License is the license name (from SKILL.md frontmatter).
	License string `json:"license,omitempty"`

	// Keywords is a comma-separated list of keywords for search (extracted from metadata).
	Keywords string `json:"keywords" gorm:"type:text;index"`

	// Body is the markdown content after the SKILL.md frontmatter.
	Body string `json:"body" gorm:"type:text"`

	// Hash is the content hash for change detection.
	// Computed by: collecting all files <=10KB in the skill directory (sorted by path),
	// SHA256 each file, concatenate all hex digests, then SHA256 the concatenation.
	Hash string `json:"hash" gorm:"index"`

	// SourcePath records where the skill was loaded from (for debugging).
	SourcePath string `json:"source_path,omitempty"`

	// DisableModelInvocation when true means the skill is only included when explicitly invoked.
	DisableModelInvocation bool `json:"disable_model_invocation" gorm:"default:false"`
}

func (a *AISkill) TableName() string {
	return "ai_skills"
}
