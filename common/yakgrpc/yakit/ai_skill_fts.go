package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

// AISkillVTableName returns the FTS5 virtual table name for AISkill.
func AISkillVTableName() string {
	return (&schema.AISkill{}).TableName() + "_fts"
}

// AISkillSearchFilter is a lightweight filter for skill search.
type AISkillSearchFilter struct {
	SkillNames []string
	Keywords   string
}

// aiSkillsTable is the GORM table name for AISkill.
// Used to qualify column names when JOINing with the FTS5 table.
const aiSkillsTable = "ai_skills"

// FilterAISkillForSearch builds a GORM query from AISkillSearchFilter.
// Column names are qualified with the base table name to avoid ambiguity
// when JOINed with the FTS5 virtual table.
func FilterAISkillForSearch(db *gorm.DB, filter *AISkillSearchFilter) *gorm.DB {
	db = db.Model(&schema.AISkill{})
	if filter == nil {
		return db
	}

	db = bizhelper.ExactQueryStringArrayOr(db, aiSkillsTable+".name", filter.SkillNames)
	db = bizhelper.FuzzSearchEx(db, []string{
		aiSkillsTable + ".name",
		aiSkillsTable + ".description",
		aiSkillsTable + ".keywords",
		aiSkillsTable + ".body",
	}, filter.Keywords, false)
	return db
}

// defaultAISkillFTS5 defines the FTS5 trigram index configuration for AISkill.
// Uses external content mode referencing the ai_skills table.
// Indexes: name, description, keywords, body
var defaultAISkillFTS5 = &bizhelper.SQLiteFTS5Config{
	BaseModel:    &schema.AISkill{},
	FTSTable:     AISkillVTableName(),
	Columns:      []string{"name", "description", "keywords", "body"},
	ContentTable: aiSkillsTable,
	Tokenize:     "trigram",
}

// EnsureAISkillFTS5 creates or updates the FTS5 trigram index for AISkill.
// Safe to call multiple times; idempotent.
func EnsureAISkillFTS5(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !schema.IsSQLite(db) {
		return nil
	}
	if err := bizhelper.SQLiteFTS5Setup(db, defaultAISkillFTS5); err != nil {
		// Some sqlite builds might not include FTS5 (e.g. custom builds).
		// Treat it as non-fatal.
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil
		}
		return err
	}
	return nil
}

// SearchAISkillBM25 uses SQLite FTS5 BM25 ranking to search AISkill.
//   - For short keywords (<3 chars): fall back to LIKE-based search via FuzzSearchEx
//   - For longer keywords: use FTS5 BM25 trigram matching for ranked results
//   - If FTS5 table is not available: fall back to LIKE-based search
func SearchAISkillBM25(db *gorm.DB, filter *AISkillSearchFilter, limit, offset int) ([]*schema.AISkill, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}

	var match string
	if filter != nil {
		match = strings.TrimSpace(filter.Keywords)
	}
	if match == "" {
		return []*schema.AISkill{}, nil
	}

	var res = make([]*schema.AISkill, 0)
	// Short keywords or non-SQLite or no FTS table: fall back to LIKE search
	if len(match) < 3 || !schema.IsSQLite(db) || !db.HasTable(defaultAISkillFTS5.FTSTable) {
		if err := FilterAISkillForSearch(db, filter).Limit(limit).Offset(offset).Find(&res).Error; err != nil {
			return nil, err
		}
		return res, nil
	}

	// BM25 path: clear Keywords to avoid double-filtering, then use FTS5
	if filter != nil {
		filter.Keywords = ""
	}

	return bizhelper.SQLiteFTS5BM25Match[*schema.AISkill](FilterAISkillForSearch(db, filter), defaultAISkillFTS5, match, limit, offset)
}

// CreateOrUpdateAISkill creates or updates an AISkill by name.
// If a skill with the same name already exists, it is updated.
func CreateOrUpdateAISkill(db *gorm.DB, skill *schema.AISkill) error {
	if db == nil {
		return utils.Error("db is nil")
	}
	if skill.Name == "" {
		return utils.Error("skill name is required")
	}

	var existing schema.AISkill
	if err := db.Where("name = ?", skill.Name).First(&existing).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return db.Create(skill).Error
		}
		return err
	}

	// Update existing record
	return db.Model(&existing).Updates(map[string]interface{}{
		"description":              skill.Description,
		"license":                  skill.License,
		"keywords":                 skill.Keywords,
		"body":                     skill.Body,
		"hash":                     skill.Hash,
		"source_path":              skill.SourcePath,
		"disable_model_invocation": skill.DisableModelInvocation,
	}).Error
}

// GetAISkillByName retrieves an AISkill by name.
func GetAISkillByName(db *gorm.DB, name string) (*schema.AISkill, error) {
	if db == nil {
		return nil, utils.Error("db is nil")
	}
	var skill schema.AISkill
	if err := db.Where("name = ?", name).First(&skill).Error; err != nil {
		return nil, err
	}
	return &skill, nil
}

// GetAISkillByHash retrieves an AISkill by hash.
func GetAISkillByHash(db *gorm.DB, hash string) (*schema.AISkill, error) {
	if db == nil {
		return nil, utils.Error("db is nil")
	}
	var skill schema.AISkill
	if err := db.Where("hash = ?", hash).First(&skill).Error; err != nil {
		return nil, err
	}
	return &skill, nil
}

// DeleteAISkillByName deletes an AISkill by name.
func DeleteAISkillByName(db *gorm.DB, name string) error {
	if db == nil {
		return utils.Error("db is nil")
	}
	return db.Where("name = ?", name).Delete(&schema.AISkill{}).Error
}
