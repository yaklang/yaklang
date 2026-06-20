package utils

import (
	"bytes"
	"io"

	"github.com/klauspost/compress/zstd"
)

// ZstdCompress 使用 zstd 以最高压缩比压缩数据，返回压缩后的字节与错误
// 相比 gzip，zstd 在同等内容下压缩比明显更高（实测嵌入式文档体积约减少 35%~40%），
// 解压速度也很快，适合内嵌大体积只读资源（如 doc.gob）。
// 参数:
//   - i: 待压缩的数据，支持字符串、字节切片或 io.Reader
//
// 返回值:
//   - 压缩后的字节切片（zstd 帧）
//   - 错误信息
func ZstdCompress(i interface{}) ([]byte, error) {
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return nil, err
	}
	switch ret := i.(type) {
	case io.Reader:
		if _, err := io.Copy(w, ret); err != nil && err != io.EOF {
			w.Close()
			return buf.Bytes(), err
		}
		if err := w.Close(); err != nil {
			return buf.Bytes(), err
		}
		return buf.Bytes(), nil
	default:
		if _, err := w.Write(InterfaceToBytes(ret)); err != nil {
			w.Close()
			return nil, err
		}
		if err := w.Close(); err != nil {
			return buf.Bytes(), err
		}
		return buf.Bytes(), nil
	}
}

// ZstdDeCompress 解压 zstd 数据，返回解压后的字节与错误
// 参数:
//   - ret: 经过 zstd 压缩的字节切片
//
// 返回值:
//   - 解压还原后的字节切片
//   - 错误信息
func ZstdDeCompress(ret []byte) ([]byte, error) {
	reader, err := zstd.NewReader(bytes.NewReader(ret))
	if err != nil {
		return nil, Errorf("create zstd reader failed: %s", err)
	}
	defer reader.Close()
	var bufBytes bytes.Buffer
	if _, err = io.Copy(&bufBytes, reader); err != nil {
		return bufBytes.Bytes(), Errorf("unzstd failed: %s", err)
	}
	return bufBytes.Bytes(), nil
}
