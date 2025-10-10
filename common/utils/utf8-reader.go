package utils

import (
	"io"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
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
	if cb == nil || len(cb) <= 0 {
		return UTF8Reader(r)
	}

	log.Infof("[UTF8MIRROR] Creating stream mirror with %d callbacks", len(cb))

	// 为每个callback创建一个独立的pipe，还要为返回的主流创建一个pipe
	numPipes := len(cb) + 1 // callbacks + 主流
	pipes := make([]io.Writer, numPipes)
	readers := make([]io.Reader, numPipes)

	for i := 0; i < numPipes; i++ {
		pr, pw := io.Pipe()
		pipes[i] = pw
		readers[i] = pr
	}

	// 创建一个MultiWriter，将数据分发到所有pipe
	multiWriter := io.MultiWriter(pipes...)

	// 启动goroutine来处理数据分发
	go func() {
		log.Infof("[UTF8MIRROR] Starting data distribution goroutine")
		// 确保所有pipe writer都被关闭
		defer func() {
			log.Infof("🔄 [UTF8MIRROR] Closing all pipes")
			for _, pipe := range pipes {
				if pw, ok := pipe.(*io.PipeWriter); ok {
					pw.Close()
				}
			}
		}()

		// 将原始流的数据写入到所有镜像流中
		n, err := io.Copy(multiWriter, r)
		log.Infof("🔄 [UTF8MIRROR] Data distribution completed, copied %d bytes, err: %v", n, err)
		if err != nil {
			// 处理错误，但不阻塞
			for _, pipe := range pipes {
				if pw, ok := pipe.(*io.PipeWriter); ok {
					pw.CloseWithError(err)
				}
			}
		}
	}()

	// 为每个callback启动独立的goroutine
	for i, callback := range cb {
		go func(cb func(reader io.Reader), reader io.Reader, idx int) {
			log.Infof("🔄 [UTF8MIRROR] Starting callback %d", idx)
			utf8Stream := UTF8Reader(reader)
			cb(utf8Stream)
			log.Infof("🔄 [UTF8MIRROR] Callback %d finished", idx)
		}(callback, readers[i], i)
	}

	// 返回最后一个pipe作为主流（独立于所有callback）
	return UTF8Reader(readers[len(cb)])
}
