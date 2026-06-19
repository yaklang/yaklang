package aireducer

import (
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils"
	"io"
)

var Exports = map[string]any{
	"NewReducerFromReader": NewReducerFromReader,
	"NewReducerFromFile":   NewReducerFromFile,
	"NewReducerFromString": NewReducerFromString,
	"File":                 _reducerFile,
	"String":               _reducerString,
	"Reader":               _reducerReader,

	"reducerCallback":            WithReducerCallback,
	"callback":                   WithReducerCallback,
	"timeTriggerInterval":        WithTimeTriggerInterval,
	"timeTriggerIntervalSeconds": WithTimeTriggerIntervalSeconds,
	"context":                    WithContext,
	"memory":                     WithMemory,
	"separator":                  WithSeparatorTrigger,
	"separatorAsBoundary":        WithSeparatorAsBoundary,
	"chunkSize":                  WithChunkSize,
	"lines":                      WithLines,
	"lineNumber":                 WithEnableLineNumber,
}

func ReducerFast(i any, callback func(chunk chunkmaker.Chunk), options ...Option) error {
	switch v := i.(type) {
	case io.Reader:
		return _reducerReader(v, callback, options...)
	default:
		return _reducerString(utils.InterfaceToString(i), callback, options...)
	}
}

// _reducerReader 从 io.Reader 读取数据并按配置切分为 chunk，对每个 chunk 调用回调（导出名为 aireducer.Reader）
// 参数:
//   - i: 数据来源 reader
//   - callback: 每生成一个 chunk 触发的回调，参数为 chunk 对象
//   - options: 切分可选项，如 aireducer.chunkSize、aireducer.separator、aireducer.lines 等
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// count = 0
// reader = str.NewReader("aaaaabbbbbccccc")
// aireducer.Reader(reader, func(chunk) { count++ }, aireducer.chunkSize(5))~
// println(count)   // OUT: 3
// ```
func _reducerReader(i io.Reader, callback func(chunk chunkmaker.Chunk), options ...Option) error {
	options = append(options, WithSimpleCallback(callback))
	reducer, err := NewReducerFromReader(i, options...)
	if err != nil {
		return err
	}

	if reducer.config.callback == nil {
		return utils.Error("reducer callback is nil")
	}
	return reducer.Run()
}

// _reducerString 将字符串按配置切分为 chunk，对每个 chunk 调用回调（导出名为 aireducer.String）
// 参数:
//   - s: 输入字符串
//   - callback: 每生成一个 chunk 触发的回调，参数为 chunk 对象
//   - options: 切分可选项，如 aireducer.chunkSize、aireducer.separator、aireducer.lines 等
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// count = 0
// aireducer.String("aaaaabbbbbccccc", func(chunk) { count++ }, aireducer.chunkSize(5))~
// println(count)   // OUT: 3
// ```
func _reducerString(s string, callback func(chunk chunkmaker.Chunk), options ...Option) error {
	options = append(options, WithSimpleCallback(callback))
	r, err := NewReducerFromString(s, options...)
	if err != nil {
		return err
	}
	if r.config.callback == nil {
		return utils.Error("reducer callback is nil")
	}
	return r.Run()
}

// _reducerFile 读取文件内容并按配置切分为 chunk，对每个 chunk 调用回调（导出名为 aireducer.File）
// 参数:
//   - filename: 文件路径
//   - callback: 每生成一个 chunk 触发的回调，参数为 chunk 对象
//   - options: 切分可选项，如 aireducer.chunkSize、aireducer.lines、aireducer.lineNumber 等
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 按 1024 字节切分文件并逐块处理（示意性示例，需替换为真实文件路径）
//
//	aireducer.File("/tmp/example.txt", func(chunk) {
//	    println(string(chunk.Data()))
//	}, aireducer.chunkSize(1024))~
//
// ```
func _reducerFile(filename string, callback func(chunk chunkmaker.Chunk), options ...Option) error {
	options = append(options, WithSimpleCallback(callback))
	r, err := NewReducerFromFile(filename, options...)
	if err != nil {
		return err
	}
	if r.config.callback == nil {
		return utils.Error("reducer callback is nil")
	}
	return r.Run()
}

//func _reducer(i any, cb func(chunk chunkmaker.Chunk), options ...Option) error {
//	options = append(options, WithSimpleCallback(cb))
//
//	var r io.Reader
//	switch v := i.(type) {
//	case io.Reader:
//		r = v
//	default:
//		r = bytes.NewReader(utils.InterfaceToBytes(i))
//	}
//
//	if r == nil {
//		return utils.Errorf("input is nil")
//	}
//	reducer, err := NewReducerFromReader(r, options...)
//	if err != nil {
//		return err
//	}
//
//	if reducer.config.callback == nil {
//		return utils.Errorf("reducer callback is nil")
//	}
//	return reducer.Run()
//}
//
//func _reducerFileLine(filename string, callback func(chunk chunkmaker.Chunk), options ...Option) error {
//	reader, err := os.OpenFile(filename, os.O_RDONLY, 0644)
//	if err != nil {
//		return err
//	}
//	pr, pw, err := os.Pipe()
//	if err != nil {
//		return utils.Errorf("create pipe failed: %v", err)
//	}
//
//	go func() {
//		defer pw.Close()
//		defer reader.Close()
//
//		lineReader := utils.PrefixLinesWithLineNumbersReader(reader)
//		io.Copy(pw, lineReader)
//	}()
//
//	return _reducer(pr, callback, options...)
//}
//
//func _reducerFile(filename string, i func(chunk chunkmaker.Chunk), options ...Option) error {
//	options = append(options, WithSimpleCallback(i))
//	r, err := NewReducerFromFile(filename, options...)
//	if err != nil {
//		return err
//	}
//	if r.config.callback == nil {
//		return nil
//	}
//	return r.Run()
//}
