package schema

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/yaklang/yaklang/common/utils"
)

// MapStringAny 用于在 gorm 模型中存储 map[string]any 类型数据
// 实现了 driver.Valuer 和 sql.Scanner 接口，支持 JSON 序列化存储
type MapStringAny map[string]any

// Value 实现 driver.Valuer 接口，将 map[string]any 转换为 JSON 字符串存储
func (m MapStringAny) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(m)
	return string(bytes), err
}

// Scan 实现 sql.Scanner 接口，将数据库中的 JSON 字符串转换回 map[string]any
func (m *MapStringAny) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return utils.Errorf("unsupported type: %T", value)
	}
	return json.Unmarshal(bytes, m)
}

// StringSlice 用于在 gorm 模型中存储 []string 类型数据
// 实现了 driver.Valuer 和 sql.Scanner 接口，支持 JSON 序列化存储
type StringSlice []string

// Value 实现 driver.Valuer 接口，将 []string 转换为 JSON 字符串存储
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(s)
	return string(bytes), err
}

// Scan 实现 sql.Scanner 接口，将数据库中的 JSON 字符串转换回 []string
func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return utils.Errorf("unsupported type: %T", value)
	}
	return json.Unmarshal(bytes, s)
}
