package jreader

import (
	"bytes"
	"encoding/binary"
	"io"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
)

func MarshalUTFString(str string) []byte {
	return append(IntTo2Bytes(len(str)), []byte(str)...)
}

func MarshalBlockDataBytes(data []byte) []byte {
	var raw []byte
	if len(data) <= 255 {
		raw = append(raw, yserx.TC_BLOCKDATA)
		raw = append(raw, IntToByte(len(raw))...)
		raw = append(raw, data...)
		return raw
	} else {
		raw = append(raw, yserx.TC_BLOCKDATALONG)
		raw = append(raw, IntTo4Bytes(len(raw))...)
		raw = append(raw, data...)
		return raw
	}
}

func MarshalBlockDataByte(data []byte) []byte {
	var raw []byte
	for _, b := range data {
		raw = append(raw, MarshalBlockDataBytes([]byte{b})...)
	}
	return raw
}

func LoadUTFString(r io.Reader) (string, error) {
	l, err := Read2ByteToInt(r)
	if err != nil {
		return "", err
	}
	raw, err := ReadBytesLengthInt(r, l)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// utils method
func ReadBytesLength(r io.Reader, length uint64) ([]byte, error) {
	var buf = make([]byte, length)

	n, err := io.ReadAtLeast(r, buf, int(length))
	if err != nil {
		return buf[:], err
	}

	if uint64(n) < length {
		return buf[:], utils.Errorf("readbytes failed current length[%v] is not [%v]", n, length)
	}
	return buf[:], nil
}

func ReadBytesLengthInt(r io.Reader, length int) ([]byte, error) {
	return ReadBytesLength(r, uint64(length))
}

func Read2ByteToInt(r io.Reader) (int, error) {
	raw, err := ReadBytesLength(r, 2)
	if err != nil {
		return 0, utils.Errorf("read bytes length failed: %s", err)
	}

	raw = append(bytes.Repeat([]byte{0x00}, 6), raw...)
	i := binary.BigEndian.Uint64(raw)
	return int(i), nil
}

func ReadByteToInt(r io.Reader) (int, error) {
	raw, err := ReadBytesLength(r, 1)
	if err != nil {
		return 0, utils.Errorf("read bytes length failed: %s", err)
	}

	raw = append(bytes.Repeat([]byte{0x00}, 7), raw...)
	i := binary.BigEndian.Uint64(raw)
	return int(i), nil
}

func Read4ByteToInt(r io.Reader) (int, error) {
	i, err := Read4ByteToUint64(r)
	if err != nil {
		return 0, err
	}
	return int(i), nil
}

func Read4ByteToUint64(r io.Reader) (uint64, error) {
	raw, err := ReadBytesLength(r, 4)
	if err != nil {
		return 0, utils.Errorf("read bytes length failed: %s", err)
	}

	raw = append(bytes.Repeat([]byte{0x00}, 4), raw...)
	return binary.BigEndian.Uint64(raw), nil
}

func Read8BytesToUint64(r io.Reader) (uint64, error) {
	raw, err := ReadBytesLength(r, 8)
	if err != nil {
		return 0, utils.Errorf("read bytes length failed: %s", err)
	}
	return binary.BigEndian.Uint64(raw), nil
}

func IntTo2Bytes(i int) []byte {
	var buf = make([]byte, 2)
	buf[0] = byte(i >> 8)
	buf[1] = byte(i)
	return buf[:]
}

func IntToByte(i int) []byte {
	var buf = make([]byte, 1)
	buf[0] = byte(i)
	return buf[:]
}

func IntTo4Bytes(i int) []byte {
	var buf = make([]byte, 4)
	buf[0] = byte(i >> 24)
	buf[1] = byte(i >> 16)
	buf[2] = byte(i >> 8)
	buf[3] = byte(i)
	return buf[:]
}

func Uint64To4Bytes(i uint64) []byte {
	var buf = make([]byte, 4)
	buf[0] = byte(i >> 24)
	buf[1] = byte(i >> 16)
	buf[2] = byte(i >> 8)
	buf[3] = byte(i)
	return buf[:]
}

func Uint64To8Bytes(i uint64) []byte {
	var buf = make([]byte, 8)
	buf[0] = byte(i >> 56)
	buf[1] = byte(i >> 48)
	buf[2] = byte(i >> 40)
	buf[3] = byte(i >> 32)
	buf[4] = byte(i >> 24)
	buf[5] = byte(i >> 16)
	buf[6] = byte(i >> 8)
	buf[7] = byte(i)
	return buf[:]
}
