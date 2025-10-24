package ssadb

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/glob"
)

func YieldIrCode(DB *gorm.DB, ctx context.Context, progName string) chan *IrCode {
	db := DB.Model(&IrCode{}).Where("program_name = ?", progName)
	ids := make([]int64, 0)
	if err := db.Pluck("code_id", &ids).Error; err != nil {
		log.Errorf("failed to get ids: %v", err)
	}
	return yieldCodeWithCache(ctx, DB, progName, ids)
}

func yieldIrIndex(DB *gorm.DB, ctx context.Context, progName string) chan *IrCode {
	filter := make(map[int64]struct{})
	db := DB.Model(&IrIndex{})
	indexCh := bizhelper.YieldModel[*IrIndex](ctx, db, bizhelper.WithYieldModel_Fast())
	for index := range indexCh {
		if index == nil {
			break
		}
		// skip duplicate
		if _, ok := filter[index.ValueID]; ok {
			continue
		}
		filter[index.ValueID] = struct{}{}
	}

	ids := lo.MapToSlice(filter, func(id int64, _ struct{}) int64 { return id })
	return yieldCodeWithCache(ctx, GetDB(), progName, ids)
}

func yieldCodeWithCache(ctx context.Context, _ *gorm.DB, progName string, ids []int64) chan *IrCode {
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
	switch compareMode {
	case ExactCompare:
		return ExactSearchVariable(db, ctx, progName, matchMod, value)
	case GlobCompare:
		return GlobSearchVariable(db, ctx, progName, matchMod, value)
	case RegexpCompare:
		return RegexpSearchVariable(db, ctx, progName, matchMod, value)
	}
	return nil
}

func ExactSearchVariable(DB *gorm.DB, ctx context.Context, progName string, mod int, value string) chan *IrCode {
	db := DB.Model(&IrIndex{})
	if mod&ConstType != 0 {
		//指定opcode为const
		_db := DB.Model(&IrCode{}).Where("opcode=5 AND const_type = 'normal' AND string=? ", value)
		return YieldIrCode(_db, ctx, progName)
	}
	switch mod {
	case NameMatch:
		db = db.Where("variable_name = ? OR class_name = ?", value, value)
	case KeyMatch:
		db = db.Where("field_name = ?", value)
	case BothMatch:
		db = db.Where("variable_name = ? OR class_name = ? OR field_name = ?", value, value, value)
	}

	return yieldIrIndex(db, ctx, progName)
}

func GlobSearchVariable(DB *gorm.DB, ctx context.Context, progName string, mod int, value string) chan *IrCode {
	regStr := glob.Glob2Regex(value)
	return RegexpSearchVariable(DB, ctx, progName, mod, regStr)
}

func RegexpSearchVariable(DB *gorm.DB, ctx context.Context, progName string, mod int, value string) chan *IrCode {
	db := DB.Model(&IrIndex{})
	if mod&ConstType != 0 {
		_db := DB.Model(&IrCode{}).Where("opcode=5 AND const_type = 'normal' AND string REGEXP ?", value)
		return YieldIrCode(_db, ctx, progName)
	}
	switch mod {
	case NameMatch:
		db = db.Where("variable_name REGEXP ? OR class_name REGEXP ?", value, value)
	case KeyMatch:
		db = db.Where("field_name REGEXP ?", value)
	case BothMatch:
		db = db.Where("variable_name REGEXP ? OR class_name REGEXP ? OR field_name REGEXP ?", value, value, value)
	}
	return yieldIrIndex(db, ctx, progName)
}

func SearchIrCodeByOpcodes(db *gorm.DB, ctx context.Context, progName string, opcodes ...int) chan *IrCode {
	db = db.Model(&IrCode{}).Where("opcode in (?)", opcodes)
	return YieldIrCode(db, ctx, progName)
}
