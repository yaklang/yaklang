package ssadb

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/glob"
)

func YieldIrCode(DB *gorm.DB, ctx context.Context, progName string) chan *IrCode {
	var ids []int64
	if err := DB.Model(&IrCode{}).Where("program_name = ?", progName).Pluck("code_id", &ids).Error; err != nil {
		log.Errorf("failed to get ids: %v", err)
		return emptyIrCodeChan()
	}
	return yieldIrCodes(ctx, progName, ids)
}

func yieldFromIrIndex(DB *gorm.DB, ctx context.Context, progName string) chan *IrCode {
	var ids []int64
	if err := DB.Model(&IrIndex{}).Where("program_name = ?", progName).Pluck("DISTINCT value_id", &ids).Error; err != nil {
		log.Errorf("failed to get ids from index: %v", err)
		return emptyIrCodeChan()
	}
	return yieldIrCodes(ctx, progName, ids)
}

func yieldIrCodes(ctx context.Context, progName string, ids []int64) chan *IrCode {
	outC := make(chan *IrCode)
	go func() {
		defer close(outC)
		idsToLoad := make([]int64, 0, len(ids))
		cache := GetIrCodeCache(progName)
		// 先从缓存加载
		for _, id := range ids {
			if ir, ok := cache.Get(id); ok {
				outC <- ir
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
			outC <- ir
		}
	}()

	return outC
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
		return YieldIrCode(query, ctx, progName)
	}

	// 3. Handle Variable/Field (Search in IrIndex)
	query := db.Model(&IrIndex{})
	query = applyMatchCondition(query, matchMod, compareMode, value)
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

func SearchIrCodeByOpcodes(db *gorm.DB, ctx context.Context, progName string, opcodes ...int) chan *IrCode {
	db = db.Model(&IrCode{}).Where("opcode in (?)", opcodes)
	return YieldIrCode(db, ctx, progName)
}

func emptyIrCodeChan() chan *IrCode {
	ch := make(chan *IrCode)
	close(ch)
	return ch
}
