package ssadb

import (
	"database/sql/driver"
	"errors"
	"reflect"
	"strconv"
	"strings"
)

// Uint64Slice 是一个自定义类型，用于处理 []uint64 的序列化和反序列化
type Uint64Slice []uint64

// Scan 实现了 sql.Scanner 接口，允许从数据库读取值时将其转换回 Uint64Slice 类型
func (us *Uint64Slice) Scan(value interface{}) error {
	if value == nil {
		*us = nil
		return nil
	}

	var strValue string
	switch v := value.(type) {
	case []byte:
		strValue = string(v)
	case string:
		strValue = v
	default:
		return errors.New("unsupported type: " + reflect.TypeOf(value).String() + " for Uint64Slice.Scan")
	}

	// 分割字符串并转换为 uint64
	parts := strings.Split(strValue, ",")
	var result Uint64Slice = make([]uint64, len(parts))
	for i, part := range parts {
		num, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return err
		}
		result[i] = num
	}
	*us = result
	return nil
}

// Value 实现了 driver.Valuer 接口，允许将 Uint64Slice 转换为一个适合存储在数据库中的形式
func (us Uint64Slice) Value() (driver.Value, error) {
	var parts []string
	for _, num := range us {
		parts = append(parts, strconv.FormatUint(num, 10))
	}
	return strings.Join(parts, ","), nil
}
