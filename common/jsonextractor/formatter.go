package jsonextractor

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func rawKeyFormatted(i any) string {
	if i == nil {
		return ""
	}
	// 处理 key 的格式
	return keyFormatted(fmt.Sprint(i))
}

func keyFormatted(i string) string {
	// 处理 key 的格式
	// 1. 去掉前后的空格
	// 2. 去掉前后的引号
	// 3. 转换为小写
	trimmed := strings.TrimSpace(i)
	if strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
		unquoted, err := strconv.Unquote(trimmed)
		if err != nil {
			trimmed = trimmed[1 : len(trimmed)-1]
		} else {
			trimmed = unquoted
		}
	}
	return trimmed
}

type RAW_VALUE_TYPE int

const (
	RAW_VALUE_TYPE_RAW RAW_VALUE_TYPE = 0
	RAW_VALUE_TYPE_ARR RAW_VALUE_TYPE = 1
	RAW_VALUE_TYPE_MAP RAW_VALUE_TYPE = 2
)

func rawValueFormatter(data any) (RAW_VALUE_TYPE, any, map[string]any, []any) {
	if data == nil {
		return RAW_VALUE_TYPE_RAW, nil, nil, nil
	}

	switch data.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		// handle key value
		var trimmedValue = strings.TrimSpace(fmt.Sprint(data))
		lowerTrimmedValue := strings.ToLower(trimmedValue)
		if lowerTrimmedValue == "true" {
			data = true
		} else if lowerTrimmedValue == "false" {
			data = false
		} else if lowerTrimmedValue == "null" {
			data = nil
		} else if lowerTrimmedValue == "undefined" {
			data = nil
		} else if matched, _ := regexp.Match(`^\d+$`, []byte(lowerTrimmedValue)); matched {
			data, _ = strconv.ParseInt(lowerTrimmedValue, 10, 64)
			data = int(data.(int64))
		} else if matched, _ := regexp.Match(`^\d+\.\d+`, []byte(lowerTrimmedValue)); matched {
			data, _ = strconv.ParseFloat(lowerTrimmedValue, 64)
		} else if strings.HasPrefix(trimmedValue, `"`) && strings.HasSuffix(trimmedValue, `"`) {
			unquoted, err := strconv.Unquote(trimmedValue)
			if err != nil {
				data = trimmedValue[1 : len(trimmedValue)-1]
			} else {
				data = unquoted
			}
		} else {
			data = trimmedValue
		}
		return RAW_VALUE_TYPE_RAW, data, nil, nil
	}

	rdata := reflect.ValueOf(data)
	if rdata.Kind() == reflect.Map {
		// handle map
		if rdata.IsNil() {
			return RAW_VALUE_TYPE_MAP, nil, make(map[string]any), nil
		}

		iter := rdata.MapRange()
		allnumber := true
		type arrayKV struct {
			Key   int
			Value any
		}
		type mapKV struct {
			Key   string
			Value any
		}
		simpleKVs := make([]arrayKV, 0)
		mapKVs := make([]mapKV, 0)
		for iter.Next() {
			k := iter.Key().Interface()
			v := iter.Value().Interface()

			// 检查 key 是否为数字
			switch k.(type) {
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				// 是数字类型，继续检查
				keyStr := fmt.Sprint(k)
				keyInt, _ := strconv.ParseInt(keyStr, 10, 64)
				var result any
				if t, anyVal, mapVal, arrVal := rawValueFormatter(v); t == RAW_VALUE_TYPE_RAW {
					result = anyVal
				} else if t == RAW_VALUE_TYPE_MAP {
					result = mapVal
				} else if t == RAW_VALUE_TYPE_ARR {
					result = arrVal
				} else {
					result = anyVal
				}
				simpleKVs = append(simpleKVs, arrayKV{
					Key:   int(keyInt),
					Value: result,
				})
				continue
			default:
				// 不是数字类型，设置 allnumber 为 false
				allnumber = false
				var result any
				if t, anyVal, mapVal, arrVal := rawValueFormatter(v); t == RAW_VALUE_TYPE_RAW {
					result = anyVal
				} else if t == RAW_VALUE_TYPE_MAP {
					result = mapVal
				} else if t == RAW_VALUE_TYPE_ARR {
					result = arrVal
				} else {
					result = anyVal
				}
				mapKVs = append(mapKVs, mapKV{
					Key:   rawKeyFormatted(k),
					Value: result,
				})
			}
		}

		if allnumber {
			// 按照 Key 排序 simpleKVs
			sort.Slice(simpleKVs, func(i, j int) bool {
				return simpleKVs[i].Key < simpleKVs[j].Key
			})

			// 将排序后的值放入 values 数组
			values := make([]any, len(simpleKVs))
			for i, kv := range simpleKVs {
				values[i] = kv.Value
			}
			return RAW_VALUE_TYPE_ARR, nil, nil, values
		} else {
			vals := make(map[string]any)
			for _, kv := range mapKVs {
				vals[kv.Key] = kv.Value
			}
			return RAW_VALUE_TYPE_MAP, nil, vals, nil
		}
	}
	return RAW_VALUE_TYPE_RAW, data, nil, nil
}

func (c *callbackManager) kv(key, data any, parents []string) {
	// raw key value callback
	originKey := key
	originValue := data

	if strings.TrimSpace(fmt.Sprint(originValue)) == "" {
		return
	}

	if c.rawKVCallback != nil {
		c.rawKVCallback(originKey, originValue)
	}

	var autoData any = data
	valType, anyData, mapData, arrData := rawValueFormatter(data)
	switch valType {
	case RAW_VALUE_TYPE_RAW:
		autoData = anyData
	case RAW_VALUE_TYPE_MAP:
		autoData = mapData
	case RAW_VALUE_TYPE_ARR:
		autoData = arrData
	default:
		autoData = anyData
	}

	if c.formatKVCallback != nil {
		c.formatKVCallback(rawKeyFormatted(key), autoData, parents)
	}

	switch key.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		keyInt, _ := strconv.ParseInt(fmt.Sprintf("%d", key), 10, 64)
		newKey := keyInt
		if c.arrayValueCallback != nil {
			c.arrayValueCallback(int(newKey), autoData)
		}
	default:
		if c.objectKeyValueCallback != nil {
			c.objectKeyValueCallback(rawKeyFormatted(key), autoData)
		}
	}

	if valType == RAW_VALUE_TYPE_ARR {
		if c.onArrayCallback != nil {
			c.onArrayCallback(arrData)
		}
	} else if valType == RAW_VALUE_TYPE_MAP {
		if c.onObjectCallback != nil {
			c.onObjectCallback(mapData)
		}
		for _, callback := range c.onConditionalObjectCallback {
			callback.Feed(mapData)
		}

		if originKey == nil {
			if c.onRootMapCallback != nil {
				c.onRootMapCallback(mapData)
			}
		}
	}
}
