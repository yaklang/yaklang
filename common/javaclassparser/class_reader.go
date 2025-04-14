package javaclassparser

import "encoding/binary"

/*
*
jvm中定义了u1，u2，u4来表示1，2，4字节的 无 符号整数
相同类型的多条数据一般按表的形式存储在class文件中，由表头和表项构成，表头是u2或者u4整数。
假设表头为10，后面就紧跟着10个表项数据
*/
type ClassReader struct {
	//class data 以最小单位byte存储，及8位
	data []byte
}

func NewClassReader(data []byte) *ClassReader {
	return &ClassReader{data: data}
}

/*
*
相当于java的 byte 8位无符号整数
*/
func (this *ClassReader) ReadUint8() uint8 {
	val := this.data[0]
	this.data = this.data[1:]
	return val
}

/*
*
相当于java的 short 16位无符号整数
这里class文件在文件系统中以大端法存储
*/
func (this *ClassReader) ReadUint16() uint16 {
	//大端法读取16位的数据
	val := binary.BigEndian.Uint16(this.data)
	this.data = this.data[2:]
	return val
}

/*
*
相当于java的 int 32位无符号整数
*/
func (this *ClassReader) ReadUint32() uint32 {
	val := binary.BigEndian.Uint32(this.data)
	this.data = this.data[4:]
	return val
}

/*
*
相当于java的 long 64位无符号整数
*/
func (this *ClassReader) ReadUint64() uint64 {
	val := binary.BigEndian.Uint64(this.data)
	this.data = this.data[8:]
	return val
}

/*
*
读取uint16表，表的大小由开头的uint16数据指出
*/
func (this *ClassReader) ReadUint16s() []uint16 {
	n := this.ReadUint16()
	s := make([]uint16, n)
	for i := range s {
		s[i] = this.ReadUint16()
	}
	return s
}

/*
*
读取制定length数量的字节
*/
func (this *ClassReader) ReadBytes(length uint32) []byte {
	bytes := this.data[:length]
	this.data = this.data[length:]
	return bytes
}
