package ssadb

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func YieldIrCodesProgramName(db *gorm.DB, ctx context.Context, program string) chan *IrCode {
	db = db.Model(&IrCode{}).Where("program_name = ?", program)
	return yieldIrCodes(db, ctx)
}

func yieldIrCodes(db *gorm.DB, ctx context.Context) chan *IrCode {
	db = db.Model(&IrCode{})
	outC := make(chan *IrCode)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*IrCode
			if _, b := bizhelper.Paging(db, page, 100, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++
			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 100 {
				return
			}
		}
	}()
	return outC
}

func yieldIrVariables(db *gorm.DB, ctx context.Context) chan int64 {
	db = db.Model(&IrVariable{})
	outC := make(chan int64)
	go func() {
		defer close(outC)

		filter := make(map[int64]struct{})

		var page = 1
		for {
			var items []*IrVariable
			if _, b := bizhelper.Paging(db, page, 100, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++
			for _, d := range items {
				for _, id := range d.InstructionID {
					if _, ok := filter[id]; ok {
						continue
					}
					filter[id] = struct{}{}

					select {
					case <-ctx.Done():
						return
					case outC <- id:
					}
				}
			}

			if len(items) < 100 {
				return
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
)

const (
	ExactCompare int = iota
	GlobCompare
	RegexpCompare
)

func SearchVariable(db *gorm.DB, compareMode, matchMod int, value string) chan int64 {
	switch compareMode {
	case ExactCompare:
		return ExactSearchVariable(db, matchMod, value)
	case GlobCompare:
		return GlobSearchVariable(db, matchMod, value)
	case RegexpCompare:
		return RegexpSearchVariable(db, matchMod, value)
	}
	return nil
}

func ExactSearchVariable(DB *gorm.DB, mod int, value string) chan int64 {
	db := DB.Model(&IrVariable{})
	switch mod {
	case NameMatch:
		db = db.Where("variable_name = ?", value)
	case KeyMatch:
		db = db.Where("slice_member_name = ? OR field_member_name = ?", value, value)
	case BothMatch:
		db = db.Where("variable_name = ? OR slice_member_name = ? OR field_member_name = ?", value, value, value)
	}
	return yieldIrVariables(db, context.Background())
}

func GlobSearchVariable(DB *gorm.DB, mod int, value string) chan int64 {
	db := DB.Model(&IrVariable{})
	switch mod {
	case NameMatch:
		db = db.Where("variable_name GLOB ?", value)
	case KeyMatch:
		db = db.Where("slice_member_name GLOB ? OR field_member_name GLOB ?", value, value)
	case BothMatch:
		db = db.Where("variable_name GLOB ? OR slice_member_name GLOB ? OR field_member_name GLOB ?", value, value, value)
	}
	return yieldIrVariables(db, context.Background())
}
func RegexpSearchVariable(DB *gorm.DB, mod int, value string) chan int64 {
	db := DB.Model(&IrVariable{})
	switch mod {
	case NameMatch:
		db = db.Where("variable_name REGEXP ?", value)
	case KeyMatch:
		db = db.Where("slice_member_name REGEXP ? OR field_member_name REGEXP ?", value, value)
	case BothMatch:
		db = db.Where("variable_name REGEXP ? OR slice_member_name REGEXP ? OR field_member_name REGEXP ?", value, value, value)
	}
	return yieldIrVariables(db, context.Background())
}
