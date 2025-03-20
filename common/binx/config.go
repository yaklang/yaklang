package binx

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type PartDescriptor struct {
	Identifier        string
	IdentifierVerbose string

	// List / Struct
	// if SubPartLength > 0 ? use List
	// if SubPartLength <= 0 ? use struct
	SubPartLength     uint64
	SubPartDescriptor []*PartDescriptor

	_byteOrder int

	// size
	size      uint64
	sizeFrom  string
	typeFlag  BinaryTypeVerbose
	ByteOrder ByteOrderEnum
	// for net.Conn
	timeout time.Duration
}

type PartDescriptorBuilder func(*PartDescriptor)

func NewDefaultNetworkPartDescriptor() *PartDescriptor {
	return &PartDescriptor{}
}

func (p *PartDescriptor) Config(i ...PartDescriptorBuilder) {
	for _, v := range i {
		v(p)
	}
}

func (p *PartDescriptor) Name(id string, verbose ...string) *PartDescriptor {
	p.Identifier = id
	p.IdentifierVerbose = strings.Join(verbose, " ")
	return p
}

func (p *PartDescriptor) SetIdentifier(id string, verbose ...string) *PartDescriptor {
	p.Identifier = id
	p.IdentifierVerbose = strings.Join(verbose, " ")
	return p
}

func (p *PartDescriptor) Verbose(verbose ...string) *PartDescriptor {
	p.IdentifierVerbose = strings.Join(verbose, " ")
	return p
}

func (p *PartDescriptor) IsEmpty() bool {
	if p == nil {
		return true
	}

	if p.size == 0 && p.sizeFrom == "" {
		if len(p.SubPartDescriptor) == 0 {
			return true
		}
	}
	return false
}

func (p *PartDescriptor) GetTimeoutDuration() time.Duration {
	if p == nil {
		return 5 * time.Second
	}
	if p.timeout > 0 {
		return p.timeout
	}
	return 5 * time.Second
}

