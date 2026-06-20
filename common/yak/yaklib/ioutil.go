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

// ReadAll 读取 Reader 中的所有字节，直到 EOF（导出名为 io.ReadAll）
//
// 参数:
//   - r: 数据来源 Reader
//
// 返回值:
//   - 读取到的全部字节
//   - 错误信息（读取出错时返回，正常读到 EOF 不算错误）
//
// Example:
// ```
// data = io.ReadAll(bufio.NewBuffer("hello yak"))~
// println(string(data))   // OUT: hello yak
// assert string(data) == "hello yak", "ReadAll should read all bytes from the reader"
// ```
func _readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// ReadFile 读取指定文件的全部内容（导出名为 io.ReadFile）
//
// 参数:
//   - path: 文件路径
//
// 返回值:
//   - 文件内容字节
//   - 错误信息（文件不存在或读取失败时返回）
//
// Example:
// ```
// fp = file.Join(os.TempDir(), "io_readfile_demo.txt")
// file.Save(fp, "hello yak")~
// data = io.ReadFile(fp)~
// println(string(data))   // OUT: hello yak
// assert string(data) == "hello yak", "ReadFile should return the whole file content"
// file.Remove(fp)
// ```
func _readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ReadEvery1s 每秒读取一次 Reader，并把读到的数据交给回调函数处理（导出名为 io.ReadEvery1s）
// 直到读取到 EOF、上下文取消、或回调返回 false 为止，常用于持续消费子进程/连接的流式输出
//
// 参数:
//   - c: 控制生命周期的上下文，取消后停止读取
//   - reader: 数据来源 Reader
//   - f: 回调函数，接收本次读到的数据；返回 false 表示停止
//
// Example:
// ```
// collected = bufio.NewBuffer()
// ctx, cancel = context.WithTimeout(context.Background(), time.ParseDuration("3s")~)
// r, w = io.Pipe()
// go func() { w.WriteString("tick"); w.Close() }()
// io.ReadEvery1s(ctx, r, func(data) { collected.Write(data); cancel(); return false })
// println(collected.String())   // OUT: tick
// assert str.Contains(collected.String(), "tick"), "ReadEvery1s should deliver data to the callback"
// ```
func _readEvery1s(c context.Context, reader io.Reader, f func([]byte) bool) {
	utils.ReadWithContextTickCallback(c, reader, f, 1*time.Second)
}

// LimitReader 返回一个最多读取 n 个字节后即返回 EOF 的 Reader（导出名为 io.LimitReader）
//
// 参数:
//   - r: 底层 Reader
//   - n: 最多读取的字节数
//
// 返回值:
//   - 受限的 Reader
//
// Example:
// ```
// lr = io.LimitReader(bufio.NewBuffer("abcdefgh"), 3)
// data = io.ReadAll(lr)~
// println(string(data))   // OUT: abc
// assert string(data) == "abc", "LimitReader should stop after n bytes"
// ```
func _limitReader(r io.Reader, n int64) io.Reader {
	return io.LimitReader(r, n)
}

// TeeReader 返回一个 Reader，从 r 读取的同时把数据写入 w（导出名为 io.TeeReader）
// 常用于在读取数据流的同时保留一份副本
//
// 参数:
//   - r: 底层 Reader
//   - w: 副本写入目标 Writer
//
// 返回值:
//   - 带旁路写入的 Reader
//
// Example:
// ```
// sink = bufio.NewBuffer()
// tr = io.TeeReader(bufio.NewBuffer("teedata"), sink)
// io.ReadAll(tr)~
// println(sink.String())   // OUT: teedata
// assert sink.String() == "teedata", "TeeReader should mirror read bytes into the writer"
// ```
func _teeReader(r io.Reader, w io.Writer) io.Reader {
	return io.TeeReader(r, w)
}

// MultiReader 将多个 Reader 串联成一个 Reader，按顺序逐个读取（导出名为 io.MultiReader）
//
// 参数:
//   - readers: 一个或多个 Reader，按传入顺序读取
//
// 返回值:
//   - 串联后的 Reader
//
// Example:
// ```
// mr = io.MultiReader(bufio.NewBuffer("foo"), bufio.NewBuffer("bar"))
// data = io.ReadAll(mr)~
// println(string(data))   // OUT: foobar
// assert string(data) == "foobar", "MultiReader should read readers in order"
// ```
func _multiReader(readers ...io.Reader) io.Reader {
	return io.MultiReader(readers...)
}

