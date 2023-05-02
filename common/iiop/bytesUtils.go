package iiop

import (
	"encoding/binary"
	"yaklang.io/yaklang/common/utils"
)

type BytesUtils struct {
	pos  int
	data []byte
}

func NewBytesUtils(data []byte) *BytesUtils {
	return &BytesUtils{
		data: data,
		pos:  0,
	}
}
func (b *BytesUtils) NewChildBytesUtils(n int) (*BytesUtils, error) {
	data, err := b.ReadBytes(n)
	if err != nil {
		return nil, err
	}
	return &BytesUtils{data: data, pos: 0}, nil

}
func (b *BytesUtils) ReadOthers() ([]byte, error) {
	if b.pos > len(b.data) {
		return nil, utils.Error("Index out of bounds")
	}
	return b.data[b.pos:], nil
}
func (b *BytesUtils) ReadByteUnsafe() byte {
	nextPos := b.pos + 1
	b.pos = nextPos
	return b.data[nextPos-1]
}
func (b *BytesUtils) ReadByte() (byte, error) {
	nextPos := b.pos + 1
	if nextPos > len(b.data) {
		return 0, utils.Error("Index out of bounds")
	}
	b.pos = nextPos
	return b.data[nextPos-1], nil
}
func (b *BytesUtils) ReadBytesUnsafe(n int) []byte {
	nextPos := b.pos + n
	b.pos = nextPos
	return b.data[nextPos-n : nextPos]
}
func (b *BytesUtils) Next(n int) {
	b.pos = b.pos + n
}
func (b *BytesUtils) ReadBytes(n int) ([]byte, error) {
	nextPos := b.pos + n
	if nextPos > len(b.data) {
		return nil, utils.Error("Index out of bounds")
	}
	return b.ReadBytesUnsafe(n), nil
}
func (b *BytesUtils) Read4BytesToIntUnsafe() int {
	d := b.ReadBytesUnsafe(4)
	n := binary.BigEndian.Uint32(d)
	return int(n)
}
func (b *BytesUtils) Read4BytesToInt() (int, error) {
	d, err := b.ReadBytes(4)
	if err != nil {
		return 0, err
	}
	n := binary.BigEndian.Uint32(d)
	return int(n), nil
}
func (b *BytesUtils) Read2BytesToIntUnsafe() int {
	d := b.ReadBytesUnsafe(2)
	n := binary.BigEndian.Uint32(d)
	return int(n)
}
func (b *BytesUtils) Read2BytesToInt() (int, error) {
	d, err := b.ReadBytes(2)
	if err != nil {
		return 0, err
	}
	n := binary.BigEndian.Uint16(d)
	return int(n), nil
}
func (b *BytesUtils) ReadByteToUint16Unsafe() uint16 {
	d := b.ReadBytesUnsafe(1)
	n := binary.BigEndian.Uint16(d)
	return n
}

func (b *BytesUtils) ReadByteToUint16() (uint16, error) {
	d, err := b.ReadBytes(1)
	if err != nil {
		return 0, err
	}
	n := binary.BigEndian.Uint16(d)
	return n, nil
}
