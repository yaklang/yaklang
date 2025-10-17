package yaklib

import (
	"bufio"
	"bytes"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
	"io"
	"reflect"

	"github.com/yaklang/yaklang/common/utils"
)

// NewBuffer 创建一个新的 Buffer 结构体引用，其帮助我们处理字符串
// Buffer 也实现了 Reader 和 Writer 接口
// 常用的 Buffer 方法有：Bytes, String, Read, Write, WriteString, WriteByte, Reset
// Example:
// ```
// buffer = bufio.NewBuffer() // 或者你也可以使用 io.NewBuffer("hello yak") 来初始化一个 Buffer
// buffer.WriteString("hello yak")
// data, err = io.ReadAll(buffer) // data = b"hello yak", err = nil
// ```
func _newBuffer(b ...[]byte) *bytes.Buffer {
	buffer := &bytes.Buffer{}
	if len(b) > 0 {
		buffer.Write(b[0])
	}
	return buffer
}

// NewReader 根据传入的 Reader 创建一个新的 BufioReader 结构体引用
// 常用的 BufioReader 方法有：Read, ReadByte, ReadBytes, ReadLine, ReadString, Reset
// Example:
// ```
// reader = bufio.NewReader(os.Stdin)
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

// NewReaderSize 根据传入的 Reader 创建一个新的 BufioReader 结构体引用，其的缓存大小为 size
// 常用的 BufioReader 方法有：Read, ReadByte, ReadBytes, ReadLine, ReadString, Reset
// Example:
// ```
// reader = bufio.NewReaderSize(os.Stdin, 1024)
// ```
func _newReaderSize(i interface{}, size int) (*bufio.Reader, error) {
	if rd, ok := i.(io.Reader); ok {
		return bufio.NewReaderSize(rd, size), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

// NewWriter 根据传入的 Writer 创建一个新的 BufioWriter 结构体引用
// 常用的 BufioWriter 方法有：Write, WriteByte, WriteString, Reset, Flush
// Example:
// ```
// writer, err = bufio.NewWriter(os.Stdout)
// writer.WriteString("hello yak")
// writer.Flush()
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

// NewWriterSize 根据传入的 Writer 创建一个新的 BufioWriter 结构体引用，其的缓存大小为 size
// 常用的 BufioWriter 方法有：Write, WriteByte, WriteString, Reset, Flush
// Example:
// ```
// writer, err = bufio.NewWriterSize(os.Stdout, 1024)
// writer.WriteString("hello yak")
// writer.Flush()
// ```
func _newWriterSize(i interface{}, size int) (*bufio.Writer, error) {
	if wd, ok := i.(io.Writer); ok {
		return bufio.NewWriterSize(wd, size), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

// NewReadWriter 根据传入的 Reader 和 Writer 创建一个新的 BufioReadWriter 结构体引用
// BufioReadWriter 可以同时调用 BufioReader 和 BufioWriter 的方法
// Example:
// ```
// rw, err = bufio.NewReadWriter(os.Stdin, os.Stdout)
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

// NewScanner 根据传入的 Reader 创建一个新的 Scanner 结构体引用
// 常用的 Scanner 方法有：Scan, Text, Err, Split, SplitFunc
// Example:
// ```
// buf = bufio.NewBuffer("hello yak\nhello yakit")
// scanner, err = bufio.NewScanner(buf)
// for scanner.Scan() {
// println(scanner.Text())
// }
// ```
func _newScanner(i interface{}) (*bufio.Scanner, error) {
	if rd, ok := i.(io.Reader); ok {
		return bufio.NewScanner(rd), nil
	} else {
		return nil, utils.Errorf("not support type: %v", reflect.TypeOf(i))
	}
}

// bufio.NewPipe 创建一个新的管道，返回一个 PipeReader 和 PipeWriter
// Example:
// ```
// r, w = bufio.NewPipe()
//
//	go func{
//	    w.Write("Hello World");
//	    w.Close()
//	}
//
// data = io.ReadAll(r)~
// println(string(data))
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
	"NewPipe":       bufpipe.NewPipe,
}
