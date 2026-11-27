package ssadb

import (
	"errors"
	"fmt"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// FinalizeStagedProgramChecked makes staged visible as target.
//
// A producer that replaces an existing program must build the new one under a
// staging name first. The old program then stays readable until the new one is
// complete, and this call swaps them in one transaction. When replace is
// false and target appeared while staged was being built, the transaction
// fails and leaves both programs untouched.
//
// staged must not have audit, risk or scan-task rows: save builds a fresh
// program, and those rows belong to the program being replaced.
func FinalizeStagedProgramChecked(db *gorm.DB, staged, target string, replace bool) error {
	if err := validateProgramReplacement(db, staged, target); err != nil {
		return err
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&IrProgram{}).Where("program_name = ?", staged).Count(&count).Error; err != nil {
			return fmt.Errorf("count staged program %q: %w", staged, err)
		}
		if count == 0 {
			return fmt.Errorf("staged program %q does not exist", staged)
		}
		if err := tx.Model(&IrProgram{}).Where("program_name = ?", target).Count(&count).Error; err != nil {
			return fmt.Errorf("count target program %q: %w", target, err)
		}
		if count > 0 {
			if !replace {
				return fmt.Errorf("program %q already exists", target)
			}
			if err := deleteProgramRowsForReplacement(tx, target); err != nil {
				return fmt.Errorf("delete program %q: %w", target, err)
			}
		}
		if err := renameProgramRowsForReplacement(tx, staged, target); err != nil {
			return fmt.Errorf("rename program %q to %q: %w", staged, target, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	forgetProgramCaches(staged)
	forgetProgramCaches(target)
	return nil
}

func validateProgramReplacement(db *gorm.DB, staged, target string) error {
	if err := validateProgramDeletion(db, staged); err != nil {
		return fmt.Errorf("staged program: %w", err)
	}
	if err := validateProgramDeletion(db, target); err != nil {
		return fmt.Errorf("target program: %w", err)
	}
	if staged == target {
		return errors.New("staged and target program names are equal")
	}
	return nil
}

// deleteProgramRowsForReplacement removes the previous target inside the
// caller's transaction. It mirrors DeleteProgramChecked, but returns the first
// failure so the transaction can roll back instead of retrying a table that
// will never recover in this call.
func deleteProgramRowsForReplacement(tx *gorm.DB, program string) error {
	for _, table := range []string{
		TableIrCodes, TableIrIndices, TableIrNamePool, TableIrSources,
		TableIrTypes, TableIrOffsets,
		TableAuditResults, TableAuditNodes, TableAuditEdges,
		schema.TableSSARisks, schema.TableSyntaxFlowScanTask,
	} {
		if !tx.HasTable(table) {
			return fmt.Errorf("%s: table is missing", table)
		}
		column := "program_name"
		if table == schema.TableSyntaxFlowScanTask {
			column = "programs"
		}
		if err := nativeExec(tx, `DELETE FROM `+QuoteIdent(tx, table)+` WHERE `+QuoteIdent(tx, column)+` = ?`, program); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	for _, table := range []string{TableIrConsts, TableIrConstPool} {
		if !tx.HasTable(table) {
			continue
		}
		if err := nativeExec(tx, `DELETE FROM `+QuoteIdent(tx, table)+` WHERE `+QuoteIdent(tx, "program_name")+` = ?`, program); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	if err := nativeExec(tx, `DELETE FROM `+QuoteIdent(tx, TableIrSources)+` WHERE `+QuoteIdent(tx, "folder_path")+` = ? AND `+QuoteIdent(tx, "file_name")+` = ?`, "/", program); err != nil {
		return fmt.Errorf("%s folder row: %w", TableIrSources, err)
	}
	if err := nativeExec(tx, `DELETE FROM `+QuoteIdent(tx, TableIrPrograms)+` WHERE `+QuoteIdent(tx, "program_name")+` = ?`, program); err != nil {
		return fmt.Errorf("%s: %w", TableIrPrograms, err)
	}
	return nil
}

// renameProgramRowsForReplacement moves the staged rows to the target name.
// The program identity is updated last, after every child table.
func renameProgramRowsForReplacement(tx *gorm.DB, staged, target string) error {
	for _, table := range []string{
		TableIrCodes, TableIrIndices, TableIrNamePool, TableIrSources,
		TableIrTypes, TableIrOffsets,
	} {
		if !tx.HasTable(table) {
			return fmt.Errorf("%s: table is missing", table)
		}
		if err := nativeExec(tx, `UPDATE `+QuoteIdent(tx, table)+` SET `+QuoteIdent(tx, "program_name")+` = ? WHERE `+QuoteIdent(tx, "program_name")+` = ?`, target, staged); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	for _, table := range []string{TableIrConsts, TableIrConstPool} {
		if !tx.HasTable(table) {
			continue
		}
		if err := nativeExec(tx, `UPDATE `+QuoteIdent(tx, table)+` SET `+QuoteIdent(tx, "program_name")+` = ? WHERE `+QuoteIdent(tx, "program_name")+` = ?`, target, staged); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	if err := nativeExec(tx, `UPDATE `+QuoteIdent(tx, TableIrSources)+` SET `+QuoteIdent(tx, "program_name")+` = ?, `+QuoteIdent(tx, "file_name")+` = ? WHERE `+QuoteIdent(tx, "folder_path")+` = ? AND `+QuoteIdent(tx, "file_name")+` = ?`,
		target, target, "/", staged); err != nil {
		return fmt.Errorf("%s folder row: %w", TableIrSources, err)
	}
	if err := nativeExec(tx, `UPDATE `+QuoteIdent(tx, TableIrPrograms)+` SET `+QuoteIdent(tx, "program_name")+` = ? WHERE `+QuoteIdent(tx, "program_name")+` = ?`, target, staged); err != nil {
		return fmt.Errorf("%s: %w", TableIrPrograms, err)
	}
	return nil
}

func forgetProgramCaches(program string) {
	deleteCache(program)
	DropIrConstPool(program)
	DropIrConstValueCache(program)
}