func read(lastResults []ResultIf, p *PartDescriptor, reader io.Reader, startOffset int64) ([]ResultIf, int64, []ResultIf, error) {
	if p.IsEmpty() {
		return nil, startOffset, lastResults, nil
	}

	conn, ok := reader.(net.Conn)
	if ok {
		conn.SetReadDeadline(time.Now().Add(p.GetTimeoutDuration()))
		defer func() {
			conn.SetReadDeadline(time.Time{})
		}()
	}

	handleSubPartDesc := func(subs []*PartDescriptor, merged ResultCompactIf) error {
		var firstOffset int64 = -1
		// 对于Struct类型(SubPartLength=0)和List类型(SubPartLength>0)使用不同的处理逻辑
		var descriptorsToProcess []*PartDescriptor
		if p.SubPartLength > 0 {
			// List模式 - 只处理前SubPartLength个元素
			descriptorsToProcess = subs[:p.SubPartLength]
		} else {
			// Struct模式 - 处理所有元素
			descriptorsToProcess = subs
		}

		// 创建正确大小的bufs数组
		var bufs = make([]ResultIf, len(descriptorsToProcess))
		var bufsSize int64

		// 遍历需要处理的描述符
		for i, desc := range descriptorsToProcess {
			var err error
			var subResults []ResultIf
			subResults, startOffset, lastResults, err = read(lastResults, desc, reader, startOffset)
			if err != nil {
				return err
			}

			if len(subResults) > 0 {
				for _, subResult := range subResults {
					if firstOffset < 0 {
						firstOffset = subResult.GetInt64Offset()
					}
					bufs[i] = subResult
					bufsSize += int64(len(bufs[i].GetBytes()))
				}
			}
		}

		// 设置结果
		merged.SetOffset(firstOffset)
		var finalBytes = make([]byte, bufsSize)
		var offset = 0
		for _, buf := range bufs {
			if buf != nil { // 避免panic
				copy(finalBytes[offset:], buf.GetBytes())
				offset += len(buf.GetBytes())
			}
		}
		merged.SetBytes(finalBytes)
		merged.SetResults(bufs)
		return nil
	}

	// list
	if p.SubPartLength > 0 {
		if len(p.SubPartDescriptor) == 0 {
			return nil, startOffset, lastResults, utils.Error("SubPartLength > 0 but SubPartDescriptor is empty")
		}
		if p.SubPartLength > uint64(len(p.SubPartDescriptor)) {
			return nil, startOffset, lastResults, utils.Error("SubPartLength > len(SubPartDescriptor)")
		}

		p.SubPartDescriptor = p.SubPartDescriptor[:p.SubPartLength]
		result := NewListResult()
		result.Identifier = p.Identifier
		result.IdentifierVerbose = p.IdentifierVerbose
		err := handleSubPartDesc(p.SubPartDescriptor, result)
		if err != nil {
			return nil, startOffset, lastResults, err
		}
		return []ResultIf{
			result,
		}, startOffset, lastResults, nil
	}

	// struct
	if len(p.SubPartDescriptor) > 0 {
		result := NewStructResult()
		result.Identifier = p.Identifier
		result.IdentifierVerbose = p.IdentifierVerbose
		err := handleSubPartDesc(p.SubPartDescriptor, result)
		if err != nil {
			return nil, startOffset, lastResults, err
		}
		return []ResultIf{
			result,
		}, startOffset, lastResults, nil
	}

	// ordinary
	if p.size <= 0 && p.sizeFrom != "" {
		ret := FindResultByIdentifier(lastResults, p.sizeFrom)
		if ret == nil {
			return nil, 0, nil, utils.Errorf("sizeFrom %v not found", p.sizeFrom)
		}
		p.size = uint64(utils.InterfaceToInt(ret.Value()))
	}
	if p.size > 0 {
		var byteBuf = make([]byte, 1)
		var readBuffer = make([]byte, p.size)
		for i := uint64(0); i < p.size; i++ {
			bytes, err := io.ReadFull(reader, byteBuf)
			if bytes != 1 {
				if err == nil {
					err = io.EOF
				}
				return nil, startOffset, lastResults, err
			}
			startOffset++
			readBuffer[i] = byteBuf[0]
		}
		result := NewResult(readBuffer)
		result.SetOffset(startOffset - int64(p.size))
		result.Identifier = p.Identifier
		result.IdentifierVerbose = p.IdentifierVerbose
		if result.Identifier == "" {
			result.Identifier = fmt.Sprintf("offset_%v_%v", result.GetOffset(), startOffset)
		}
		result.Type = p.typeFlag
		lastResults = append(lastResults, result)
		return []ResultIf{result}, startOffset, lastResults, nil
	}

	return nil, startOffset, lastResults, utils.Error("unknown error, size or size from is not valid")
}

func NewPartDescriptor(dataType BinaryTypeVerbose, size uint64) *PartDescriptor {
	return &PartDescriptor{
		typeFlag: dataType,
		size:     size,
	}
}

// toList 创建一个列表类型描述符，用于从二进制数据中按顺序读取多个相同格式的元素
// @param {PartDescriptor} builder 列表中的元素描述符
// @return {PartDescriptor} 返回列表类型描述符对象
// Example:
// ```
// // 读取两个uint16构成的列表
// result = bin.Read(data, bin.toList(bin.toUint16("item"), bin.toUint16("item")))~
// list = result[0]
// item1 = list.Result[0].AsUint16()
// item2 = list.Result[1].AsUint16()
// ```
func NewListDescriptor(builder ...*PartDescriptor) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.SubPartLength = uint64(len(builder))
	descriptor.SubPartDescriptor = builder
	return descriptor
}

