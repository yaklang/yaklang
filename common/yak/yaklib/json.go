package yaklib

import (
	"encoding/json"
	"reflect"
	"strings"
	"yaklang/common/jsonextractor"
	"yaklang/common/jsonpath"
	"yaklang/common/log"
	"yaklang/common/utils"
)

var JsonExports = map[string]interface{}{
	"New":     _yakJson,
	"Marshal": json.Marshal,
	"dumps":   _jsonDumps,
	"loads":   _jsonLoad,

	// 这是 JSONPath 模块
	"Find":          jsonpath.Find,
	"FindPath":      jsonpath.FindFirst,
	"ReplaceAll":    jsonpath.ReplaceAll,
	"ExtractJSON":   jsonextractor.ExtractStandardJSON,
	"ExtractJSONEx": jsonextractor.ExtractJSONWithRaw,
}

func _jsonDumps(raw interface{}) string {
	bytes, err := json.Marshal(raw)
	if err != nil {
		log.Errorf("parse error: %v", err)
		return ""
	}
	return string(bytes)
}

func _jsonLoad(raw interface{}) interface{} {
	var i interface{}
	var defaultValue = make(map[string]interface{})

	str := utils.InterfaceToString(raw)
	str = strings.TrimSpace(str)
	err := json.Unmarshal([]byte(str), &i)
	if err != nil {
		// 尝试解码
		if strings.Contains(err.Error(), `character 'x'`) {
			fixed := string(jsonextractor.FixJson([]byte(str)))
			if fixed != "" {
				str = fixed
			}
			err := json.Unmarshal([]byte(str), &i)
			if err == nil {
				return i
			}
		}

		// 如果 JSON 解码失败则尝试修复一下
		if strings.HasPrefix(str, "{") {
			fixed, ok := jsonextractor.JsonValidObject([]byte(str))
			if ok {
				err := json.Unmarshal([]byte(fixed), &i)
				if err == nil {
					return i
				}
			}
		}
		log.Error(err)
		return defaultValue
	}
	return i
}

type yakJson struct {
	origin     interface{}
	jsonObject interface{}
}

// 判断是不是 map/object {}
func (y *yakJson) IsObject() bool {
	return y.jsonObject != nil && reflect.TypeOf(y.jsonObject).Kind() == reflect.Map
}

func (y *yakJson) IsMap() bool {
	return y.IsObject()
}

// 判断是不是 []
func (y *yakJson) IsSlice() bool {
	return y.jsonObject != nil && ((reflect.TypeOf(y.jsonObject).Kind() == reflect.Slice) ||
		(reflect.TypeOf(y.jsonObject).Kind() == reflect.Array))
}

func (y *yakJson) IsArray() bool {
	return y.IsSlice()
}

// 判断是不是 null
func (y *yakJson) IsNil() bool {
	return y.jsonObject == nil
}

func (y *yakJson) IsNull() bool {
	return y.IsNil()
}

// 判断是不是 string
func (y *yakJson) IsString() bool {
	return y.jsonObject != nil && (reflect.TypeOf(y.jsonObject).Kind() == reflect.String)
}

// 判断是不是 number
func (y *yakJson) IsNumber() bool {
	return y.jsonObject != nil && (reflect.TypeOf(y.jsonObject).Kind() == reflect.Float64 ||
		reflect.TypeOf(y.jsonObject).Kind() == reflect.Int ||
		reflect.TypeOf(y.jsonObject).Kind() == reflect.Int64 ||
		reflect.TypeOf(y.jsonObject).Kind() == reflect.Uint64 ||
		reflect.TypeOf(y.jsonObject).Kind() == reflect.Float32 ||
		reflect.TypeOf(y.jsonObject).Kind() == reflect.Int)
}

func (y *yakJson) Value() interface{} {
	return y.jsonObject
}

func _yakJson(i interface{}) (*yakJson, error) {
	j := &yakJson{}

	var raw interface{}
	j.origin = i

	switch ret := i.(type) {
	case []byte:
		err := json.Unmarshal(ret, &raw)
		if err != nil {
			return nil, err
		}
	case string:
		err := json.Unmarshal([]byte(ret), &raw)
		if err != nil {
			return nil, err
		}
	default:
		rawBytes, err := json.Marshal(ret)
		if err != nil {
			return nil, utils.Errorf("marshal input{%#v} failed: %v", ret, err)
		}

		err = json.Unmarshal(rawBytes, &raw)
		if err != nil {
			return nil, err
		}
	}
	j.jsonObject = raw

	return j, nil
}
