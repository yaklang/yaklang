package ssadb

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/utils/glob"
)

func YieldIrCode(DB *gorm.DB, ctx context.Context, progName string) <-chan *IrCode {
	var ids []int64
	query := DB
	if query == nil {
		query = GetDB()
	}
	query = query.Model(&IrCode{}).Where("program_name = ?", progName)
	if err := query.Pluck("code_id", &ids).Error; err != nil {
		log.Errorf("failed to get ids: %v", err)
		return emptyIrCodeChan()
	}
	return yieldIrCodes(ctx, progName, ids)
}

func yieldFromIrIndex(DB *gorm.DB, ctx context.Context, progName string) <-chan *IrCode {
	var ids []int64
	if err := DB.Model(&IrIndex{}).Where("program_name = ?", progName).Pluck("DISTINCT value_id", &ids).Error; err != nil {
		log.Errorf("failed to get ids from index: %v", err)
		return emptyIrCodeChan()
	}
	return yieldIrCodes(ctx, progName, ids)
}

// yieldFromIrIndexWithExcludeFiles queries from IrIndex, excluding specified files
// DB: query with applied matching conditions (e.g. variable_id = ?)
// excludeFiles: list of file paths to exclude (normalized paths, e.g. "/test.go")
func yieldFromIrIndexWithExcludeFiles(DB *gorm.DB, ctx context.Context, progName string, excludeFiles []string) <-chan *IrCode {
	var ids []int64

	// Step 1: Get matched value_ids from DB (DB already contains match conditions and program_name)
	var matchedIds []int64
	distinctIrIndicesValueID := "DISTINCT " + TableIrIndices + ".value_id"
	if err := DB.Pluck(distinctIrIndicesValueID, &matchedIds).Error; err != nil {
		log.Errorf("failed to get matched ids: %v", err)
		return emptyIrCodeChan()
	}

	if len(matchedIds) == 0 {
		return emptyIrCodeChan()
	}

	// Step 2: Join to exclude files based on matched value_ids
	baseDB := GetDB()
	query := baseDB.Model(&IrIndex{}).
		Select(distinctIrIndicesValueID).
		Joins("INNER JOIN "+TableIrCodes+" ON "+TableIrIndices+".value_id = "+TableIrCodes+".code_id").
		Joins("INNER JOIN "+TableIrSources+" ON "+TableIrCodes+".source_code_hash = "+TableIrSources+".source_code_hash").
		Where(TableIrIndices+".program_name = ?", progName).
		Where(TableIrCodes+".program_name = ?", progName).
		Where(TableIrSources+".program_name = ?", progName).
		Where(TableIrIndices+".value_id IN (?)", matchedIds)

	// Add exclusion conditions if needed
	if len(excludeFiles) > 0 {
		concatExpr := getConcatExpression(baseDB)
		excludeConditions := make([]string, 0, len(excludeFiles))
		excludeArgs := make([]interface{}, 0, len(excludeFiles))
		for _, filePath := range excludeFiles {
			normalizedPath := normalizeFilePathForExclusion(filePath)
			excludeConditions = append(excludeConditions, concatExpr+" != ?")
			excludeArgs = append(excludeArgs, normalizedPath)
		}
		if len(excludeConditions) > 0 {
			query = query.Where(strings.Join(excludeConditions, " AND "), excludeArgs...)
		}
	}

	if err := query.Pluck(distinctIrIndicesValueID, &ids).Error; err != nil {
		log.Errorf("failed to get ids from index with exclude files: %v", err)
		return emptyIrCodeChan()
	}
	return yieldIrCodes(ctx, progName, ids)
}

func yieldIrCodes(ctx context.Context, progName string, ids []int64) <-chan *IrCode {
	outC := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
	go func() {
		defer outC.Close()
		_ = diagnostics.TrackLow("ssadb.yieldIrCodes", func() error {
			idsToLoad := make([]int64, 0, len(ids))
			cache := GetIrCodeCache(progName)
			// Load from cache first
			for _, id := range ids {
				if ir, ok := cache.Get(id); ok {
					outC.SafeFeed(ir)
				} else {
					idsToLoad = append(idsToLoad, id)
				}
			}
			if len(idsToLoad) == 0 {
				return nil
			}

			// Batch load missing data
			db := GetDB().Model(&IrCode{}).Where("program_name = ?", progName)
			ch := bizhelper.FastPagination[*IrCode](ctx, db, nil,
				bizhelper.WithFastPaginator_IDs(idsToLoad), bizhelper.WithFastPaginator_IndexField("code_id"),
			)
			for ir := range ch {
				cache.Set(ir.CodeID, ir)
				outC.SafeFeed(ir)
			}
			return nil
		})
	}()

	return outC.OutputChannel()
}

func SearchVariable(db *gorm.DB, ctx context.Context, progName string, cache *NameCache, compareMode CompareMode, matchMod MatchMode, value string) <-chan *IrCode {
	return SearchVariableWithExcludeFiles(db, ctx, progName, cache, compareMode, matchMod, value, nil)
}

// SearchVariableWithExcludeFiles searches variables, supports excluding specified files
func SearchVariableWithExcludeFiles(db *gorm.DB, ctx context.Context, progName string, cache *NameCache, compareMode CompareMode, matchMod MatchMode, value string, excludeFiles []string) <-chan *IrCode {
	var result <-chan *IrCode
	_ = diagnostics.TrackLow("ssadb.SearchVariableWithExcludeFiles", func() error {
		result = searchVariableWithExcludeFiles(db, ctx, progName, cache, compareMode, matchMod, value, excludeFiles)
		return nil
	})
	return result
}

