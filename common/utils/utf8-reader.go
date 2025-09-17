package utils

import (
	"io"
	"unicode/utf8"
)

type utf8Reader struct {
	r      io.Reader
	buffer []byte // 内部缓冲区，存储未完整读取的字节
}

func (r *utf8Reader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	// 特殊情况：如果缓冲区长度为1，UTF8Reader失效，直接透传
	if len(p) == 1 {
		// 先从内部缓冲区读取
		if len(r.buffer) > 0 {
			p[0] = r.buffer[0]
			r.buffer = r.buffer[1:]
			return 1, nil
		}
		return r.r.Read(p)
	}

	// 如果有内部缓冲区数据，先处理它
	totalData := make([]byte, 0, len(r.buffer)+len(p))
	totalData = append(totalData, r.buffer...)

	// 从底层reader读取数据，但要留出空间给可能的不完整字符
	tempBuf := make([]byte, len(p))
	readCount, err := r.r.Read(tempBuf)
	if readCount > 0 {
		totalData = append(totalData, tempBuf[:readCount]...)
	}

	// 清空缓冲区
	r.buffer = r.buffer[:0]

	// 如果没有数据，直接返回
	if len(totalData) == 0 {
		return 0, err
	}

	// 找到最后一个完整UTF-8字符的结束位置
	validLen := r.findLastValidUTF8Boundary(totalData, len(p))

	// 复制有效数据到输出缓冲区
	copy(p, totalData[:validLen])

	// 将剩余数据保存到内部缓冲区
	if validLen < len(totalData) {
		r.buffer = append(r.buffer, totalData[validLen:]...)
	}

	return validLen, err
}

// findLastValidUTF8Boundary 找到最后一个完整UTF-8字符的边界
func (r *utf8Reader) findLastValidUTF8Boundary(data []byte, maxLen int) int {
	if len(data) == 0 {
		return 0
	}

	// 限制检查长度
	checkLen := len(data)
	if checkLen > maxLen {
		checkLen = maxLen
	}

	// 特殊情况：如果缓冲区长度小于UTF-8字符最大长度，保证分开读
	if maxLen < 4 {
		// 对于小缓冲区，只返回能装下的字节数，即使可能在字符中间分割
		if checkLen <= maxLen {
			return checkLen
		}
		return maxLen
	}

	// 从后往前检查，找到最后一个有效的UTF-8字符边界
	for i := checkLen; i > 0; i-- {
		// 检查从开始到位置i的数据是否是有效的UTF-8
		if utf8.Valid(data[:i]) {
			return i
		}
	}

	// 如果都不是有效的UTF-8，可能是在字符中间被截断
	// 从后往前找第一个可能的UTF-8起始字节
	for i := checkLen - 1; i >= 0; i-- {
		b := data[i]

		// 检查是否是UTF-8起始字节
		if (b & 0x80) == 0 { // ASCII字符
			return i + 1
		} else if (b & 0xC0) == 0xC0 { // UTF-8多字节字符的起始字节
			// 检查从这个位置开始是否有完整的UTF-8字符
			remaining := data[i:]
			if utf8.Valid(remaining) {
				return len(data)
			}
			// 如果不完整，返回这个起始字节之前的位置
			return i
		}
	}

	// 如果都是continuation字节，返回0
	return 0
}

func UTF8Reader(r io.Reader) io.Reader {
	return &utf8Reader{r: r, buffer: make([]byte, 0)}
}
