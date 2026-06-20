package yaklib

import (
	"bytes"
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
	prefix       string
	indent       string
	noEscapeHTML bool
}

type JsonOpt func(opt *jsonConfig)

var JsonExports = map[string]interface{}{
	"New":          _yakJson,
	"Marshal":      _jsonMarshal,
	"dumps":        _jsonDumps,
	"loads":        _jsonLoad,
	"withPrefix":   _withPrefix,
	"withIndent":   _withIndent,
	"noEscapeHTML": _withNoEscapeHTML,

	// 这是 JSONPath 模块
	"Find":          _jsonpathFind,
	"FindPath":      _jsonpathFindPath,
	"ReplaceAll":    _jsonpathReplaceAll,
	"ExtractJSON":   _jsonpathExtractJSON,
	"ExtractJSONEx": _jsonpathExtractJSONEx,
}

func NewJsonConfig() *jsonConfig {
	return &jsonConfig{
		prefix: "",
		indent: "  ",
	}
}

// withPrefix 设置 json.dumps 输出时每一行的前缀（导出名为 json.withPrefix）
// 作为 json.dumps 的可选项使用，常配合 withIndent 控制多行 JSON 的排版
//
// 参数:
//   - prefix: 每行前缀字符串
//
// 返回值:
//   - 可传入 json.dumps 的序列化选项
//
// Example:
// ```
// s = json.dumps({"a": "b"}, json.withPrefix(">>"), json.withIndent("  "))
// println(s)
// assert str.Contains(s, ">>"), "withPrefix should prepend the prefix to indented lines"
// ```
func _withPrefix(prefix string) JsonOpt {
	return func(opt *jsonConfig) {
		opt.prefix = prefix
	}
}

// noEscapeHTML 设置 json.dumps 时不转义 HTML 字符（导出名为 json.noEscapeHTML）
// 默认情况下 <, >, & 会被转义为 \u003c 等；启用该选项后保持原样输出
// 作为 json.dumps 的可选项使用
//
// 返回值:
//   - 可传入 json.dumps 的序列化选项
//
// Example:
// ```
// s = json.dumps({"a": "<x>"}, json.noEscapeHTML())
// println(s)
// assert str.Contains(s, "<x>"), "noEscapeHTML should keep raw HTML characters"
// ```
func _withNoEscapeHTML() JsonOpt {
	return func(opt *jsonConfig) {
		opt.noEscapeHTML = true
	}
}

// withIndent 设置 json.dumps 输出时的缩进字符串（导出名为 json.withIndent）
// 设置后输出为带缩进的多行 JSON，便于阅读；作为 json.dumps 的可选项使用
//
// 参数:
//   - indent: 每一级缩进使用的字符串（如四个空格）
//
// 返回值:
//   - 可传入 json.dumps 的序列化选项
//
// Example:
// ```
// s = json.dumps({"a": "b"}, json.withIndent("    "))
// println(s)
// assert str.Contains(s, "\n"), "withIndent should produce multiline output"
// ```
func _withIndent(indent string) JsonOpt {
	return func(opt *jsonConfig) {
		opt.indent = indent
	}
}

// Marshal 将一个对象序列化为 JSON 字节（导出名为 json.Marshal）
//
// 参数:
//   - v: 要序列化的对象
//
// 返回值:
//   - 序列化后的 JSON 字节
//   - 错误信息（序列化失败时返回）
//
// Example:
// ```
// b = json.Marshal({"a": "b"})~
// println(string(b))   // OUT: {"a":"b"}
// assert string(b) == "{\"a\":\"b\"}", "Marshal should produce compact JSON bytes"
// ```
func _jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// dumps 将一个对象转换为 JSON 字符串，返回转换后的字符串
// 它还可以接收零个到多个请求选项函数，用于配置转换过程，控制转换后的缩进，前缀等
// 参数:
//   - raw: 要序列化的对象
//   - opts: 可选的序列化选项（缩进、前缀、是否转义 HTML 等）
//
// 返回值:
//   - 序列化后的 JSON 字符串，失败返回空字符串
//
// Example:
// ```
// s = json.dumps({"name": "yak"})
// // dumps 默认输出带缩进的多行 JSON，这里打印是否包含被序列化的值
// println(str.Contains(s, "yak"))   // OUT: true
// assert str.Contains(s, "yak"), "dumps output should contain the value"
// ```
func _jsonDumps(raw interface{}, opts ...JsonOpt) string {
	config := NewJsonConfig()
	for _, opt := range opts {
		opt(config)
	}

	// 如果需要禁用 HTML 转义，使用 Encoder
	if config.noEscapeHTML {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if config.prefix != "" || config.indent != "" {
			encoder.SetIndent(config.prefix, config.indent)
		}
		if err := encoder.Encode(raw); err != nil {
			log.Errorf("json dumps error: %v", err)
			return ""
		}
		// Encode 会在末尾添加换行符，需要去掉
		result := buf.String()
		if len(result) > 0 && result[len(result)-1] == '\n' {
			result = result[:len(result)-1]
		}
		return result
	}

	// 默认使用标准 Marshal
	var (
		resultBytes []byte
		err         error
	)

	if config.prefix == "" && config.indent == "" {
		resultBytes, err = json.Marshal(raw)
	} else {
		resultBytes, err = json.MarshalIndent(raw, config.prefix, config.indent)
	}

	if err != nil {
		log.Errorf("json dumps error: %v", err)
		return ""
	}
	return string(resultBytes)
}

