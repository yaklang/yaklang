package yaklib

import (
	"io"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

// jsonstream 库提供"数据流 + 回调"风格的 JSON 解析能力，底层复用 common/jsonextractor 字符级流式解析引擎。
// 它与 json 库的区别在于：json 库是整体解析（必须拿到完整文档再 Unmarshal），
// 而 jsonstream 库边读边解析，支持字段级字符流回调、容错解析、常量内存处理大字段，
// 与 Yaklang AI 解析 LLM 数据流（SSE 流）使用的是同一套引擎。
//
// 关键词: jsonstream, 流式 JSON, 数据流回调, jsonextractor
var JsonStreamExports = map[string]interface{}{
	"Extract":             _jsonStreamExtract,
	"ExtractFromReader":   _jsonStreamExtractFromReader,
	"onObject":            _jsonStreamOnObject,
	"onArray":             _jsonStreamOnArray,
	"onKeyValue":          _jsonStreamOnKeyValue,
	"onRawKeyValue":       _jsonStreamOnRawKeyValue,
	"onKeyValueEx":        _jsonStreamOnKeyValueEx,
	"onRootMap":           _jsonStreamOnRootMap,
	"onConditionalObject": _jsonStreamOnConditionalObject,
	"onField":             _jsonStreamOnField,
	"onFields":            _jsonStreamOnFields,
	"onFieldRegexp":       _jsonStreamOnFieldRegexp,
	"onFieldGlob":         _jsonStreamOnFieldGlob,
	"onFinished":          _jsonStreamOnFinished,
	"onError":             _jsonStreamOnError,
}

// Extract 以流式方式解析一段 JSON 内容（字符串或字节），并通过回调选项处理解析结果。
// 它边解析边触发回调，并能容错处理非标准 JSON。
// 参数:
//   - input: 待解析的 JSON 内容(字符串或字节切片)
//   - opts: 一个或多个回调选项，如 jsonstream.onObject(...)
//
// 返回值:
//   - 解析过程中产生的错误
//
// Example:
// ```
// jsonstream.Extract(`{"name": "Alice", "age": 30}`,
//
//	jsonstream.onObject(func(data) {
//	    println("object:", data["name"])
//	}),
//
// )
// ```
func _jsonStreamExtract(input interface{}, opts ...jsonextractor.CallbackOption) error {
	return jsonextractor.ExtractStructuredJSON(utils.InterfaceToString(input), opts...)
}

// ExtractFromReader 从数据流（io.Reader）中以流式方式解析 JSON 内容，适合处理大文件、网络流或边生产边消费的场景。
// 参数:
//   - reader: 提供 JSON 内容的数据流
//   - opts: 一个或多个回调选项，如 jsonstream.onObject(...)
//
// 返回值:
//   - 解析过程中产生的错误
//
// Example:
// ```
// r, w = io.Pipe()
//
//	go func() {
//	    w.WriteString(`{"name": "Alice"}`)
//	    w.Close()
//	}()
//
// jsonstream.ExtractFromReader(r, jsonstream.onObject(func(data) {
//
//	println("object:", data["name"])
//
// }))
// ```
func _jsonStreamExtractFromReader(reader io.Reader, opts ...jsonextractor.CallbackOption) error {
	return jsonextractor.ExtractStructuredJSONFromStream(reader, opts...)
}

// onObject 注册对象回调，当一个完整的 JSON 对象解析完成时触发，回调参数为该对象（map）。
// 参数:
//   - callback: 对象解析完成时调用的回调，参数为解析出的对象 map
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
// jsonstream.Extract(`{"name": "Alice", "age": 30}`, jsonstream.onObject(func(data) {
//
//	println(data["name"])
//
// }))
// ```
func _jsonStreamOnObject(callback func(data map[string]any)) jsonextractor.CallbackOption {
	return jsonextractor.WithObjectCallback(callback)
}

// onArray 注册数组回调，当一个完整的 JSON 数组解析完成时触发，回调参数为该数组（list）。
// 参数:
//   - callback: 数组解析完成时调用的回调，参数为解析出的数组
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
// jsonstream.Extract(`[1, 2, 3]`, jsonstream.onArray(func(data) {
//
//	println(len(data))
//
// }))
// ```
func _jsonStreamOnArray(callback func(data []any)) jsonextractor.CallbackOption {
	return jsonextractor.WithArrayCallback(callback)
}

// onKeyValue 注册键值对回调，解析对象时每遇到一个键值对就触发，回调参数为键名与对应值。
// 参数:
//   - callback: 每个键值对触发的回调，参数为键名与对应的值
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
// jsonstream.Extract(`{"name": "Alice", "age": 30}`, jsonstream.onKeyValue(func(key, value) {
//
//	println(key, "=", value)
//
// }))
// ```
func _jsonStreamOnKeyValue(callback func(key string, data any)) jsonextractor.CallbackOption {
	return jsonextractor.WithObjectKeyValue(callback)
}

// onRawKeyValue 注册原始键值对回调，回调参数为未经处理的原始键与原始值。
// 参数:
//   - callback: 每个键值对触发的回调，参数为原始键与原始值
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
// jsonstream.Extract(`{"name": "Alice"}`, jsonstream.onRawKeyValue(func(key, value) {
//
//	println(key, value)
//
// }))
// ```
func _jsonStreamOnRawKeyValue(callback func(key, data any)) jsonextractor.CallbackOption {
	return jsonextractor.WithRawKeyValueCallback(callback)
}

// onKeyValueEx 注册带父路径的键值对回调，回调参数为键、值以及该键所在的嵌套父路径（list）。
// 参数:
//   - callback: 每个键值对触发的回调，参数为键、值与父路径列表
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
//
//	jsonstream.Extract(`{"user": {"name": "Alice"}}`, jsonstream.onKeyValueEx(func(key, value, parents) {
//	    println(key, "=", value, "parents:", parents)
//	}))
//
// ```
func _jsonStreamOnKeyValueEx(callback func(key, data any, parents []string)) jsonextractor.CallbackOption {
	return jsonextractor.WithFormatKeyValueCallback(callback)
}

// onRootMap 注册根对象回调，仅当顶层 JSON 对象解析完成时触发。
// 参数:
//   - callback: 顶层对象解析完成时调用的回调，参数为根对象 map
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
// jsonstream.Extract(`{"name": "Alice"}`, jsonstream.onRootMap(func(data) {
//
//	println(data["name"])
//
// }))
// ```
func _jsonStreamOnRootMap(callback func(data map[string]any)) jsonextractor.CallbackOption {
	return jsonextractor.WithRootMapCallback(callback)
}

// onConditionalObject 注册条件对象回调，只有当对象同时包含 keys 中列出的所有键时才触发。
// 参数:
//   - keys: 触发回调所需同时包含的键名列表
//   - callback: 满足条件时调用的回调，参数为该对象 map
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
//
//	jsonstream.Extract(`{"name": "Alice", "email": "a@b.com"}`,
//	    jsonstream.onConditionalObject(["name", "email"], func(data) {
//	        println("user:", data["name"], data["email"])
//	    }),
//	)
//
// ```
func _jsonStreamOnConditionalObject(keys []string, callback func(data map[string]any)) jsonextractor.CallbackOption {
	return jsonextractor.WithRegisterConditionalObjectCallback(keys, callback)
}

// onField 为指定字段注册字符级流式处理器，解析过程中字段值逐字符写入 reader，无需等待字段完整。
// 回调参数为字段名、数据流 reader（可用 io.ReadAll 消费）以及父路径。该回调在独立 goroutine 中执行。
// 参数:
//   - fieldName: 要处理的字段名
//   - handler: 字段流处理器，参数为键名、字段值的数据流 reader 与父路径
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
//
//	jsonstream.Extract(`{"content": "very long text..."}`,
//	    jsonstream.onField("content", func(key, reader, parents) {
//	        data = io.ReadAll(reader)~
//	        println(key, "size:", len(data))
//	    }),
//	)
//
// ```
func _jsonStreamOnField(fieldName string, handler func(key string, reader io.Reader, parents []string)) jsonextractor.CallbackOption {
	return jsonextractor.WithRegisterFieldStreamHandler(fieldName, handler)
}

// onFields 为多个字段注册统一的字符级流式处理器，任意一个字段名匹配即触发（包含匹配，大小写不敏感）。
// 参数:
//   - fieldNames: 要处理的字段名列表
//   - handler: 字段流处理器，参数为键名、字段值的数据流 reader 与父路径
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
//
//	jsonstream.Extract(`{"data1": "aaa", "data2": "bbb"}`,
//	    jsonstream.onFields(["data1", "data2"], func(key, reader, parents) {
//	        data = io.ReadAll(reader)~
//	        println(key, string(data))
//	    }),
//	)
//
// ```
func _jsonStreamOnFields(fieldNames []string, handler func(key string, reader io.Reader, parents []string)) jsonextractor.CallbackOption {
	return jsonextractor.WithRegisterMultiFieldStreamHandler(fieldNames, handler)
}

// onFieldRegexp 使用正则表达式匹配字段名，为匹配的字段注册字符级流式处理器。
// 参数:
//   - pattern: 用于匹配字段名的正则表达式
//   - handler: 字段流处理器，参数为键名、字段值的数据流 reader 与父路径
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
//
//	jsonstream.Extract(`{"user_name": "alice", "user_age": 25}`,
//	    jsonstream.onFieldRegexp("^user_.*", func(key, reader, parents) {
//	        data = io.ReadAll(reader)~
//	        println(key, string(data))
//	    }),
//	)
//
// ```
func _jsonStreamOnFieldRegexp(pattern string, handler func(key string, reader io.Reader, parents []string)) jsonextractor.CallbackOption {
	return jsonextractor.WithRegisterRegexpFieldStreamHandler(pattern, handler)
}

// onFieldGlob 使用 Glob 通配符匹配字段名，为匹配的字段注册字符级流式处理器。
// 参数:
//   - pattern: 用于匹配字段名的 Glob 通配符
//   - handler: 字段流处理器，参数为键名、字段值的数据流 reader 与父路径
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
//
//	jsonstream.Extract(`{"config_host": "localhost", "config_port": 8080}`,
//	    jsonstream.onFieldGlob("config_*", func(key, reader, parents) {
//	        data = io.ReadAll(reader)~
//	        println(key, string(data))
//	    }),
//	)
//
// ```
func _jsonStreamOnFieldGlob(pattern string, handler func(key string, reader io.Reader, parents []string)) jsonextractor.CallbackOption {
	return jsonextractor.WithRegisterGlobFieldStreamHandler(pattern, handler)
}

// onFinished 注册解析完成回调，当数据流被完整解析且没有错误时触发。
// 参数:
//   - callback: 解析完成时调用的无参回调
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
// jsonstream.Extract(`{"a": 1}`, jsonstream.onFinished(func() {
//
//	println("stream finished")
//
// }))
// ```
func _jsonStreamOnFinished(callback func()) jsonextractor.CallbackOption {
	return jsonextractor.WithStreamFinishedCallback(callback)
}

// onError 注册解析错误回调，当解析过程中发生错误时触发，回调参数为错误对象。
// 参数:
//   - callback: 发生错误时调用的回调，参数为错误对象
//
// 返回值:
//   - 可传给 Extract/ExtractFromReader 的回调选项
//
// Example:
// ```
// jsonstream.ExtractFromReader(reader, jsonstream.onError(func(err) {
//
//	log.Errorf("stream error: %v", err)
//
// }))
// ```
func _jsonStreamOnError(callback func(err error)) jsonextractor.CallbackOption {
	return jsonextractor.WithStreamErrorCallback(callback)
}
