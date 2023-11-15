package base

import (
	"errors"
	"github.com/icza/bitio"
	"io"
)

type BitReader struct {
	*bitio.Reader
}
type BitWriter struct {
	*bitio.Writer
	preIsByte  bool
	preByteLen uint8
}

func (b *BitReader) ReadBits(n uint64) ([]byte, error) {
	bytesLen := n / 8
	bitLen := n % 8
	buf := make([]byte, bytesLen)
	n1, err := b.Reader.Read(buf)
	if err != nil {
		return nil, err
	}
	if n1 != int(bytesLen) {
		return nil, io.ErrUnexpectedEOF
	}
	if bitLen > 0 {
		bit, err := b.Reader.ReadBits(uint8(bitLen))
		if err != nil {
			return nil, err
		}
		buf = append(buf, byte(bit))
	}
	return buf, nil
}
func NewBitReader(reader io.Reader) *BitReader {
	return &BitReader{bitio.NewReader(reader)}
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
	if b.preIsByte {
		if b.preByteLen+uint8(bitLen) != 8 {
			return errors.New("pre byte len not equal 8")
		}
		defer func() {
			b.preIsByte = false
			b.preByteLen = 0
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
			b.preIsByte = true
			b.preByteLen = uint8(bitLen)
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
