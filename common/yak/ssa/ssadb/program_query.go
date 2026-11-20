package ssadb

import (
	"errors"

	"github.com/yaklang/gorm"
)

// LoadProgramsByRegexp returns application programs whose name matches the
// pattern. The regexp operator differs between PostgreSQL ("~") and the other
// supported dialects ("REGEXP"), so the SQL stays in ssadb where the dialect
// helpers live.
func LoadProgramsByRegexp(db *gorm.DB, match string) ([]*IrProgram, error) {
	if db == nil {
		return nil, errors.New("SSA IR database is not configured")
	}
	column := QuoteIdent(db, "program_name")
	var programs []*IrProgram
	err := db.Model(&IrProgram{}).
		Where(RegexpPredicate(db, column)+" OR "+column+" = ?", match, match).
		Where("program_kind = ?", Application).
		Find(&programs).Error
	if err != nil {
		return nil, err
	}
	return programs, nil
}
