package ssa

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"slices"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Fingerprint the complete IR payload, including in-place relation edits.
// Database row identity and timestamps are assigned by GORM, not the compiler.
func irCodePersistenceFingerprint(ir *ssadb.IrCode) ([sha256.Size]byte, error) {
	snapshot := *ir
	snapshot.Model = gorm.Model{}
	// Match the actual write hook before JSON's invalid-UTF-8 replacement.
	// Copy the string slice so normalization cannot mutate the source record.
	snapshot.Variable = slices.Clone(ir.Variable)
	if err := snapshot.BeforeSave(nil); err != nil {
		return [sha256.Size]byte{}, err
	}
	// Int64Map contains private key/value fields, so ordinary JSON loses the
	// relation IDs. Use every column's SQL value, also equating nil/empty
	// slices whose driver.Valuer stores the same empty string.
	row := reflect.ValueOf(&snapshot).Elem()
	columns := make([]any, 0, row.NumField()-1)
	for i := 0; i < row.NumField(); i++ {
		field := row.Type().Field(i)
		if field.Name == "Model" {
			continue
		}
		value := row.Field(i).Interface()
		if valuer, ok := value.(driver.Valuer); ok {
			var err error
			value, err = valuer.Value()
			if err != nil {
				return [sha256.Size]byte{}, err
			}
		}
		columns = append(columns, value)
	}
	encoded, err := json.Marshal(columns)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}
