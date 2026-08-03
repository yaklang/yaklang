package ssadb

import (
	"context"
	"strings"

	"github.com/yaklang/gorm"
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
	query = query.Model(&IrCode{}).Where(TableIrCodes+".program_name = ?", progName)
	if err := query.Pluck(TableIrCodes+".code_id", &ids).Error; err != nil {
		log.Errorf("failed to get ids: %v", err)
		return emptyIrCodeChan()
	}
	return yieldIrCodes(ctx, progName, ids)
}

func yieldFromIrIndex(DB *gorm.DB, ctx context.Context, progName string) <-chan *IrCode {
	var ids []int64
	if err := DB.Model(&IrIndex{}).Where(TableIrIndices+".program_name = ?", progName).Pluck("DISTINCT "+TableIrIndices+".value_id", &ids).Error; err != nil {
		log.Errorf("failed to get ids from index: %v", err)
		return emptyIrCodeChan()
	}
	return yieldIrCodes(ctx, progName, ids)
}

// yieldFromIrIndexWithExcludeFiles queries from IrIndex, excluding specified files.
// Uses fast index pluck + in-memory file filtering instead of a multi-table JOIN.
func yieldFromIrIndexWithExcludeFiles(DB *gorm.DB, ctx context.Context, progName string, excludeFiles []string) <-chan *IrCode {
	var matchedIds []int64
	distinctIrIndicesValueID := "DISTINCT " + TableIrIndices + ".value_id"
	if err := DB.Pluck(distinctIrIndicesValueID, &matchedIds).Error; err != nil {
		log.Errorf("failed to get matched ids: %v", err)
		return emptyIrCodeChan()
	}
	if len(matchedIds) == 0 {
		return emptyIrCodeChan()
	}
	return yieldIrCodesWithExclude(ctx, progName, matchedIds, excludeFiles)
}

func buildExcludeFileSet(excludeFiles []string) map[string]struct{} {
	if len(excludeFiles) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(excludeFiles)*2)
	for _, filePath := range excludeFiles {
		n := normalizeFilePathForExclusion(filePath)
		set[n] = struct{}{}
		set[strings.TrimPrefix(n, "/")] = struct{}{}
	}
	return set
}

func irCodeFilePath(ir *IrCode) string {
	if ir == nil || ir.IsEmptySourceCodeHash() {
		return ""
	}
	editor, err := GetEditorByHash(ir.SourceCodeHash)
	if err != nil || editor == nil {
		return ""
	}
	path := editor.GetFilePath()
	if path == "" {
		path = editor.GetUrl()
	}
	return normalizeFilePathForExclusion(path)
}

func irCodeExcludedBySet(ir *IrCode, excludeSet map[string]struct{}) bool {
	if len(excludeSet) == 0 {
		return false
	}
	path := irCodeFilePath(ir)
	if path == "" {
		return false
	}
	if _, ok := excludeSet[path]; ok {
		return true
	}
	return false
}

func yieldIrCodesWithExclude(ctx context.Context, progName string, ids []int64, excludeFiles []string) <-chan *IrCode {
	excludeSet := buildExcludeFileSet(excludeFiles)
	in := yieldIrCodes(ctx, progName, ids)
	if len(excludeSet) == 0 {
		return in
	}
	outC := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
	go func() {
		defer outC.Close()
		for ir := range in {
			if irCodeExcludedBySet(ir, excludeSet) {
				continue
			}
			outC.SafeFeed(ir)
		}
	}()
	return outC.OutputChannel()
}

// yieldFromIrIndexWithIncludeFiles queries from IrIndex, keeping only specified files.
func yieldFromIrIndexWithIncludeFiles(DB *gorm.DB, ctx context.Context, progName string, includeFiles []string) <-chan *IrCode {
	var matchedIds []int64
	distinctIrIndicesValueID := "DISTINCT " + TableIrIndices + ".value_id"
	if err := DB.Pluck(distinctIrIndicesValueID, &matchedIds).Error; err != nil {
		log.Errorf("failed to get matched ids: %v", err)
		return emptyIrCodeChan()
	}
	if len(matchedIds) == 0 {
		return emptyIrCodeChan()
	}
	return yieldIrCodesWithInclude(ctx, progName, matchedIds, includeFiles)
}

func irCodeIncludedBySet(ir *IrCode, includeSet map[string]struct{}) bool {
	if len(includeSet) == 0 {
		return false
	}
	path := irCodeFilePath(ir)
	if path == "" {
		return false
	}
	if _, ok := includeSet[path]; ok {
		return true
	}
	if _, ok := includeSet[strings.TrimPrefix(path, "/")]; ok {
		return true
	}
	return false
}

