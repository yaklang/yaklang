package utils

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
)

// Compress 使用 gzip 压缩数据，返回压缩后的字节与错误
// 参数:
//   - i: 待压缩的数据，支持字符串、字节切片或 io.Reader
//
// 返回值:
//   - 压缩后的字节切片（带 gzip 魔数头）
//   - 错误信息
//
// Example:
// ```
// // 压缩后再解压应还原原始数据（round-trip）
// compressed = gzip.Compress("hello yaklang")~
// assert gzip.IsGzip(compressed), "compressed output should have gzip magic header"
// raw = gzip.Decompress(compressed)~
// assert string(raw) == "hello yaklang", "gzip compress then decompress should round-trip"
// println("gzip round-trip example passed")
// ```
func GzipCompress(i interface{}) ([]byte, error) {
	var buf bytes.Buffer
	var w = gzip.NewWriter(&buf)
	switch ret := i.(type) {
	case io.Reader:
		_, err := io.Copy(w, ret)
		w.Close()
		if err != nil && err != io.EOF {
			return buf.Bytes(), err
		}
		return buf.Bytes(), nil
	default:
		_, err := w.Write(InterfaceToBytes(ret))
		if err != nil {
			return nil, err
		}
		w.Flush()
		w.Close()
		return buf.Bytes(), nil
	}
}

func ZlibCompress(i interface{}) ([]byte, error) {
	var buf bytes.Buffer
	var w = zlib.NewWriter(&buf)
	switch ret := i.(type) {
	case io.Reader:
		_, err := io.Copy(w, ret)
		w.Close()
		if err != nil && err != io.EOF {
			return buf.Bytes(), err
		}
		return buf.Bytes(), nil
	default:
		_, err := w.Write(InterfaceToBytes(ret))
		if err != nil {
			return nil, err
		}
		w.Flush()
		w.Close()
		return buf.Bytes(), nil
	}
}

// Decompress 解压 gzip 数据，返回解压后的字节与错误
// 参数:
//   - ret: 经过 gzip 压缩的字节切片
//
// 返回值:
//   - 解压还原后的字节切片
//   - 错误信息
//
// Example:
// ```
// // 压缩再解压应还原原始数据（round-trip）
// compressed = gzip.Compress("hello yaklang")~
// raw = gzip.Decompress(compressed)~
// assert string(raw) == "hello yaklang", "gzip decompress should restore original data"
// println("gzip decompress example passed")
// ```
func GzipDeCompress(ret []byte) ([]byte, error) {
	var reader *gzip.Reader
	var err error
	reader, err = gzip.NewReader(bytes.NewBuffer(ret))
	if err != nil {
		return nil, Errorf("create gzip reader failed: %s", err)
	}
	var bufBytes bytes.Buffer
	_, err = io.Copy(&bufBytes, reader)
	reader.Close()
	if err != nil {
		return bufBytes.Bytes(), Errorf("ungzip failed: %s", err)
	}
	return bufBytes.Bytes(), nil
}

func ZlibDeCompress(ret []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewBuffer(ret))
	if err != nil {
		return nil, Errorf("create gzip reader failed: %s", err)
	}
	var bufBytes bytes.Buffer
	_, err = io.Copy(&bufBytes, reader)
	reader.Close()
	if err != nil {
		return bufBytes.Bytes(), Errorf("ungzip failed: %s", err)
	}
	return bufBytes.Bytes(), nil
}

func IsGzipBytes(i interface{}) bool {
	switch ret := i.(type) {
	case io.Reader:
		_, err := gzip.NewReader(ret)
		if err != nil {
			return false
		}
		return true
	default:
		var buf = bytes.NewBuffer(InterfaceToBytes(i))
		return IsGzipBytes(buf)
	}
}
