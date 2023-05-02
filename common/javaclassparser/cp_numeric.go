package javaclassparser

import "math"

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

func (self *ConstantIntegerInfo) readInfo(cp *ClassParser) {
	bytes := cp.reader.readUint32()
	self.Value = int32(bytes)
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

func (self *ConstantFloatInfo) readInfo(cp *ClassParser) {
	bytes := cp.reader.readUint32()
	self.Value = math.Float32frombits(bytes)
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
	Value int64
}

func (self *ConstantLongInfo) readInfo(cp *ClassParser) {
	bytes := cp.reader.readUint64()
	self.Value = int64(bytes)
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

func (self *ConstantDoubleInfo) readInfo(cp *ClassParser) {
	bytes := cp.reader.readUint64()
	self.Value = math.Float64frombits(bytes)
}
