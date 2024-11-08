package padding

import (
	"bytes"
	"errors"
	"io"
)

// ZeroPaddingReader 符合PKCS#7填充的输入流
type ZeroPaddingReader struct {
	fIn       io.Reader
	padding   []byte
	blockSize int
	readed    int64
	eof       bool
	eop       bool
}

// NewZeroPaddingReader 创建Zero填充Reader
// in: 输入流
// blockSize: 分块大小
func NewZeroPaddingReader(in io.Reader, blockSize int) PaddingReader {
	return &ZeroPaddingReader{
		fIn:       in,
		padding:   nil,
		eof:       false,
		eop:       false,
		blockSize: blockSize,
	}
}

func (p *ZeroPaddingReader) ReadBlock() ([]byte, error) {
	if p.eof && p.eop {
		return nil, io.EOF
	}

	buf := make([]byte, p.blockSize)
	var off = 0
	if !p.eof {
		// read from reader
		n, err := io.ReadFull(p.fIn, buf)
		if err == nil {
			return buf, nil
		}
		p.readed += int64(n)
		if errors.Is(err, io.EOF) {
			p.eof = true
			return buf, io.EOF
		} else if errors.Is(err, io.ErrUnexpectedEOF) {
			p.eof = true
			p.newPadding()
			off = n
		} else {
			return nil, err
		}
	}

	if !p.eop {
		if len(p.padding) == p.blockSize-off {
			copy(buf[off:], p.padding)
			p.eop = true
		}
	}
	return buf, nil
}

func (p *ZeroPaddingReader) Read(buf []byte) (int, error) {
	/*
		- 读取文件
			- 文件长度充足， 直接返还
			- 不充足
		- 读取到 n 字节， 剩余需要 m 字节
		- 从 padding 中读取然后追加到 buff
			- EOF  直接返回， 整个Reader end
	*/
	// 都读取完了
	if p.eof && p.eop {
		return 0, io.EOF
	}

	var n, off = 0, 0
	var err error
	if !p.eof {
		// 读取文件
		n, err = p.fIn.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			// 错误返回
			return 0, err
		}
		p.readed += int64(n)
		if errors.Is(err, io.EOF) {
			// 标志文件结束
			p.eof = true
		}
		if n == len(buf) {
			// 长度足够直接返回
			return n, err
		}
		p.newPadding()
		off = n
	}

	if !p.eop {
		if len(p.padding) == p.blockSize-off {
			copy(buf[off:], p.padding)
			p.eop = true
			n += len(p.padding)
		}
	}
	return n, err
}

// 新建Padding
func (p *ZeroPaddingReader) newPadding() {
	if p.padding != nil {
		return
	}
	size := p.blockSize - int(p.readed%int64(p.blockSize))
	p.padding = bytes.Repeat([]byte{0x00}, size)
}

// ZeroPaddingWriter 符合PKCS#7去除的输入流，最后一个 分组根据会根据填充情况去除填充。
type ZeroPaddingWriter struct {
	cache     *bytes.Buffer // 缓存区
	out       io.Writer     // 输出位置
	blockSize int           // 分块大小
}

// NewZeroPaddingWriter PKCS#7 填充Writer 可以去除填充
func NewZeroPaddingWriter(out io.Writer, blockSize int) PaddingWriter {
	cache := bytes.NewBuffer(make([]byte, 0, 1024))
	return &ZeroPaddingWriter{out: out, blockSize: blockSize, cache: cache}
}

// Write 保留一个填充大小的数据，其余全部写入输出中
func (p *ZeroPaddingWriter) Write(buff []byte) (n int, err error) {
	// 写入缓存
	n, err = p.cache.Write(buff)
	if err != nil {
		return 0, err
	}
	if p.cache.Len() > p.blockSize {
		size := p.cache.Len() - p.blockSize
		swap := make([]byte, size)
		_, _ = p.cache.Read(swap)
		_, err = p.out.Write(swap)
		if err != nil {
			return 0, err
		}
	}
	return n, err

}

// Final 去除填充写入最后一个分块
func (p *ZeroPaddingWriter) Final() error {
	b := p.cache.Bytes()
	length := len(b)
	if length%p.blockSize != 0 {
		return errors.New("非法的Zero填充")
	}
	_, err := p.out.Write(bytes.TrimRight(b, string([]byte{0x0})))
	return err
}
