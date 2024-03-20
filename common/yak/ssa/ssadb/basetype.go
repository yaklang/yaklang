package ssadb

import (
	"database/sql/driver"
	"errors"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"reflect"
	"strconv"
	"strings"
)

type Int64Map map[int64]int64

func (m *Int64Map) Scan(value any) error {
	if m == nil {
		return nil
	}
	val := codec.AnyToString(value)
	nm := make(map[int64]int64)
	for _, sub := range strings.Split(val, ",") {
		subVals := strings.Split(sub, ":")
		if len(subVals) != 2 {
			continue
		}
		nmKey, err := strconv.ParseInt(subVals[0], 10, 64)
		if err != nil {
			continue
		}
		nmVal, err := strconv.ParseInt(subVals[1], 10, 64)
		if err != nil {
			continue
		}
		nm[nmKey] = nmVal
	}
	*m = nm
	return nil
}

func (m Int64Map) Value() (driver.Value, error) {
	var parts []string
	for k, v := range m {
		parts = append(parts, strconv.FormatInt(k, 10)+":"+strconv.FormatInt(v, 10))
	}
	return strings.Join(parts, ","), nil
}

// Int64Slice 是一个自定义类型，用于处理 []int64 的序列化和反序列化
type Int64Slice []int64

// Scan 实现了 sql.Scanner 接口，允许从数据库读取值时将其转换回 Int64Slice 类型
func (us *Int64Slice) Scan(value interface{}) error {
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
		return errors.New("unsupported type: " + reflect.TypeOf(value).String() + " for Int64Slice.Scan")
	}

	// 分割字符串并转换为 int64
	parts := strings.Split(strValue, ",")
	var result Int64Slice = make([]int64, len(parts))
	for i, part := range parts {
		num, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return err
		}
		result[i] = num
	}
	*us = result
	return nil
}

// Value 实现了 driver.Valuer 接口，允许将 Int64Slice 转换为一个适合存储在数据库中的形式
func (us Int64Slice) Value() (driver.Value, error) {
	var parts []string
	for _, num := range us {
		parts = append(parts, strconv.FormatInt(num, 10))
	}
	return strings.Join(parts, ","), nil
}