func yieldIrCodesWithInclude(ctx context.Context, progName string, ids []int64, includeFiles []string) <-chan *IrCode {
	includeSet := buildExcludeFileSet(includeFiles) // same path normalization
	in := yieldIrCodes(ctx, progName, ids)
	if len(includeSet) == 0 {
		// Empty include set means no files match — return empty channel.
		outC := chanx.NewUnlimitedChan[*IrCode](ctx, 1)
		outC.Close()
		return outC.OutputChannel()
	}
	outC := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
	go func() {
		defer outC.Close()
		for ir := range in {
			if !irCodeIncludedBySet(ir, includeSet) {
				continue
			}
			outC.SafeFeed(ir)
		}
	}()
	return outC.OutputChannel()
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
	return searchVariableWithFileFilter(db, ctx, progName, cache, compareMode, matchMod, value, excludeFiles, nil)
}

// SearchVariableWithIncludeFiles searches variables, keeping only results from includeFiles.
func SearchVariableWithIncludeFiles(db *gorm.DB, ctx context.Context, progName string, cache *NameCache, compareMode CompareMode, matchMod MatchMode, value string, includeFiles []string) <-chan *IrCode {
	var result <-chan *IrCode
	_ = diagnostics.TrackLow("ssadb.SearchVariableWithIncludeFiles", func() error {
		result = searchVariableWithFileFilter(db, ctx, progName, cache, compareMode, matchMod, value, nil, includeFiles)
		return nil
	})
	return result
}

func searchVariableWithFileFilter(db *gorm.DB, ctx context.Context, progName string, cache *NameCache, compareMode CompareMode, matchMod MatchMode, value string, excludeFiles, includeFiles []string) <-chan *IrCode {
	// 1. Handle Glob -> Regexp
	if compareMode == GlobCompare {
		value = glob.Glob2Regex(value)
		compareMode = RegexpCompare
	}

	filterConst := func(ch <-chan *IrCode) <-chan *IrCode {
		excludeSet := buildExcludeFileSet(excludeFiles)
		includeSet := buildExcludeFileSet(includeFiles)
		if len(excludeSet) == 0 && len(includeSet) == 0 {
			resultCh := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
			go func() {
				defer resultCh.Close()
				for ir := range ch {
					resultCh.SafeFeed(ir)
				}
			}()
			return resultCh.OutputChannel()
		}
		resultCh := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
		go func() {
			defer resultCh.Close()
			for ir := range ch {
				if len(includeSet) > 0 && !irCodeIncludedBySet(ir, includeSet) {
					continue
				}
				if irCodeExcludedBySet(ir, excludeSet) {
					continue
				}
				resultCh.SafeFeed(ir)
			}
		}()
		return resultCh.OutputChannel()
	}

	// 2. Handle ConstType
	if matchMod&ConstType != 0 {
		// Use GetDB() with fully-qualified program_name — GetDBInProgram's bare
		// "program_name = ?" becomes ambiguous once IrSources is joined for exclude.
		query := GetDB().Model(&IrCode{}).
			Where(TableIrCodes+".program_name = ?", progName).
			Where(TableIrCodes+".opcode = ? AND "+TableIrCodes+".const_type = ?", 5, "normal")
		if compareMode == ExactCompare {
			query = query.Where(TableIrCodes+".string = ?", value)
		} else {
			// This regex operation on the 'string' column (TEXT) is likely a full table scan if no index exists.
			// Keep dialect compatibility:
			// - SQLite: "REGEXP" via the registered regexp() function in sqlite3_extended driver.
			// - MySQL:  "REGEXP"
			// - Postgres: "~"
			dialect := GetDB().Dialect().GetName()
			switch dialect {
			case "postgres", "postgresql":
				query = query.Where(TableIrCodes+".string ~ ?", value)
			default:
				query = query.Where(TableIrCodes+".string REGEXP ?", value)
			}
		}
		// ConstType query also needs file exclusion — filter in memory after load.
		ch := YieldIrCode(query, ctx, progName)
		return filterConst(ch)
	}

	// 3. Handle Variable/Field (Search in IrIndex)
	query := db.Model(&IrIndex{})
	// PASS progName to applyMatchCondition
	query = applyMatchCondition(query, progName, cache, matchMod, compareMode, value)

	if len(includeFiles) > 0 {
		return yieldFromIrIndexWithIncludeFiles(query, ctx, progName, includeFiles)
	}
	if len(excludeFiles) > 0 {
		return yieldFromIrIndexWithExcludeFiles(query, ctx, progName, excludeFiles)
	}
	ch := yieldFromIrIndex(query, ctx, progName)
	resultCh := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
	go func() {
		defer resultCh.Close()
		for ir := range ch {
			resultCh.SafeFeed(ir)
		}
	}()
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