// NopCloser 将一个 Reader 包装为 ReadCloser，其 Close 方法为空操作（导出名为 io.NopCloser）
// 常用于满足需要 ReadCloser 的接口，但底层数据源不需要真正关闭的场景
//
// 参数:
//   - r: 底层 Reader
//
// 返回值:
//   - 带空 Close 的 ReadCloser
//
// Example:
// ```
// nc = io.NopCloser(bufio.NewBuffer("nopdata"))
// nc.Close()
// data = io.ReadAll(nc)~
// println(string(data))   // OUT: nopdata
// assert string(data) == "nopdata", "NopCloser should stay readable after Close"
// ```
func _nopCloser(r io.Reader) io.ReadCloser {
	return io.NopCloser(r)
}

// Pipe 创建一个内存管道，返回配对的读取端与写入端（导出名为 io.Pipe）
// 写入端写入的数据可从读取端读出，常用于在协程间传递流式数据
//
// 返回值:
//   - 管道读取端
//   - 管道写入端
//
// Example:
// ```
// r, w = io.Pipe()
// go func() { w.WriteString("piped"); w.Close() }()
// data = io.ReadAll(r)~
// println(string(data))   // OUT: piped
// assert string(data) == "piped", "Pipe should transfer data from writer to reader"
// ```
func _ioPipe() (*bufpipe.PipeReader, *bufpipe.PipeWriter) {
	return bufpipe.NewPipe()
}

// Copy 将 reader 中的数据全部拷贝到 writer，直到 EOF 或出错（导出名为 io.Copy）
//
// 参数:
//   - writer: 目标 Writer
//   - reader: 数据来源 Reader
//
// 返回值:
//   - 实际拷贝的字节数
//   - 错误信息（拷贝出错时返回）
//
// Example:
// ```
// sink = bufio.NewBuffer()
// n = io.Copy(sink, bufio.NewBuffer("copydata"))~
// println(sink.String())   // OUT: copydata
// assert sink.String() == "copydata", "Copy should copy all bytes to the writer"
// assert n == 8, "Copy should return the number of bytes copied"
// ```
func _copy(writer io.Writer, reader io.Reader) (written int64, err error) {
	return io.Copy(writer, reader)
}

// CopyN 将 reader 中最多 n 个字节拷贝到 writer，直到 EOF 或拷满 n 字节（导出名为 io.CopyN）
//
// 参数:
//   - writer: 目标 Writer
//   - reader: 数据来源 Reader
//   - n: 最多拷贝的字节数
//
// 返回值:
//   - 实际拷贝的字节数
//   - 错误信息（提前 EOF 或出错时返回）
//
// Example:
// ```
// sink = bufio.NewBuffer()
// io.CopyN(sink, bufio.NewBuffer("abcdef"), 3)~
// println(sink.String())   // OUT: abc
// assert sink.String() == "abc", "CopyN should copy at most n bytes"
// ```
func _copyN(writer io.Writer, reader io.Reader, n int64) (written int64, err error) {
	return io.CopyN(writer, reader, n)
}

// WriteString 将字符串写入 writer（导出名为 io.WriteString）
//
// 参数:
//   - writer: 目标 Writer
//   - s: 要写入的字符串
//
// 返回值:
//   - 实际写入的字节数
//   - 错误信息（写入出错时返回）
//
// Example:
// ```
// sink = bufio.NewBuffer()
// n = io.WriteString(sink, "hello yak")~
// println(sink.String())   // OUT: hello yak
// assert sink.String() == "hello yak", "WriteString should write the string to the writer"
// assert n == 9, "WriteString should return the number of bytes written"
// ```
func _writeString(writer io.Writer, s string) (n int, err error) {
	return io.WriteString(writer, s)
}

// ReadStable 从 reader 稳定读取数据，在指定时间内无新数据或读到 EOF 时返回（导出名为 io.ReadStable）
// 适合读取不会主动关闭、但会间歇产出数据的流（如某些网络连接）
//
// 参数:
//   - reader: 数据来源 Reader
//   - float: 稳定等待的秒数（支持小数）
//
// 返回值:
//   - 读取到的数据字节
//
// Example:
// ```
// data = io.ReadStable(bufio.NewBuffer("stable"), 1)
// println(string(data))   // OUT: stable
// assert string(data) == "stable", "ReadStable should read available data within the window"
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