// loads 将一个 JSON 字符串转换为对象，返回转换后的对象，通常是一个omap
// 参数:
//   - raw: JSON 字符串（任意可转为字符串的输入）
//   - opts: 可选的解析选项（当前预留）
//
// 返回值:
//   - 解析后的对象，通常是有序 map（omap）
//
// Example:
// ```
// m = json.loads(`{"a": "b", "c": "d"}`)
// println(m["a"])   // OUT: b
// assert m["a"] == "b", "loads should parse the first field"
// assert m["c"] == "d", "loads should parse the second field"
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

// New 根据传入的值创建一个 JSON 对象（导出名为 json.New）
// 返回的对象提供 IsObject/IsArray/IsString/IsNumber/IsNull/Value 等类型判断与取值方法
//
// 参数:
//   - i: 输入值，可为 JSON 字符串、字节或任意可序列化对象
//
// 返回值:
//   - JSON 对象
//   - 错误信息（解析或序列化失败时返回）
//
// Example:
// ```
// v = json.New(`{"a": "b", "c": "d"}`)~
// println(v.IsObject())   // OUT: true
// assert v.IsObject(), "New should recognize a JSON object"
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

// Find 使用 JSONPath 查找并返回 JSON 中匹配的所有值（导出名为 json.Find）
//
// 参数:
//   - json: JSON 字符串或已解析的对象
//   - jsonPath: JSONPath 表达式（如 $..a）
//
// 返回值:
//   - 匹配到的所有值组成的切片
//
// Example:
// ```
// v = json.Find(`{"a":"a1","c":{"a":"a2"}}`, "$..a")
// println(v)
// assert len(v) == 2, "Find with $..a should match two values"
// ```
func _jsonpathFind(json interface{}, jsonPath string) interface{} {
	return jsonpath.Find(json, jsonPath)
}

// FindPath 使用 JSONPath 查找并返回 JSON 中匹配的第一个值（导出名为 json.FindPath）
//
// 参数:
//   - json: JSON 字符串或已解析的对象
//   - jsonPath: JSONPath 表达式（如 $..a）
//
// 返回值:
//   - 第一个匹配到的值
//
// Example:
// ```
// v = json.FindPath(`{"a":"a1","c":{"a":"a2"}}`, "$..a")
// println(v)   // OUT: a1
// assert v == "a1", "FindPath should return the first matched value"
// ```
func _jsonpathFindPath(json interface{}, jsonPath string) interface{} {
	return jsonpath.FindFirst(json, jsonPath)
}

// ReplaceAll 使用 JSONPath 替换 JSON 中所有匹配的值，返回替换后的对象（导出名为 json.ReplaceAll）
//
// 参数:
//   - json: JSON 字符串或已解析的对象
//   - jsonPath: JSONPath 表达式（如 $..a）
//   - replaceValue: 用于替换的新值
//
// 返回值:
//   - 替换后的对象
//
// Example:
// ```
// v = json.ReplaceAll(`{"a":"a1","c":{"a":"a2"}}`, "$..a", "b")
// s = json.dumps(v)
// println(s)
// assert str.Contains(s, "\"b\""), "ReplaceAll should replace matched values with the new value"
// ```
func _jsonpathReplaceAll(json interface{}, jsonPath string, replaceValue interface{}) any {
	return jsonpath.ReplaceAll(json, jsonPath, replaceValue)
}

// ExtractJSON 从一段文本中提取并修复其中包含的所有 JSON 字符串（导出名为 json.ExtractJSON）
// 常用于从 AI 回复、日志等夹杂自然语言的文本中抽取 JSON 片段
//
// 参数:
//   - raw: 包含 JSON 片段的原始文本
//
// 返回值:
//   - 提取并修复后的 JSON 字符串切片
//
// Example:
// ```
// v = json.ExtractJSON(`prefix {"a": "b"} mid {"c": "d"} end`)
// println(len(v))   // OUT: 2
// assert len(v) == 2, "ExtractJSON should extract two JSON fragments"
// ```
func _jsonpathExtractJSON(raw string) []string {
	return jsonextractor.ExtractStandardJSON(raw)
}

// ExtractJSONEx 从一段文本中提取所有 JSON 字符串，同时返回修复后与修复前的版本（导出名为 json.ExtractJSONEx）
// 相比 ExtractJSON，额外返回修复前的原始片段，便于对照
//
// 参数:
//   - raw: 包含 JSON 片段的原始文本
//
// 返回值:
//   - results: 修复后的 JSON 字符串切片
//   - rawStr: 修复前的原始 JSON 字符串切片（无需修复时可能为空）
//
// Example:
// ```
// fixed, raws = json.ExtractJSONEx(`see {"a": "b"}`)
// println(len(fixed))   // OUT: 1
// assert len(fixed) == 1, "ExtractJSONEx should extract one JSON fragment"
// ```
func _jsonpathExtractJSONEx(raw string) (results []string, rawStr []string) {
	return jsonextractor.ExtractJSONWithRaw(raw)
}
