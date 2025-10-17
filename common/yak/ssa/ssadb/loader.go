package ssadb

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/glob"
)

func YieldIrCodesProgramName(rawDB *gorm.DB, ctx context.Context, program string) chan *IrCode {
	db := rawDB.Model(&IrCode{}).Where("program_name = ?", program)
	return yieldIrCodes(db, ctx)
}
func yieldIrCodes(DB *gorm.DB, ctx context.Context) chan *IrCode {
	return bizhelper.YieldModel[*IrCode](ctx, DB)
}

func yieldIrIndex(DB *gorm.DB, ctx context.Context) chan *IrCode {
	db := DB.Model(&IrIndex{})
	outC := make(chan *IrCode)
	filter := make(map[int64]struct{})
	go func() {
		defer close(outC)
		for index := range bizhelper.YieldModel[*IrIndex](ctx, db) {
			if index == nil {
				break
			}
			// skip duplicate
			if _, ok := filter[index.ValueID]; ok {
				continue
			}
			filter[index.ValueID] = struct{}{}
			// get ir code
			code := GetIrCodeById(GetDB(), index.ProgramName, index.ValueID)
			select {
			case <-ctx.Done():
				return
			case outC <- code:
			}
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

func SearchVariable(db *gorm.DB, ctx context.Context, compareMode, matchMod int, value string) chan *IrCode {
	switch compareMode {
	case ExactCompare:
		return ExactSearchVariable(db, ctx, matchMod, value)
	case GlobCompare:
		return GlobSearchVariable(db, ctx, matchMod, value)
	case RegexpCompare:
		return RegexpSearchVariable(db, ctx, matchMod, value)
	}
	return nil
}

func ExactSearchVariable(DB *gorm.DB, ctx context.Context, mod int, value string) chan *IrCode {
	db := DB.Model(&IrIndex{})
	if mod&ConstType != 0 {
		//指定opcode为const
		_db := DB.Model(&IrCode{}).Where("opcode=5 AND const_type = 'normal' AND string=? ", value)
		return yieldIrCodes(_db, ctx)
	}
	switch mod {
	case NameMatch:
		db = db.Where("variable_name = ? OR class_name = ?", value, value)
	case KeyMatch:
		db = db.Where("field_name = ?", value)
	case BothMatch:
		db = db.Where("variable_name = ? OR class_name = ? OR field_name = ?", value, value, value)
	}

	return yieldIrIndex(db, ctx)
}

func GlobSearchVariable(DB *gorm.DB, ctx context.Context, mod int, value string) chan *IrCode {
	regStr := glob.Glob2Regex(value)
	return RegexpSearchVariable(DB, ctx, mod, regStr)
}

func RegexpSearchVariable(DB *gorm.DB, ctx context.Context, mod int, value string) chan *IrCode {
	db := DB.Model(&IrIndex{})
	if mod&ConstType != 0 {
		_db := DB.Model(&IrCode{}).Where("opcode=5 AND const_type = 'normal' AND string REGEXP ?", value)
		return yieldIrCodes(_db, ctx)
	}
	switch mod {
	case NameMatch:
		db = db.Where("variable_name REGEXP ? OR class_name REGEXP ?", value, value)
	case KeyMatch:
		db = db.Where("field_name REGEXP ?", value)
	case BothMatch:
		db = db.Where("variable_name REGEXP ? OR class_name REGEXP ? OR field_name REGEXP ?", value, value, value)
	}
	return yieldIrIndex(db, ctx)
}

func SearchIrCodeByOpcodes(db *gorm.DB, ctx context.Context, opcodes ...int) chan *IrCode {
	db = db.Model(&IrCode{}).Where("opcode in (?)", opcodes)
	return yieldIrCodes(db, ctx)
}

func GetVariableByValue(valueID int64) []*IrIndex {
	db := GetDB()
	var ir []*IrIndex
	if err := db.Model(&IrIndex{}).Where("value_id = ? and variable_name != ?", valueID, "").Find(&ir).Error; err != nil {
		return nil
	}
	return ir
}
