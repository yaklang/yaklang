package yaklib

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// ReadAll 读取 Reader 中的所有字节，返回读取到的数据和错误
// Example:
// ```
// data, err = ioutil.ReadAll(reader)
// ```
func _readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// ReadFile 读取指定文件中的所有内容，返回读取到的数据和错误
// Example:
// ```
// // 假设存在文件 /tmp/test.txt，内容为 "hello yak"
// data, err = ioutil.ReadFile("/tmp/test.txt") // data = b"hello yak", err = nil
// ```
func _readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ReadEvery1s 每秒读取 Reader 一次，直到读取到 EOF 或者回调函数返回 false
// Example:
// ```
// r, w = io.Pipe() // 创建一个管道，返回一个读取端和一个写入端
//
//	go func{
//	    for {
//		       w.WriteString("hello yak\n")
//		       time.Sleep(1)
//		   }
//	}
//
//	io.ReadEvery1s(context.New(), r, func(data) {
//	    println(string(data))
//		   return true
//	})
//
// ```
func _readEvery1s(c context.Context, reader io.Reader, f func([]byte) bool) {
	utils.ReadWithContextTickCallback(c, reader, f, 1*time.Second)
}

// LimitReader 返回一个 Reader，该 Reader 从 r 中读取字节，但在读取 n 个字节后就会返回 EOF
// Example:
// ```
// lr = io.LimitReader(reader, 1024)
// ```
func _limitReader(r io.Reader, n int64) io.Reader {
	return io.LimitReader(r, n)
}

// TeeReader 返回一个 Reader，该 Reader 从 r 中读取字节，并将读取到的字节写入 w 中
// 该 Reader 通常用于保存已经读取的数据副本
// Example:
// ```
// tr = io.TeeReader(reader, buf)
// io.ReadAll(tr)
// // 现在 buf 中也保存了 reader 中的读到的所有数据
// ```
func _teeReader(r io.Reader, w io.Writer) io.Reader {
	return io.TeeReader(r, w)
}

// MultiReader 返回一个 Reader，该 Reader 从多个 Reader 中读取数据
// Example:
// ```
// mr = io.MultiReader(reader1, reader2) // 读取 mr 即按照顺序读取 reader1 和 reader2 中的数据
// io.ReadAll(mr)
// ```
func _multiReader(readers ...io.Reader) io.Reader {
	return io.MultiReader(readers...)
}

// NopCloser 返回一个 ReadCloser，该 ReadCloser 从 r 中读取数据，并实现了一个空的 Close 方法
// Example:
// ```
// r = io.NopCloser(reader)
// r.Close() // 什么都不做
// ```
func _nopCloser(r io.Reader) io.ReadCloser {
	return io.NopCloser(r)
}

// Pipe 创建一个管道，返回一个读取端和一个写入端以
// Example:
// ```
// r, w = io.Pipe()
//
//	go func {
//	    w.WriteString("hello yak")
//	    w.Close()
//	}
//
// bytes, err = io.ReadAll(r)
// die(err)
// dump(bytes)
// ```
func _ioPipe() (*bufpipe.PipeReader, *bufpipe.PipeWriter) {
	return bufpipe.NewPipe()
}

// Copy 将 reader 中的数据拷贝到 writer 中，直到读取到 EOF 或者发生错误，返回拷贝的字节数和错误
// Example:
// ```
// n, err = io.Copy(writer, reader)
// ```
func _copy(writer io.Writer, reader io.Reader) (written int64, err error) {
	return io.Copy(writer, reader)
}

// CopyN 将 reader 中的数据拷贝到 writer 中，直到读取到 EOF 或者拷贝了 n 个字节，返回拷贝的字节数和错误
// Example:
// ```
// n, err = io.CopyN(writer, reader, 1024)
// ```
func _copyN(writer io.Writer, reader io.Reader, n int64) (written int64, err error) {
	return io.CopyN(writer, reader, n)
}

// WriteString 将字符串 s 写入 writer 中，返回写入的字节数和错误
// Example:
// ```
// n, err = io.WriteString(writer, "hello yak")
// ```
func _writeString(writer io.Writer, s string) (n int, err error) {
	return io.WriteString(writer, s)
}

// ReadStable 从 reader 中稳定地读取数据，直到读取到 EOF 或者超时，返回读取到的数据
// Example:
// ```
// data = io.ReadStable(reader, 60)
// ```
func _readStable(reader io.Reader, float float64) []byte {
	return utils.StableReader(reader, utils.FloatSecondDuration(float), 10*1024*1024)
}

// Discard 是一个 writer，它将所有写入的数据都丢弃掉
var Discard = ioutil.Discard

// EOF 是一个错误，表示读取到了 EOF
var EOF = io.EOF

var IoExports = map[string]interface{}{
	"ReadAll":     _readAll,
	"ReadFile":    _readFile,
	"ReadEvery1s": _readEvery1s,

	// 继承自 io
	"LimitReader": _limitReader,
	"TeeReader":   _teeReader,
	"MultiReader": _multiReader,
	"NopCloser":   _nopCloser,
	"Pipe":        _ioPipe,
	"Copy":        _copy,
	"CopyN":       _copyN,
	//"NewSectionReader": io.NewSectionReader,
	//"ReadFull":         io.ReadFull,
	//"ReadAtLeast":      io.ReadAtLeast,
	"WriteString": _writeString,
	"ReadStable":  _readStable,

	"Discard": Discard,
	"EOF":     EOF,
}
