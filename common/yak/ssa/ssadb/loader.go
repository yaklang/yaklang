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

// fileFilterMode selects how path sets are applied after index pluck.
type fileFilterMode int

const (
	fileFilterNone fileFilterMode = iota
	fileFilterExclude
	fileFilterInclude
)

// yieldFromIrIndexWithFileFilter plucks matched value_ids then filters by file path in memory.
func yieldFromIrIndexWithFileFilter(DB *gorm.DB, ctx context.Context, progName string, files []string, mode fileFilterMode) <-chan *IrCode {
	var matchedIds []int64
	distinctIrIndicesValueID := "DISTINCT " + TableIrIndices + ".value_id"
	if err := DB.Pluck(distinctIrIndicesValueID, &matchedIds).Error; err != nil {
		log.Errorf("failed to get matched ids: %v", err)
		return emptyIrCodeChan()
	}
	if len(matchedIds) == 0 {
		return emptyIrCodeChan()
	}
	return yieldIrCodesWithFileFilter(ctx, progName, matchedIds, files, mode)
}

func yieldFromIrIndexWithExcludeFiles(DB *gorm.DB, ctx context.Context, progName string, excludeFiles []string) <-chan *IrCode {
	return yieldFromIrIndexWithFileFilter(DB, ctx, progName, excludeFiles, fileFilterExclude)
}

func yieldFromIrIndexWithIncludeFiles(DB *gorm.DB, ctx context.Context, progName string, includeFiles []string) <-chan *IrCode {
	return yieldFromIrIndexWithFileFilter(DB, ctx, progName, includeFiles, fileFilterInclude)
}

func buildFilePathSet(files []string) map[string]struct{} {
	if len(files) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(files)*2)
	for _, filePath := range files {
		n := normalizeFilePath(filePath)
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
	return normalizeFilePath(path)
}

// irCodePassesFileFilter keeps empty-path IR (extern/lib) under both include and exclude.
func irCodePassesFileFilter(ir *IrCode, pathSet map[string]struct{}, mode fileFilterMode) bool {
	if mode == fileFilterNone || len(pathSet) == 0 {
		return mode != fileFilterInclude // empty include set → no match; empty exclude → keep all
	}
	path := irCodeFilePath(ir)
	if path == "" {
		return true
	}
	_, inSet := pathSet[path]
	if !inSet {
		_, inSet = pathSet[strings.TrimPrefix(path, "/")]
	}
	switch mode {
	case fileFilterInclude:
		return inSet
	case fileFilterExclude:
		return !inSet
	default:
		return true
	}
}

func yieldIrCodesWithFileFilter(ctx context.Context, progName string, ids []int64, files []string, mode fileFilterMode) <-chan *IrCode {
	pathSet := buildFilePathSet(files)
	if mode == fileFilterInclude && len(pathSet) == 0 {
		return emptyIrCodeChan()
	}
	in := yieldIrCodes(ctx, progName, ids)
	if mode == fileFilterNone || (mode == fileFilterExclude && len(pathSet) == 0) {
		return in
	}
	outC := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
	go func() {
		defer outC.Close()
		for ir := range in {
			if !irCodePassesFileFilter(ir, pathSet, mode) {
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
		result = searchVariableWithFileFilter(db, ctx, progName, cache, compareMode, matchMod, value, excludeFiles, nil)
		return nil
	})
	return result
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
	if compareMode == GlobCompare {
		value = glob.Glob2Regex(value)
		compareMode = RegexpCompare
	}

	filterLoaded := func(ch <-chan *IrCode) <-chan *IrCode {
		mode := fileFilterNone
		files := excludeFiles
		if len(includeFiles) > 0 {
			mode = fileFilterInclude
			files = includeFiles
		} else if len(excludeFiles) > 0 {
			mode = fileFilterExclude
		}
		if mode == fileFilterNone {
			return ch
		}
		pathSet := buildFilePathSet(files)
		if mode == fileFilterInclude && len(pathSet) == 0 {
			return emptyIrCodeChan()
		}
		outC := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
		go func() {
			defer outC.Close()
			for ir := range ch {
				if !irCodePassesFileFilter(ir, pathSet, mode) {
					continue
				}
				outC.SafeFeed(ir)
			}
		}()
		return outC.OutputChannel()
	}

	if matchMod&ConstType != 0 {
		query := GetDB().Model(&IrCode{}).
			Where(TableIrCodes+".program_name = ?", progName).
			Where(TableIrCodes+".opcode = ? AND "+TableIrCodes+".const_type = ?", 5, "normal")
		if compareMode == ExactCompare {
			query = query.Where(TableIrCodes+".string = ?", value)
		} else {
			dialect := GetDB().Dialect().GetName()
			switch dialect {
			case "postgres", "postgresql":
				query = query.Where(TableIrCodes+".string ~ ?", value)
			default:
				query = query.Where(TableIrCodes+".string REGEXP ?", value)
			}
		}
		return filterLoaded(YieldIrCode(query, ctx, progName))
	}

	query := db.Model(&IrIndex{})
	query = applyMatchCondition(query, progName, cache, matchMod, compareMode, value)

	if len(includeFiles) > 0 {
		return yieldFromIrIndexWithIncludeFiles(query, ctx, progName, includeFiles)
	}
	if len(excludeFiles) > 0 {
		return yieldFromIrIndexWithExcludeFiles(query, ctx, progName, excludeFiles)
	}
	return yieldFromIrIndex(query, ctx, progName)
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

// normalizeFilePath ensures a leading "/".
func normalizeFilePath(filePath string) string {
	if filePath == "" {
		return ""
	}
	if !strings.HasPrefix(filePath, "/") {
		return "/" + filePath
	}
	return filePath
}

// normalizeFilePathForExclusion is kept for callers; delegates to normalizeFilePath.
func normalizeFilePathForExclusion(filePath string) string {
	return normalizeFilePath(filePath)
}

func emptyIrCodeChan() <-chan *IrCode {
	ch := make(chan *IrCode)
	close(ch)
	return ch
}
