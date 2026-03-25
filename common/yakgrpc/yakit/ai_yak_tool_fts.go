package yakit

import (
	"regexp"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

var aiToolSearchASCIITermRegexp = regexp.MustCompile(`[A-Za-z0-9_]{3,}`)
var aiToolSearchHanTermRegexp = regexp.MustCompile(`[\p{Han}]{2,}`)

func AIYakToolVTableName() string {
	return (&schema.AIYakTool{}).TableName() + "_fts"
}

type AIYakToolFilter struct {
	ToolNames     []string
	ToolPaths     []string
	OnlyFavorites bool
	Keywords      []string
}

func FilterAIYakTools(db *gorm.DB, filter *AIYakToolFilter) *gorm.DB {
	db = db.Model(&schema.AIYakTool{})
	if filter == nil {
		return db
	}

	db = bizhelper.ExactQueryStringArrayOr(db, "name", filter.ToolNames)
	db = bizhelper.ExactQueryStringArrayOr(db, "path", filter.ToolPaths)
	if filter.OnlyFavorites {
		db = db.Where("is_favorite = ?", true)
	}
	var keywords []string
	for _, kw := range filter.Keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		keywords = append(keywords, kw)
	}
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"name", "keywords", "description", "path"}, keywords, false)
	return db
}

var defaultAIYakToolFTS5 = &bizhelper.SQLiteFTS5Config{
	BaseModel: &schema.AIYakTool{},
	FTSTable:  AIYakToolVTableName(),
	Columns:   []string{"name", "verbose_name", "description", "keywords", "path"},
	// Use external content mode to keep the FTS index consistent and avoid FTS5 maintenance commands
	// that are not supported by all SQLite builds for contentful FTS tables.
	ContentTable: "ai_yak_tools",
	Tokenize:     "trigram",
}

//func init() {
//	// Ensure AIYakTool has a proper FTS index in SQLite profile DB to accelerate searching.
//	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_PROFILE_DATABASE, func(db *gorm.DB) {
//		if db == nil {
//			return
//		}
//		if !schema.IsSQLite(db) {
//			return
//		}
//		baseTable := (&schema.AIYakTool{}).TableName()
//		if !db.HasTable(baseTable) {
//			// Base table is gone, but the FTS virtual table may remain; clean it up.
//			if err := bizhelper.SQLiteFTS5Drop(db, defaultAIYakToolFTS5); err != nil {
//				log.Warnf("failed to drop orphan ai_yak_tools fts5 index: %v", err)
//			}
//			return
//		}
//		if err := EnsureAIYakToolFTS5(db); err != nil {
//			log.Warnf("failed to setup ai_yak_tools fts5 index: %v", err)
//		}
//	})
//}

func EnsureAIYakToolFTS5(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !schema.IsSQLite(db) {
		return nil
	}
	if err := bizhelper.SQLiteFTS5Setup(db, defaultAIYakToolFTS5); err != nil {
		// Some sqlite builds might not include FTS5 (e.g. custom builds).
		// Treat it as non-fatal and let the caller decide how to handle.
		if strings.Contains(err.Error(), "no such module: fts5") {
			return nil
		}
		return err
	}
	return nil
}

// SearchAIYakToolBM25 uses SQLite FTS5 BM25 ranking to search AIYakTool.
// It follows the same pattern as SearchVectorStoreDocumentBM25:
// - Extract match from filter.Keywords
// - For short keywords (<3), fall back to LIKE-based search
// - For longer keywords, clear filter.Keywords and apply FTS (to avoid double filtering)
func SearchAIYakToolBM25(db *gorm.DB, filter *AIYakToolFilter, limit, offset int) ([]*schema.AIYakTool, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}

	var rawMatches []string
	if filter != nil {
		for _, m := range filter.Keywords {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			rawMatches = append(rawMatches, m)
		}
	}
	if len(rawMatches) == 0 {
		return []*schema.AIYakTool{}, nil
	}

	matches := expandAIYakToolSearchTerms(rawMatches)
	if len(matches) == 0 {
		matches = rawMatches
	}

	var res = make([]*schema.AIYakTool, 0)
	maxLen := 0
	for _, m := range matches {
		if len(m) > maxLen {
			maxLen = len(m)
		}
	}
	if maxLen < 3 || !schema.IsSQLite(db) || !db.HasTable(defaultAIYakToolFTS5.FTSTable) {
		if err := FilterAIYakTools(db, cloneAIYakToolFilterWithKeywords(filter, matches)).Limit(limit).Offset(offset).Find(&res).Error; err != nil {
			return nil, err
		}
		return res, nil
	}

	ftsFilter := filter
	if filter != nil {
		ftsFilter = cloneAIYakToolFilterWithKeywords(filter, nil)
	}

	res, err := bizhelper.SQLiteFTS5BM25Match[*schema.AIYakTool](FilterAIYakTools(db, ftsFilter), defaultAIYakToolFTS5, matches, limit, offset)
	if err == nil {
		return res, nil
	}

	res = make([]*schema.AIYakTool, 0)
	if fallbackErr := FilterAIYakTools(db, cloneAIYakToolFilterWithKeywords(filter, matches)).Limit(limit).Offset(offset).Find(&res).Error; fallbackErr != nil {
		return nil, err
	}
	return res, nil
}

func cloneAIYakToolFilterWithKeywords(filter *AIYakToolFilter, keywords []string) *AIYakToolFilter {
	if filter == nil {
		if keywords == nil {
			return nil
		}
		return &AIYakToolFilter{Keywords: append([]string(nil), keywords...)}
	}
	cloned := *filter
	if keywords == nil {
		cloned.Keywords = nil
	} else {
		cloned.Keywords = append([]string(nil), keywords...)
	}
	return &cloned
}

func expandAIYakToolSearchTerms(matches []string) []string {
	seen := make(map[string]struct{})
	results := make([]string, 0, len(matches))
	appendTerm := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		if _, ok := seen[term]; ok {
			return
		}
		seen[term] = struct{}{}
		results = append(results, term)
	}

	for _, match := range matches {
		trimmed := strings.TrimSpace(match)
		if trimmed == "" {
			continue
		}

		if isSafeFTS5Term(trimmed) {
			appendTerm(trimmed)
		}

		for _, token := range aiToolSearchASCIITermRegexp.FindAllString(trimmed, -1) {
			appendTerm(strings.ToLower(token))
		}

		for _, seq := range aiToolSearchHanTermRegexp.FindAllString(trimmed, -1) {
			runes := []rune(seq)
			if len(runes) <= 3 {
				appendTerm(seq)
				continue
			}

			for size := 3; size <= 6 && size <= len(runes); size++ {
				for start := 0; start+size <= len(runes); start++ {
					appendTerm(string(runes[start : start+size]))
				}
			}
		}
	}

	return results
}

func isSafeFTS5Term(term string) bool {
	return !strings.ContainsAny(term, `"'/:()[]{}\\`)
}
