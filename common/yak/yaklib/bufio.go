package yaklib

import (
	"bufio"
	"bytes"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
	"io"
	"reflect"

	"github.com/yaklang/yaklang/common/utils"
)

// NewBuffer 创建一个新的 Buffer 结构体引用，其帮助我们处理字符串（导出名为 bufio.NewBuffer）
// Buffer 同时实现了 Reader 和 Writer 接口
// 常用方法：Bytes, String, Read, Write, WriteString, WriteByte, Reset
//
// 参数:
//   - b: 可选的初始内容字节
//
// 返回值:
//   - 新建的 Buffer 对象
//
// Example:
// ```
// buffer = bufio.NewBuffer()
// buffer.WriteString("hello yak")
// println(buffer.String())   // OUT: hello yak
// assert buffer.String() == "hello yak", "NewBuffer should hold written content"
// ```
func _newBuffer(b ...[]byte) *bytes.Buffer {
	buffer := &bytes.Buffer{}
	if len(b) > 0 {
		buffer.Write(b[0])
	}
	return buffer
}

// NewReader 根据传入的 Reader 创建一个新的带缓冲 Reader（导出名为 bufio.NewReader）
// 常用方法：Read, ReadByte, ReadBytes, ReadLine, ReadString, Reset
//
// 参数:
//   - raw: 可选的底层 io.Reader；不传则使用空缓冲
//
// 返回值:
//   - 带缓冲的 Reader 对象
//   - 错误信息（传入类型非 io.Reader 时返回）
//
// Example:
// ```
// reader = bufio.NewReader(bufio.NewBuffer("line1\nline2"))~
// first = reader.ReadString('\n')~
// println(first)   // OUT: line1
// assert first == "line1\n", "NewReader ReadString should read up to and including the delimiter"
// ```
func _newReader(raw ...interface{}) (*bufio.Reader, error) {
	var i any
	if len(raw) > 0 {
		i = raw[0].(io.Reader)
	} else {
		i = &bytes.Buffer{}
	}
	if rd, ok := i.(io.Reader); ok {
		return bufio.NewReader(rd), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

// NewReaderSize 根据传入的 Reader 创建一个指定缓冲区大小的带缓冲 Reader（导出名为 bufio.NewReaderSize）
// 常用方法：Read, ReadByte, ReadBytes, ReadLine, ReadString, Reset
//
// 参数:
//   - i: 底层 io.Reader
//   - size: 缓冲区大小（字节）
//
// 返回值:
//   - 带缓冲的 Reader 对象
//   - 错误信息（传入类型非 io.Reader 时返回）
//
// Example:
// ```
// reader = bufio.NewReaderSize(bufio.NewBuffer("abcdef"), 1024)~
// part = reader.ReadString('c')~
// println(part)   // OUT: abc
// assert part == "abc", "NewReaderSize ReadString should read up to delimiter c"
// ```
func _newReaderSize(i interface{}, size int) (*bufio.Reader, error) {
	if rd, ok := i.(io.Reader); ok {
		return bufio.NewReaderSize(rd, size), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

// NewWriter 根据传入的 Writer 创建一个新的带缓冲 Writer（导出名为 bufio.NewWriter）
// 写入会先进入缓冲区，需调用 Flush 才会真正写到底层 Writer
// 常用方法：Write, WriteByte, WriteString, Reset, Flush
//
// 参数:
//   - raw: 可选的底层 io.Writer；不传则写入空缓冲
//
// 返回值:
//   - 带缓冲的 Writer 对象
//   - 错误信息（传入类型非 io.Writer 时返回）
//
// Example:
// ```
// sink = bufio.NewBuffer()
// writer = bufio.NewWriter(sink)~
// writer.WriteString("hello yak")
// writer.Flush()
// println(sink.String())   // OUT: hello yak
// assert sink.String() == "hello yak", "NewWriter should flush buffered data to the sink"
// ```
func _newWriter(raw ...interface{}) (*bufio.Writer, error) {
	var i any
	if len(raw) > 0 {
		i = raw[0].(io.Writer)
	} else {
		i = &bytes.Buffer{}
	}
	if wd, ok := i.(io.Writer); ok {
		return bufio.NewWriter(wd), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

// NewWriterSize 根据传入的 Writer 创建一个指定缓冲区大小的带缓冲 Writer（导出名为 bufio.NewWriterSize）
// 常用方法：Write, WriteByte, WriteString, Reset, Flush
//
// 参数:
//   - i: 底层 io.Writer
//   - size: 缓冲区大小（字节）
//
// 返回值:
//   - 带缓冲的 Writer 对象
//   - 错误信息（传入类型非 io.Writer 时返回）
//
// Example:
// ```
// sink = bufio.NewBuffer()
// writer = bufio.NewWriterSize(sink, 1024)~
// writer.WriteString("hello yak")
// writer.Flush()
// println(sink.String())   // OUT: hello yak
// assert sink.String() == "hello yak", "NewWriterSize should flush buffered data to the sink"
// ```
func _newWriterSize(i interface{}, size int) (*bufio.Writer, error) {
	if wd, ok := i.(io.Writer); ok {
		return bufio.NewWriterSize(wd, size), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

// NewReadWriter 根据传入的 Reader 和 Writer 创建一个带缓冲的 ReadWriter（导出名为 bufio.NewReadWriter）
// ReadWriter 可同时调用带缓冲 Reader 与 Writer 的方法
//
// 参数:
//   - i: 底层 io.Reader
//   - i2: 底层 io.Writer
//
// 返回值:
//   - 带缓冲的 ReadWriter 对象
//   - 错误信息（任一参数类型不符时返回）
//
// Example:
// ```
// rw = bufio.NewReadWriter(bufio.NewBuffer("input"), bufio.NewBuffer())~
// line = rw.ReadString('t')~
// println(line)   // OUT: input
// assert line == "input", "NewReadWriter should read from the underlying reader"
// ```
func _newReadWriter(i, i2 interface{}) (*bufio.ReadWriter, error) {
	var (
		rd  *bufio.Reader
		wd  *bufio.Writer
		err error
	)

	rd, err = _newReader(i)
	if err != nil {
		return nil, err
	}
	wd, err = _newWriter(i2)
	if err != nil {
		return nil, err
	}

	return bufio.NewReadWriter(rd, wd), nil
}

// NewScanner 根据传入的 Reader 创建一个 Scanner（导出名为 bufio.NewScanner）
// Scanner 默认按行切分输入，常用方法：Scan, Text, Err, Split, SplitFunc
//
// 参数:
//   - i: 底层 io.Reader
//
// 返回值:
//   - Scanner 对象
//   - 错误信息（传入类型非 io.Reader 时返回）
//
// Example:
// ```
// scanner = bufio.NewScanner(bufio.NewBuffer("a\nb\nc"))~
// count = 0
// for scanner.Scan() { count++ }
// println(count)   // OUT: 3
// assert count == 3, "NewScanner should iterate over 3 lines"
// ```
func _newScanner(i interface{}) (*bufio.Scanner, error) {
	if rd, ok := i.(io.Reader); ok {
		return bufio.NewScanner(rd), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

// NewPipe 创建一个内存管道，返回配对的 PipeReader 与 PipeWriter（导出名为 bufio.NewPipe）
// 写入端写入的数据可从读取端读出，常用于在协程间传递流式数据
//
// 返回值:
//   - 管道读取端 PipeReader
//   - 管道写入端 PipeWriter
//
// Example:
// ```
// r, w = bufio.NewPipe()
// go func() { w.Write("Hello World"); w.Close() }()
// data = io.ReadAll(r)~
// println(string(data))   // OUT: Hello World
// assert string(data) == "Hello World", "NewPipe should transfer data from writer to reader"
// ```
func _newPipe() (*bufpipe.PipeReader, *bufpipe.PipeWriter) {
	r, w := bufpipe.NewPipe()
	return r, w
}

var BufioExport = map[string]interface{}{
	"NewBuffer":     _newBuffer,
	"NewReader":     _newReader,
	"NewReaderSize": _newReaderSize,
	"NewWriter":     _newWriter,
	"NewWriterSize": _newWriterSize,
	"NewReadWriter": _newReadWriter,
	"NewScanner":    _newScanner,
	"NewPipe":       _newPipe,
}
