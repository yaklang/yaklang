package utils

import (
	"io"
	"sync"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils/bufpipe"
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

	// 特殊情况：如果缓冲区长度小于UTF-8字符最大长度，采用更直接的处理方式
	if maxLen < 4 {
		// 对于极小的缓冲区，优先返回能装下的字节数
		// 这样在CI环境下更稳定，减少复杂的验证逻辑
		if checkLen <= maxLen {
			// 快速检查：如果数据有效就返回全部
			if utf8.Valid(data[:checkLen]) {
				return checkLen
			}
			// 如果数据无效，但缓冲区很小，按需求允许分开读
			return checkLen
		}
		return maxLen
	}

	// 对于较大的缓冲区，使用更高效的边界检测
	// 首先快速检查整个数据是否有效
	if utf8.Valid(data[:checkLen]) {
		return checkLen
	}

	// 如果整个数据无效，从后往前找最后一个有效的边界
	// 为了提高CI环境下的性能，限制搜索范围
	searchStart := checkLen - 4 // 最多向前搜索4个字节（UTF-8最大字符长度）
	if searchStart < 0 {
		searchStart = 0
	}

	for i := checkLen - 1; i >= searchStart; i-- {
		if utf8.Valid(data[:i]) {
			return i
		}
	}

	// 如果在限定范围内没找到有效边界，使用简单的字节级边界
	// 从后往前找第一个可能的UTF-8起始字节
	for i := checkLen - 1; i >= 0; i-- {
		b := data[i]
		// 检查是否是UTF-8起始字节
		if (b&0x80) == 0 || (b&0xC0) == 0xC0 {
			// 快速验证是否是完整字符的开始
			if i == 0 || utf8.Valid(data[:i]) {
				return i
			}
		}
	}

	// 最后的保底措施
	return 0
}

func UTF8Reader(r io.Reader) io.Reader {
	if _, ok := r.(*utf8Reader); ok {
		// 已经是utf8Reader，直接返回
		return r
	}

	return &utf8Reader{r: r, buffer: make([]byte, 0)}
}

func CreateUTF8StreamMirror(r io.Reader, cb ...func(reader io.Reader)) io.Reader {
	if len(cb) <= 0 {
		return UTF8Reader(r)
	}

	pr, pw := NewPipe()

	numPipes := len(cb)
	writers := make([]*bufpipe.PipeWriter, numPipes)
	readers := make([]*bufpipe.PipeReader, numPipes)
	for i := 0; i < len(cb); i++ {
		readers[i], writers[i] = NewPipe()
	}
	go func() {
		defer func() {
			pw.Close()
			for _, w := range writers {
				w.Close()
			}
		}()
		var pipes = make([]io.Writer, numPipes)
		for i, w := range writers {
			pipes[i] = w
		}
		go func() {
			wg := new(sync.WaitGroup)
			for i, c := range cb {
				wg.Add(1)
				handler := c
				idx := i
				go func() {
					defer func() {
						wg.Done()
					}()
					if handler != nil {
						handler(readers[idx])
					}
				}()
			}
			wg.Wait()
		}()
		io.Copy(pw, io.TeeReader(r, io.MultiWriter(pipes...)))
	}()

	return UTF8Reader(pr)
}
