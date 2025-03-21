package yaklib

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
)

var literalReflectType_OrderedMap = reflect.TypeOf((*orderedmap.OrderedMap)(nil))

type jsonConfig struct {
	prefix string
	indent string
}

type JsonOpt func(opt *jsonConfig)

var JsonExports = map[string]interface{}{
	"New":        _yakJson,
	"Marshal":    _jsonMarshal,
	"dumps":      _jsonDumps,
	"loads":      _jsonLoad,
	"withPrefix": _withPrefix,
	"withIndent": _withIndent,

	// 这是 JSONPath 模块
	"Find":          jsonpath.Find,
	"FindPath":      jsonpath.FindFirst,
	"ReplaceAll":    jsonpath.ReplaceAll,
	"ExtractJSON":   jsonextractor.ExtractStandardJSON,
	"ExtractJSONEx": jsonextractor.ExtractJSONWithRaw,
}

func NewJsonConfig() *jsonConfig {
	return &jsonConfig{
		prefix: "",
		indent: "  ",
	}
}

// withPrefix 设置 JSON dumps时的前缀
// Example:
// ```
// v = json.dumps({"a": "b", "c": "d"}, json.withPrefix("  "))
// ```
func _withPrefix(prefix string) JsonOpt {
	return func(opt *jsonConfig) {
		opt.prefix = prefix
	}
}

// withIndent 设置 JSON dumps时的缩进
// Example:
// ```
// v = json.dumps({"a": "b", "c": "d"}, json.withIndent("  "))
// ```
func _withIndent(indent string) JsonOpt {
	return func(opt *jsonConfig) {
		opt.indent = indent
	}
}

// Marshal 将一个对象转换为 JSON bytes，返回转换后的 bytes 与错误
// Example:
// ```
// v, err = json.Marshal({"a": "b", "c": "d"})
// // v = b"{"a": "b", "c": "d"}"
// ```
func _jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// dumps 将一个对象转换为 JSON 字符串，返回转换后的字符串
// 它还可以接收零个到多个请求选项函数，用于配置转换过程，控制转换后的缩进，前缀等
// Example:
// ```
// v = json.dumps({"a": "b", "c": "d"})
// ```
func _jsonDumps(raw interface{}, opts ...JsonOpt) string {
	config := NewJsonConfig()
	for _, opt := range opts {
		opt(config)
	}
	var (
		bytes []byte
		err   error
	)

	if config.prefix == "" && config.indent == "" {
		bytes, err = json.Marshal(raw)
	} else {
		bytes, err = json.MarshalIndent(raw, config.prefix, config.indent)
	}

	if err != nil {
		log.Errorf("json dumps error: %v", err)
		return ""
	}
	return string(bytes)
}

// loads 将一个 JSON 字符串转换为对象，返回转换后的对象，通常是一个omap
// Example:
// ```
// v = json.loads(`{"a": "b", "c": "d"}`)
// ```
func _jsonLoad(raw interface{}, opts ...JsonOpt) interface{} {
	// opts 中暂时没有load的选项，所以这里暂时不处理
	i := orderedmap.New()

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

		var v any
		err = json.Unmarshal([]byte(str), &v)
		if err == nil {
			return v
		}

		log.Error(err)
		return i
	}
	return i
}

type yakJson struct {
	origin     interface{}
	jsonObject interface{}
}

// 判断是不是 map/object {}
func (y *yakJson) IsObject() bool {
	refTyp := reflect.TypeOf(y.jsonObject)
	return y.jsonObject != nil && (refTyp.Kind() == reflect.Map || refTyp == literalReflectType_OrderedMap)
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

// New 根据传入的值创建并返回一个新的 JSON 对象与错误
// Example:
// ```
// v, err = json.New("foo")
// v, err = json.New(b"bar")
// v, err = json.New({"a": "b", "c": "d"})
// ```
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

// Find 使用 JSONPath 查找并返回 JSON 中的所有值
// Example:
// ```
// v = json.Find(`{"a":"a1","c":{"a":"a2"}}`, "$..a") // v = [a1, a2]
// ```
func _jsonpathFind(json interface{}, jsonPath string) interface{} {
	return jsonpath.Find(json, jsonPath)
}

// FindPath 使用 JSONPath 查找并找回啊 JSON 中的第一个值
// Example:
// ```
//
//	v = json.Find(`{"a":"a1","c":{"a":"a2"}}`, "$..a") // v = a1
//
// ```
func _jsonpathFindPath(json interface{}, jsonPath string) interface{} {
	return jsonpath.FindFirst(json, jsonPath)
}

// ReplaceAll 使用 JSONPath 替换 JSON 中的所有值，返回替换后的 JSON map
// Example:
// ```
// v = json.ReplaceAll(`{"a":"a1","c":{"a":"a2"}}`, "$..a", "b") // v = {"a":"b","c":{"a":"b"}}
// ```
func _jsonpathReplaceAll(json interface{}, jsonPath string, replaceValue interface{}) any {
	return jsonpath.ReplaceAll(json, jsonPath, replaceValue)
}

// ExtractJSON 从一段文本中中提取所有修复后的 JSON 字符串
// Example:
// ```
// v = json.ExtractJSON(`Here is your result: {"a": "b"} and {"c": "d"}`)
// // v = ["{"a": "b"}", "{"c": "d"}"]
// ```
func _jsonpathExtractJSON(raw string) []string {
	return jsonextractor.ExtractStandardJSON(raw)
}

// ExtractJSONEx 从一段文本中中提取所有修复后的 JSON 字符串，返回修复后的 JSON 字符串与修复前的 JSON 字符串
func _jsonpathExtractJSONEx(raw string) (results []string, rawStr []string) {
	return jsonextractor.ExtractJSONWithRaw(raw)
}