// toStruct 创建一个结构体类型描述符，用于从二进制数据中读取不同类型字段组成的结构
// @param {PartDescriptor} builder 结构体中的字段描述符
// @return {PartDescriptor} 返回结构体类型描述符对象
// Example:
// ```
// // 读取包含magic(uint16)和version(uint8)的结构体
// result = bin.Read(data, bin.toStruct(
//
//	bin.toUint16("magic"),
//	bin.toUint8("version")
//
// ))~
// structResult = result[0]
// magic = structResult.Result[0].AsUint16()
// version = structResult.Result[1].AsUint8()
// ```
func NewStructDescriptor(builder ...*PartDescriptor) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.SubPartLength = 0
	descriptor.SubPartDescriptor = builder
	return descriptor
}

// toUint8 创建一个8位无符号整数类型描述符，用于从二进制数据中读取uint8值
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewUint8(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 1
	descriptor.typeFlag = Uint8
	return descriptor.Name(name, values...)
}

// NewByte 创建一个字节类型描述符，等同于NewUint8，用于从二进制数据中读取单个字节
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewByte(name string, values ...string) *PartDescriptor {
	return NewUint8(name, values...)
}

// toUint16 创建一个16位无符号整数类型描述符，用于从二进制数据中读取uint16值
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewUint16(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 2
	descriptor.typeFlag = Uint16
	return descriptor.Name(name, values...)
}

// toUint32 创建一个32位无符号整数类型描述符，用于从二进制数据中读取uint32值
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewUint32(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 4
	descriptor.typeFlag = Uint32
	return descriptor.Name(name, values...)
}

// toUint64 创建一个64位无符号整数类型描述符，用于从二进制数据中读取uint64值
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewUint64(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 8
	descriptor.typeFlag = Uint64
	return descriptor.Name(name, values...)
}

// toInt8 创建一个8位整数类型描述符，用于从二进制数据中读取int8值
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewInt8(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 1
	descriptor.typeFlag = Int8
	return descriptor.Name(name, values...)
}

// toInt16 创建一个16位整数类型描述符，用于从二进制数据中读取int16值
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewInt16(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 2
	descriptor.typeFlag = Int16
	return descriptor.Name(name, values...)
}

// toInt32 创建一个32位整数类型描述符，用于从二进制数据中读取int32值
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewInt32(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 4
	descriptor.typeFlag = Int32
	return descriptor.Name(name, values...)
}

// toInt64 创建一个64位整数类型描述符，用于从二进制数据中读取int64值
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} values 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewInt64(name string, values ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 8
	descriptor.typeFlag = Int64
	return descriptor.Name(name, values...)
}

// toRaw 创建一个字节数组类型描述符，用于从二进制数据中读取字节序列
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {number|string} size 字节长度或引用其他字段名称作为长度值
// @return {PartDescriptor} 返回类型描述符对象
// Example:
// ```
// // 读取长度为5的字节数组
// bin.Read(data, bin.toBytes("content", 5))
//
// // 读取长度由另一个字段决定的字节数组
// bin.Read(data, bin.toUint8("length"), bin.toBytes("content", "length"))
// ```
func NewBytes(name string, size any) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	sizeFrom := utils.InterfaceToString(size)
	if utils.IsValidInteger(sizeFrom) {
		descriptor.size = uint64(utils.InterfaceToInt(size))
	} else {
		descriptor.sizeFrom = sizeFrom
	}
	descriptor.typeFlag = Bytes
	return descriptor.Name(name)
}

// NewBuffer 创建一个字节数组类型描述符，等同于NewBytes，用于从二进制数据中读取字节序列
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {number|string} size 字节长度或引用其他字段名称作为长度值
// @return {PartDescriptor} 返回类型描述符对象
func NewBuffer(name string, size any) *PartDescriptor {
	return NewBytes(name, size)
}

// toBool 创建一个布尔类型描述符，用于从二进制数据中读取布尔值（非零为true）
// @param {string} name 字段名称，用于之后通过Find函数查找
// @param {string} verbose 可选的详细描述
// @return {PartDescriptor} 返回类型描述符对象
func NewBool(name string, verbose ...string) *PartDescriptor {
	var descriptor = NewDefaultNetworkPartDescriptor()
	descriptor.size = 1
	descriptor.typeFlag = Bool
	return descriptor.Name(name, verbose...)
}
