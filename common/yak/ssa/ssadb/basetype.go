package ssadb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// type Int64Map map[int64]int64
type item[T any] struct {
	key, value T
}

type Int64Map []item[int64]

func (m *Int64Map) Append(key, value int64) {
	*m = append(*m, item[int64]{key, value})
}

func (m Int64Map) ForEach(fn func(key, value int64)) {
	for _, item := range m {
		fn(item.key, item.value)
	}
}

func (m *Int64Map) Scan(value any) error {
	if m == nil {
		return nil
	}
	val := codec.AnyToString(value)
	subVal := strings.Split(val, ",")
	nm := make(Int64Map, 0, len(subVal))
	for _, sub := range subVal {
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
		nm.Append(nmKey, nmVal)
	}
	*m = nm
	return nil
}

func (m Int64Map) Value() (driver.Value, error) {
	var parts []string
	m.ForEach(func(key, value int64) {
		parts = append(parts, strconv.FormatInt(key, 10)+":"+strconv.FormatInt(value, 10))
	})
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
	if strValue == "" {
		*us = nil
		return nil
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

type StringSlice []string

func (ss *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*ss = nil
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

	if strValue == "" {
		*ss = nil
		return nil
	}

	parts := strings.Split(strValue, ",")
	*ss = parts
	return nil
}

func (us StringSlice) Value() (driver.Value, error) {
	return strings.Join(us, ","), nil
}

type StringMap map[string]string

func (m StringMap) Value() (driver.Value, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (m *StringMap) Scan(value any) error {
	if m == nil {
		return nil
	}
	val := codec.AnyToBytes(value)
	if err := json.Unmarshal(val, m); err != nil {
		log.Errorf("failed to unmarshal string(%#v) map: %v", string(val), err)
		*m = make(StringMap)
		(*m)[string(val)] = codec.Md5(string(val))
		return nil
	}
	return nil
}
