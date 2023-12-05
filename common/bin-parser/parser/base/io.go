package base

import (
	"bytes"
	"errors"
	"github.com/icza/bitio"
	"io"
)

type ConcatReader struct {
	readers []io.Reader
}

func NewConcatReader(readers ...io.Reader) *ConcatReader {
	return &ConcatReader{readers: readers}
}

func (cr *ConcatReader) Read(p []byte) (n int, err error) {
	current := 0
	allEOF := true
	for len(cr.readers) > 0 {
		if current >= len(p) {
			break
		}
		n, err = cr.readers[0].Read(p[current:])
		current += n
		if err == io.EOF {
			cr.readers = cr.readers[1:]
		} else {
			allEOF = false
		}
	}
	if allEOF {
		return current, io.EOF
	}
	return current, nil
}

type bitReaderBackup struct {
	afterRead func(d []byte, length uint64) error
	recovery  func() error
}
type BitReader struct {
	Reader     *bitio.Reader
	backupList []*bitReaderBackup
}
type BitWriter struct {
	*bitio.Writer
	PreIsBit   bool
	PreByte    uint8
	PreByteLen uint8
}

func (r *BitReader) Read(p []byte) (n int, err error) {
	return r.Reader.Read(p)
}
func (b *BitReader) Backup() error {
	var buf bytes.Buffer
	writer := NewBitWriter(&buf)
	bak := &bitReaderBackup{}
	b.backupList = append(b.backupList, bak)
	bak.afterRead = func(d []byte, length uint64) error {
		return writer.WriteBits(d, length)
	}
	bak.recovery = func() error {
		if len(b.backupList) == 0 {
			return errors.New("no backup")
		}
		if writer.PreIsBit {
			n, err := b.ReadBits(uint64(8 - writer.PreByteLen))
			if err != nil {
				return err
			}
			if len(n) != 1 {
				return errors.New("read bits error")
			}
			err = writer.WriteBits(n, uint64(8-writer.PreByteLen))
			if err != nil {
				return err
			}
		}
		b.Reader = bitio.NewReader(NewConcatReader(&buf, b.Reader))
		b.backupList = b.backupList[:len(b.backupList)-1]
		return nil
	}
	return nil
}
func (b *BitReader) PopBackup() error {
	if len(b.backupList) == 0 {
		return errors.New("no backup")
	}
	b.backupList = b.backupList[:len(b.backupList)-1]
	return nil
}
func (b *BitReader) Recovery() error {
	if len(b.backupList) == 0 {
		return errors.New("no backup")
	}
	return b.backupList[len(b.backupList)-1].recovery()
}
func (b *BitReader) ReadBits(n uint64) ([]byte, error) {
	syncBak := func(buf []byte, n uint64) error {
		for _, backup := range b.backupList {
			err := backup.afterRead(buf, n)
			if err != nil {
				return err
			}
		}
		return nil
	}
	bytesLen := n / 8
	bitLen := n % 8
	buf := make([]byte, bytesLen)
	n1, err := b.Reader.Read(buf)
	err = syncBak(buf, uint64(n1*8))
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if n1 != int(bytesLen) {
		return nil, io.ErrUnexpectedEOF
	}
	if bitLen > 0 {
		bit, err := b.Reader.ReadBits(uint8(bitLen))
		err = syncBak([]byte{byte(bit)}, bitLen)
		if err != nil {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		buf = append(buf, byte(bit))
	}
	return buf, nil
}
func NewBitReader(reader io.Reader) *BitReader {
	return &BitReader{
		Reader: bitio.NewReader(reader),
	}
}
func NewBitWriter(writer io.Writer) *BitWriter {
	return &BitWriter{Writer: bitio.NewWriter(writer)}
}
func (b *BitWriter) WriteBits(bs []byte, length uint64) (err error) {
	if length == 0 {
		return nil
	}
	bytesLen := length / 8
	bitLen := length % 8
	if b.PreIsBit {
		if b.PreByteLen+uint8(bitLen) != 8 {
			return errors.New("pre byte len not equal 8")
		}
		defer func() {
			b.PreIsBit = false
			b.PreByteLen = 0
		}()
		if len(bs) == 0 {
			return errors.New("empty bytes")
		}
		err = b.Writer.WriteBits(uint64(bs[0]), uint8(bitLen))
		if err != nil {
			return err
		}
		bs = bs[1:]
		if bytesLen > 0 {
			_, err = b.Writer.Write(bs[:bytesLen])
			if err != nil {
				return err
			}
		}
	} else {
		if bitLen != 0 {
			b.PreIsBit = true
			b.PreByte = bs[bytesLen]
			b.PreByteLen = uint8(bitLen)
		}
		if bytesLen > 0 {
			_, err = b.Writer.Write(bs[:bytesLen])
			if err != nil {
				return err
			}
		}
		if bitLen != 0 {
			err = b.Writer.WriteBits(uint64(bs[bytesLen]), uint8(bitLen))
			if err != nil {
				return err
			}
		}
	}
	return nil
}
