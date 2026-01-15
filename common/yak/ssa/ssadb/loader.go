package ssadb

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/glob"
)

func YieldIrCode(DB *gorm.DB, ctx context.Context, progName string) <-chan *IrCode {
	var ids []int64
	if err := DB.Model(&IrCode{}).Where("program_name = ?", progName).Pluck("code_id", &ids).Error; err != nil {
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

// yieldFromIrIndexWithExcludeFiles 从 IrIndex 查询，排除指定的文件
// DB: 已经应用了匹配条件的查询（如 variable_name = ?）
// excludeFiles: 要排除的文件路径列表（规范化后的路径，如 "/test.go"）
// SQL 查询逻辑：
// SELECT DISTINCT ir_indices.value_id
// FROM ir_indices
// INNER JOIN ir_codes ON ir_indices.value_id = ir_codes.code_id
// INNER JOIN ir_sources ON ir_codes.source_code_hash = ir_sources.source_code_hash
// WHERE ir_indices.program_name = ?
//
//	AND (已应用的匹配条件，如 variable_name = ?)
//	AND CONCAT(ir_sources.folder_path, ir_sources.file_name) NOT IN (排除的文件列表)
func yieldFromIrIndexWithExcludeFiles(DB *gorm.DB, ctx context.Context, progName string, excludeFiles []string) chan *IrCode {
	var ids []int64

	// 构建查询：通过 JOIN 排除指定文件
	// 注意：DB 已经包含了匹配条件（如 variable_name = ?），我们需要在此基础上添加 JOIN 和排除条件
	// 为了避免列名歧义，我们需要确保所有 program_name 条件都明确指定了表名
	// 由于 DB 可能已经包含了 WHERE program_name = ?（没有表名前缀），我们需要重新构建查询
	// 方案：先获取匹配的 value_id（使用 DB 的查询），然后再 JOIN 排除文件
	// 这样可以避免在 JOIN 后产生 program_name 列名歧义

	// 第一步：从 DB 中获取匹配的 value_id（DB 已经包含了匹配条件和 program_name 条件）
	var matchedIds []int64
	if err := DB.Select("DISTINCT ir_indices.value_id").Pluck("DISTINCT ir_indices.value_id", &matchedIds).Error; err != nil {
		log.Errorf("failed to get matched ids: %v", err)
		return emptyIrCodeChan()
	}

	if len(matchedIds) == 0 {
		return emptyIrCodeChan()
	}

	// 第二步：基于匹配的 value_id，JOIN 排除文件
	// 使用传入的 DB 来获取数据库连接，确保方言检测一致
	// 注意：我们需要从 DB 中获取底层连接，但由于 GORM 的限制，我们使用 GetDB() 但确保使用相同的方言检测
	baseDB := GetDB()
	query := baseDB.Table("ir_indices").
		Select("DISTINCT ir_indices.value_id").
		Joins("INNER JOIN ir_codes ON ir_indices.value_id = ir_codes.code_id").
		Joins("INNER JOIN ir_sources ON ir_codes.source_code_hash = ir_sources.source_code_hash").
		Where("ir_indices.program_name = ?", progName).
		Where("ir_codes.program_name = ?", progName).
		Where("ir_sources.program_name = ?", progName).
		Where("ir_indices.value_id IN (?)", matchedIds)

	// 如果有要排除的文件，添加排除条件
	if len(excludeFiles) > 0 {
		// 使用 baseDB 来检测数据库方言，确保与实际查询使用的数据库一致
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

	if err := query.Pluck("DISTINCT ir_indices.value_id", &ids).Error; err != nil {
		log.Errorf("failed to get ids from index with exclude files: %v", err)
		return emptyIrCodeChan()
	}
	return yieldIrCodes(ctx, progName, ids)
}

func yieldIrCodes(ctx context.Context, progName string, ids []int64) chan *IrCode {
	outC := make(chan *IrCode)
	go func() {
		defer outC.Close()
		idsToLoad := make([]int64, 0, len(ids))
		cache := GetIrCodeCache(progName)
		// 先从缓存加载
		for _, id := range ids {
			if ir, ok := cache.Get(id); ok {
				outC.SafeFeed(ir)
			} else {
				idsToLoad = append(idsToLoad, id)
			}
		}
		if len(idsToLoad) == 0 {
			return
		}

		// 批量加载缺失的数据
		db := GetDB().Model(&IrCode{}).Where("program_name = ?", progName)
		ch := bizhelper.FastPagination[*IrCode](ctx, db, nil,
			bizhelper.WithFastPaginator_IDs(idsToLoad), bizhelper.WithFastPaginator_IndexField("code_id"),
		)
		for ir := range ch {
			cache.Set(ir.CodeID, ir)
			outC.SafeFeed(ir)
		}
	}()

	return outC.OutputChannel()
}

// type MatchMode int
const (
	NameMatch int = 1
	KeyMatch      = 1 << 1
	BothMatch     = NameMatch | KeyMatch
	ConstType     = 1 << 2
)

const (
	ExactCompare int = iota
	GlobCompare
	RegexpCompare
	OpcodeCompare
)

func SearchVariable(db *gorm.DB, ctx context.Context, progName string, compareMode, matchMod int, value string) chan *IrCode {
	return SearchVariableWithExcludeFiles(db, ctx, progName, compareMode, matchMod, value, nil)
}

// SearchVariableWithExcludeFiles 搜索变量，支持排除指定文件
// excludeFiles: 要排除的文件路径列表（规范化后的路径，如 "/test.go"）
// SQL 查询逻辑：
// SELECT DISTINCT ir_indices.value_id
// FROM ir_indices
// INNER JOIN ir_codes ON ir_indices.value_id = ir_codes.code_id
// INNER JOIN ir_sources ON ir_codes.source_code_hash = ir_sources.source_code_hash
// WHERE ir_indices.program_name = ?
//
//	AND (ir_indices.variable_name = ? OR ir_indices.class_name = ?)
//	AND CONCAT(ir_sources.folder_path, ir_sources.file_name) NOT IN (排除的文件列表)
func SearchVariableWithExcludeFiles(db *gorm.DB, ctx context.Context, progName string, compareMode, matchMod int, value string, excludeFiles []string) chan *IrCode {
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
			query = query.Where("string REGEXP ?", value)
		}
		// ConstType 查询也需要排除文件
		if len(excludeFiles) > 0 {
			query = query.Joins("INNER JOIN ir_sources ON ir_codes.source_code_hash = ir_sources.source_code_hash").
				Where("ir_sources.program_name = ?", progName)
			// 使用传入的 db 来检测数据库方言
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
		return YieldIrCode(query, ctx, progName)
	}

	// 3. Handle Variable/Field (Search in IrIndex)
	query := db.Model(&IrIndex{})
	query = applyMatchCondition(query, matchMod, compareMode, value)

	// 如果有要排除的文件，使用带排除功能的查询
	if len(excludeFiles) > 0 {
		return yieldFromIrIndexWithExcludeFiles(query, ctx, progName, excludeFiles)
	}
	return yieldFromIrIndex(query, ctx, progName)
}

func applyMatchCondition(db *gorm.DB, mod int, compareMode int, value string) *gorm.DB {
	matchName := mod&NameMatch != 0
	matchField := mod&KeyMatch != 0
	if !matchName && !matchField {
		matchName = true
	}

	switch compareMode {
	case RegexpCompare:
		switch {
		case matchName && matchField:
			return db.Where("variable_name REGEXP ? OR class_name REGEXP ? OR field_name REGEXP ?", value, value, value)
		case matchName:
			return db.Where("variable_name REGEXP ? OR class_name REGEXP ?", value, value)
		case matchField:
			return db.Where("field_name REGEXP ?", value)
		default:
			return db
		}
	default: // ExactCompare and others
		switch {
		case matchName && matchField:
			return db.Where("variable_name = ? OR class_name = ? OR field_name = ?", value, value, value)
		case matchName:
			return db.Where("variable_name = ? OR class_name = ?", value, value)
		case matchField:
			return db.Where("field_name = ?", value)
		default:
			return db
		}
	}
}

func SearchIrCodeByOpcodes(db *gorm.DB, ctx context.Context, progName string, opcodes ...int) <-chan *IrCode {
	db = db.Model(&IrCode{}).Where("opcode in (?)", opcodes)
	return YieldIrCode(db, ctx, progName)
}

// normalizeFilePathForExclusion 规范化文件路径用于排除查询
// 确保路径以 / 开头
func normalizeFilePathForExclusion(filePath string) string {
	if !strings.HasPrefix(filePath, "/") {
		return "/" + filePath
	}
	return filePath
}

// getConcatExpression 根据数据库方言返回字符串拼接表达式
// SQLite 使用 ||，MySQL/PostgreSQL 使用 CONCAT
func getConcatExpression(db *gorm.DB) string {
	if db == nil {
		// 如果 db 为 nil，使用默认的 GetDB()
		db = GetDB()
	}
	dialect := db.Dialect().GetName()
	switch dialect {
	case "sqlite3", "sqlite":
		// SQLite 使用 || 操作符拼接字符串
		return "(ir_sources.folder_path || ir_sources.file_name)"
	default:
		// MySQL, PostgreSQL 等使用 CONCAT 函数
		return "CONCAT(ir_sources.folder_path, ir_sources.file_name)"
	}
}

func emptyIrCodeChan() chan *IrCode {
	ch := make(chan *IrCode)
	close(ch)
	return ch
}
