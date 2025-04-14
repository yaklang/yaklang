package constant_pool

import (
	"math"

	"github.com/yaklang/yaklang/common/javaclassparser/types"
)

/*
*
常量池中integer
四字节存储整数常量

	CONSTANT_INTEGER_INFO {
		u1 tag;
		u4 bytes;
	}
*/
type ConstantIntegerInfo struct {
	Type string
	//实际上，比int小的boolean、byte、short、char也可以放在里面
	Value int32
}

func (self *ConstantIntegerInfo) readInfo(cp types.ClassReader) {
	bytes := cp.ReadUint32()
	self.Value = int32(bytes)
}

func (self *ConstantIntegerInfo) writeInfo(writer types.ClassWriter) {
	writer.Write4Byte(uint32(self.Value))
}

func (self *ConstantIntegerInfo) GetTag() uint8 {
	return CONSTANT_Integer
}

func (self *ConstantIntegerInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantIntegerInfo) GetType() string {
	return self.Type
}

/*
*
常量池中float
四字节

	CONSTANT_FLOAT_INFO {
		u1 tag;
		u4 bytes;
	}
*/
type ConstantFloatInfo struct {
	Type  string
	Value float32
}

func (self *ConstantFloatInfo) readInfo(cp types.ClassReader) {
	bytes := cp.ReadUint32()
	self.Value = math.Float32frombits(bytes)
}

func (self *ConstantFloatInfo) writeInfo(writer types.ClassWriter) {
	bits := math.Float32bits(self.Value)
	writer.Write4Byte(bits)
}

func (self *ConstantFloatInfo) GetTag() uint8 {
	return CONSTANT_Float
}

func (self *ConstantFloatInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantFloatInfo) GetType() string {
	return self.Type
}

/*
*
常量池中long
特殊一些 八字节，分成高8字节和低8字节

	CONSTANT_LONG_INFO {
		u1 tag;
		u4 high_bytes;
		u4 low_bytes;
	}
*/
type ConstantLongInfo struct {
	Type  string
	Value uint64
}

func (self *ConstantLongInfo) readInfo(cp types.ClassReader) {
	self.Value = cp.ReadUint64()
}

func (self *ConstantLongInfo) writeInfo(writer types.ClassWriter) {
	writer.Write8Byte(self.Value)
}

func (self *ConstantLongInfo) GetTag() uint8 {
	return CONSTANT_Long
}

func (self *ConstantLongInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantLongInfo) GetType() string {
	return self.Type
}

/*
*
常量池中double
同样特殊 八字节

	CONSTANT_DOUBLE_INFO {
		u1 tag;
		u4 high_bytes;
		u4 low_bytes;
	}
*/
type ConstantDoubleInfo struct {
	Type  string
	Value float64
}

func (self *ConstantDoubleInfo) GetTag() uint8 {
	return CONSTANT_Double
}

func (self *ConstantDoubleInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantDoubleInfo) GetType() string {
	return self.Type
}

func (self *ConstantDoubleInfo) readInfo(cp types.ClassReader) {
	bytes := cp.ReadUint64()
	self.Value = math.Float64frombits(bytes)
}

func (self *ConstantDoubleInfo) writeInfo(writer types.ClassWriter) {
	bits := math.Float64bits(self.Value)
	writer.Write8Byte(bits)
}