func searchVariableWithExcludeFiles(db *gorm.DB, ctx context.Context, progName string, cache *NameCache, compareMode CompareMode, matchMod MatchMode, value string, excludeFiles []string) <-chan *IrCode {
	// 1. Handle Glob -> Regexp
	if compareMode == GlobCompare {
		value = glob.Glob2Regex(value)
		compareMode = RegexpCompare
	}

	// 2. Handle ConstType
	if matchMod&ConstType != 0 {
		query := db.Model(&IrCode{}).Where("opcode=5 AND const_type = 'normal'")
		if compareMode == ExactCompare {
			query = query.Where("string = ?", value)
		} else {
			// This regex operation on the 'string' column (TEXT) is likely a full table scan if no index exists.
			// Keep dialect compatibility:
			// - SQLite: "REGEXP" via the registered regexp() function in sqlite3_extended driver.
			// - MySQL:  "REGEXP"
			// - Postgres: "~"
			dialect := db.Dialect().GetName()
			switch dialect {
			case "postgres", "postgresql":
				query = query.Where("string ~ ?", value)
			default:
				query = query.Where("string REGEXP ?", value)
			}
		}
		// ConstType query also needs file exclusion
		if len(excludeFiles) > 0 {
			query = query.Joins("INNER JOIN "+TableIrSources+" ON "+TableIrCodes+".source_code_hash = "+TableIrSources+".source_code_hash").
				Where(TableIrSources+".program_name = ?", progName)
			concatExpr := getConcatExpression(db)
			excludeConditions := make([]string, 0, len(excludeFiles))
			excludeArgs := make([]interface{}, 0, len(excludeFiles))
			for _, filePath := range excludeFiles {
				normalizedPath := normalizeFilePathForExclusion(filePath)
				excludeConditions = append(excludeConditions, concatExpr+" != ?")
				excludeArgs = append(excludeArgs, normalizedPath)
			}
			if len(excludeConditions) > 0 {
				query = query.Where(strings.Join(excludeConditions, " AND "), excludeArgs...)
			}
		}
		ch := YieldIrCode(query, ctx, progName)
		resultCh := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
		go func() {
			defer resultCh.Close()
			for ir := range ch {
				resultCh.SafeFeed(ir)
			}
		}()
		return resultCh.OutputChannel()
	}

	// 3. Handle Variable/Field (Search in IrIndex)
	query := db.Model(&IrIndex{})
	// PASS progName to applyMatchCondition
	query = applyMatchCondition(query, progName, cache, matchMod, compareMode, value)

	var resultCh *chanx.UnlimitedChan[*IrCode]
	if len(excludeFiles) > 0 {
		ch := yieldFromIrIndexWithExcludeFiles(query, ctx, progName, excludeFiles)
		resultCh = chanx.NewUnlimitedChan[*IrCode](ctx, 100)
		go func() {
			defer resultCh.Close()
			for ir := range ch {
				resultCh.SafeFeed(ir)
			}
		}()
	} else {
		ch := yieldFromIrIndex(query, ctx, progName)
		resultCh = chanx.NewUnlimitedChan[*IrCode](ctx, 100)
		go func() {
			defer resultCh.Close()
			for ir := range ch {
				resultCh.SafeFeed(ir)
			}
		}()
	}
	return resultCh.OutputChannel()
}

func applyMatchCondition(db *gorm.DB, progName string, cache *NameCache, mod MatchMode, compareMode CompareMode, value string) *gorm.DB {
	matchName := mod&NameMatch != 0
	matchField := mod&KeyMatch != 0
	if !matchName && !matchField {
		matchName = true
	}

	ids := cache.GetIDsByPattern(value, compareMode)
	if len(ids) == 0 {
		return db.Where("1 = 0")
	}

	fields := []string{}
	if matchName {
		fields = append(fields, "variable_id", "class_id")
	}
	if matchField {
		fields = append(fields, "field_id")
	}

	if len(fields) > 0 {
		uids := make([]uint64, len(ids))
		for i, id := range ids {
			uids[i] = uint64(id)
		}
		return bizhelper.ExactQueryMultipleUInt64ArrayOr(db, fields, uids)
	}
	return db
}

func SearchIrCodeByOpcodes(db *gorm.DB, ctx context.Context, progName string, opcodes ...int) <-chan *IrCode {
	db = db.Model(&IrCode{}).Where("opcode in (?)", opcodes)
	return YieldIrCode(db, ctx, progName)
}

// normalizeFilePathForExclusion normalizes file path for exclusion query
// Ensures path starts with /
func normalizeFilePathForExclusion(filePath string) string {
	if !strings.HasPrefix(filePath, "/") {
		return "/" + filePath
	}
	return filePath
}

// getConcatExpression returns string concatenation expression based on DB dialect
// SQLite uses ||, MySQL/PostgreSQL uses CONCAT
func getConcatExpression(db *gorm.DB) string {
	if db == nil {
		db = GetDB()
	}
	dialect := db.Dialect().GetName()
	switch dialect {
	case "sqlite3", "sqlite":
		// SQLite uses || operator
		return "(" + TableIrSources + ".folder_path || " + TableIrSources + ".file_name)"
	default:
		// MySQL, PostgreSQL use CONCAT function
		return "CONCAT(" + TableIrSources + ".folder_path, " + TableIrSources + ".file_name)"
	}
}

func emptyIrCodeChan() <-chan *IrCode {
	ch := make(chan *IrCode)
	close(ch)
	return ch
}
