package schema

import (
	"database/sql/driver"
	"fmt"
)

// SQLTextBytes preserves the byte API and JSON representation of a text-backed
// payload while binding SQL text. Binding []byte to a DM CLOB hex-encodes it.
type SQLTextBytes []byte

func (b SQLTextBytes) Value() (driver.Value, error) {
	if b == nil {
		return nil, nil
	}
	return string(b), nil
}
func (b *SQLTextBytes) Scan(value interface{}) error {
	switch v := value.(type) {
	case nil:
		*b = nil
	case string:
		*b = append((*b)[:0], v...)
	case []byte:
		*b = append((*b)[:0], v...)
	default:
		return fmt.Errorf("unsupported SQL text value %T", value)
	}
	return nil
}
