package ssadb

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
)

// DM rejects a second index on the same ordered columns. Reuse equivalent
// indexes, and replace an equivalent non-unique index only when installing the
// IR uniqueness invariant. Existing duplicate data fails before index removal.
func installDMIRIndexes(db *gorm.DB) error {
	for _, spec := range []struct {
		name, table, columns string
		unique               bool
	}{
		{"ux_ir_codes_program_code", TableIrCodes, "program_name,code_id", true},
		{"ux_ir_offsets_program_value_file_range", TableIrOffsets, "program_name,value_id,file_hash,start_offset,end_offset,COALESCE(variable_name,'')", true},
		{"idx_ir_codes_program_opcode", TableIrCodes, "program_name,opcode", false},
		{"ux_ir_types_program_type", TableIrTypes, "program_name,type_id", true},
		{"idx_ir_indices_program_value", TableIrIndices, "program_name,value_id", false},
		{"idx_ir_name_pool_program_name_name", TableIrNamePool, "program_name,name", false},
	} {
		if db.Dialect().HasIndex(spec.table, spec.name) {
			continue
		}
		rows, err := db.DB().Query(`SELECT I.INDEX_NAME,I.UNIQUENESS,C.COLUMN_NAME FROM ALL_INDEXES I JOIN ALL_IND_COLUMNS C ON C.INDEX_OWNER=I.OWNER AND C.INDEX_NAME=I.INDEX_NAME WHERE I.OWNER=SYS_CONTEXT('USERENV','CURRENT_SCHEMA') AND I.TABLE_NAME=? ORDER BY I.INDEX_NAME,C.COLUMN_POSITION`, strings.ToUpper(spec.table))
		if err != nil {
			return err
		}
		type definition struct {
			unique  bool
			columns []string
		}
		indexes := map[string]*definition{}
		for rows.Next() {
			var name, unique string
			var column sql.NullString
			if err = rows.Scan(&name, &unique, &column); err != nil {
				rows.Close()
				return err
			}
			idx := indexes[name]
			if idx == nil {
				idx = &definition{unique: unique == "UNIQUE"}
				indexes[name] = idx
			}
			idx.columns = append(idx.columns, strings.ToLower(column.String))
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		matched := []string{}
		reused := false
		for name, index := range indexes {
			if strings.Join(index.columns, ",") != spec.columns {
				continue
			}
			if !spec.unique || index.unique {
				reused = true
				break
			}
			matched = append(matched, name)
		}
		if reused {
			continue
		}
		if spec.unique {
			var duplicates int64
			if err = db.DB().QueryRow("SELECT COUNT(*) FROM (SELECT " + spec.columns + " FROM " + spec.table + " GROUP BY " + spec.columns + " HAVING COUNT(*)>1)").Scan(&duplicates); err != nil {
				return err
			}
			if duplicates > 0 {
				return fmt.Errorf("DM IR index %s requires explicit duplicate repair", spec.name)
			}
		}
		for _, name := range matched {
			if err = db.Exec("DROP INDEX " + db.Dialect().Quote(name)).Error; err != nil {
				return err
			}
		}
		indexName := spec.name
		if len(matched) > 0 {
			indexName = matched[0]
		}
		kind := ""
		if spec.unique {
			kind = "UNIQUE "
		}
		if err = db.Exec("CREATE " + kind + "INDEX " + db.Dialect().Quote(indexName) + " ON " + db.Dialect().Quote(spec.table) + " (" + spec.columns + ")").Error; err != nil {
			return err
		}
	}
	return nil
}
