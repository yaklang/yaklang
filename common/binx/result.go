package binx

type BinaryTypeVerbose string

var Uint8 BinaryTypeVerbose = "uint8"
var Uint16 BinaryTypeVerbose = "uint16"
var Uint32 BinaryTypeVerbose = "uint32"
var Uint64 BinaryTypeVerbose = "uint64"
var Int8 BinaryTypeVerbose = "int8"
var Int16 BinaryTypeVerbose = "int16"
var Int32 BinaryTypeVerbose = "int32"
var Int64 BinaryTypeVerbose = "int64"
var Bytes BinaryTypeVerbose = "bytes"
var Bool BinaryTypeVerbose = "bool"

type ResultIf interface {
	GetBytes() []byte
	GetInt64Offset() int64
	GetOffset() int

	NetworkByteOrder() ResultIf
	BigEndian() ResultIf
	LittleEndian() ResultIf

	AsInt() int
	AsInt8() int8
	AsInt16() int16
	AsInt32() int32
	AsInt64() int64
	AsUint() uint
	AsUint8() uint8
	AsUint16() uint16
	AsUint32() uint32
	AsUint64() uint64
	AsBool() bool
	AsString() string
	Value() any
}

// Find 根据字段名称在解析结果中查找对应的字段值
// @param {[]ResultIf} results 通过bin.Read获取的解析结果
// @param {string} name 要查找的字段名称
// @return {ResultIf} 找到的字段值，如果未找到则返回nil
// Example:
// ```
// result = bin.Read(data, bin.toUint16("magic"), bin.toUint8("version"))~
// magic = bin.Find(result, "magic")
//
//	if magic != nil {
//	  println("Magic:", magic.AsUint16())
//	}
//
// ```
func FindResultByIdentifier(results []ResultIf, name string) ResultIf {
	for _, r := range results {
		switch ret := r.(type) {
		case *Result:
			if ret.Identifier == name {
				return ret
			}
		case *StructResult:
			for _, sub := range ret.Result {
				if subResult := FindResultByIdentifier([]ResultIf{sub}, name); subResult != nil {
					return subResult
				}
			}
		case *ListResult:
			for _, sub := range ret.Result {
				if subResult := FindResultByIdentifier([]ResultIf{sub}, name); subResult != nil {
					return subResult
				}
			}
		}
	}
	return nil
}

type ResultCompactIf interface {
	ResultIf

	SetBytes([]byte)
	SetOffset(int64)
	SetResults([]ResultIf)
}

type ByteOrderEnum int

var (
	BigEndianByteOrder    ByteOrderEnum = 0
	NetworkByteOrder                    = BigEndianByteOrder
	LittleEndianByteOrder ByteOrderEnum = 1
)

type ResultBase struct {
	Bytes             []byte
	Identifier        string
	IdentifierVerbose string
	Offset            int64
	ByteOrder         ByteOrderEnum
}

func (s *ResultBase) NetworkByteOrder() ResultIf {
	s.ByteOrder = NetworkByteOrder
	return s
}

func (s *ResultBase) BigEndian() ResultIf {
	s.ByteOrder = BigEndianByteOrder
	return s
}

func (s *ResultBase) LittleEndian() ResultIf {
	s.ByteOrder = LittleEndianByteOrder
	return s
}

func (r *ResultBase) Value() any {
	return r.GetBytes()
}

func (r *ResultBase) GetBytes() []byte {
	return r.Bytes
}

func (r *ResultBase) GetInt64Offset() int64 {
	return r.Offset
}

func (r *ResultBase) GetOffset() int {
	return int(r.Offset)
}

func (r *ResultBase) SetOffset(i int64) {
	r.Offset = i
}

func (r *ResultBase) SetBytes(b []byte) {
	r.Bytes = b
}

func (r *ResultBase) AsInt8() int8 {
	raw := r.GetBytes()
	if len(raw) < 1 {
		return 0
	}
	return int8(raw[0])
}

func (r *ResultBase) AsInt16() int16 {
	raw := r.GetBytes()
	if len(raw) < 2 {
		return 0
	}
	switch r.ByteOrder {
	case LittleEndianByteOrder:
		return int16(raw[1])<<8 | int16(raw[0])
	default:
		return int16(raw[0])<<8 | int16(raw[1])
	}
}

func (r *ResultBase) AsInt32() int32 {
	raw := r.GetBytes()
	if len(raw) < 4 {
		return 0
	}
	switch r.ByteOrder {
	case LittleEndianByteOrder:
		return int32(raw[3])<<24 | int32(raw[2])<<16 | int32(raw[1])<<8 | int32(raw[0])
	default:
		return int32(raw[0])<<24 | int32(raw[1])<<16 | int32(raw[2])<<8 | int32(raw[3])
	}
}

func (r *ResultBase) AsInt64() int64 {
	raw := r.GetBytes()
	if len(raw) < 8 {
		return 0
	}
	switch r.ByteOrder {
	case LittleEndianByteOrder:
		return int64(raw[7])<<56 | int64(raw[6])<<48 | int64(raw[5])<<40 | int64(raw[4])<<32 | int64(raw[3])<<24 | int64(raw[2])<<16 | int64(raw[1])<<8 | int64(raw[0])
	default:
		return int64(raw[0])<<56 | int64(raw[1])<<48 | int64(raw[2])<<40 | int64(raw[3])<<32 | int64(raw[4])<<24 | int64(raw[5])<<16 | int64(raw[6])<<8 | int64(raw[7])
	}
}

func (r *ResultBase) AsUint8() uint8 {
	raw := r.GetBytes()
	if len(raw) < 1 {
		return 0
	}
	return uint8(raw[0])
}

func (r *ResultBase) AsUint16() uint16 {
	raw := r.GetBytes()
	if len(raw) < 2 {
		return 0
	}
	switch r.ByteOrder {
	case LittleEndianByteOrder:
		return uint16(raw[1])<<8 | uint16(raw[0])
	default:
		return uint16(raw[0])<<8 | uint16(raw[1])
	}
}

func (r *ResultBase) AsUint32() uint32 {
	raw := r.GetBytes()
	if len(raw) < 4 {
		return 0
	}
	switch r.ByteOrder {
	case LittleEndianByteOrder:
		return uint32(raw[3])<<24 | uint32(raw[2])<<16 | uint32(raw[1])<<8 | uint32(raw[0])
	default:
		return uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
	}
}

func (r *ResultBase) AsUint64() uint64 {
	raw := r.GetBytes()
	if len(raw) < 8 {
		return 0
	}
	switch r.ByteOrder {
	case LittleEndianByteOrder:
		return uint64(raw[7])<<56 | uint64(raw[6])<<48 | uint64(raw[5])<<40 | uint64(raw[4])<<32 | uint64(raw[3])<<24 | uint64(raw[2])<<16 | uint64(raw[1])<<8 | uint64(raw[0])
	default:
		return uint64(raw[0])<<56 | uint64(raw[1])<<48 | uint64(raw[2])<<40 | uint64(raw[3])<<32 | uint64(raw[4])<<24 | uint64(raw[5])<<16 | uint64(raw[6])<<8 | uint64(raw[7])
	}
}

func (r *ResultBase) AsBool() bool {
	raw := r.GetBytes()
	if len(raw) < 1 {
		return false
	}
	return raw[0] != 0
}

func (r *ResultBase) AsInt() int {
	return int(r.AsInt64())
}

func (r *ResultBase) AsUint() uint {
	return uint(r.AsUint64())
}

func (r *ResultBase) AsString() string {
	return string(r.GetBytes())
}

type Result struct {
	*ResultBase

	Type        BinaryTypeVerbose
	TypeVerbose string
}

type StructResult struct {
	*ResultBase

	Result []ResultIf
}

type ListResult struct {
	*ResultBase

	Length int
	Result []ResultIf
}

func (r *ListResult) SetResults(i []ResultIf) {
	r.Result = i
	r.Length = len(i)
}

func (r *StructResult) SetResults(i []ResultIf) {
	r.Result = i
}

func NewResult(raw []byte) *Result {
	return &Result{
		ResultBase: &ResultBase{Bytes: raw},
	}
}

func NewListResult() *ListResult {
	return &ListResult{ResultBase: &ResultBase{}}
}

func NewStructResult() *StructResult {
	return &StructResult{ResultBase: &ResultBase{}}
}

func (r *Result) Value() any {
	switch r.Type {
	case Uint8:
		return r.AsUint8()
	case Uint16:
		return r.AsUint16()
	case Uint32:
		return r.AsUint32()
	case Uint64:
		return r.AsUint64()
	case Int8:
		return r.AsInt8()
	case Int16:
		return r.AsInt16()
	case Int32:
		return r.AsInt32()
	case Int64:
		return r.AsInt64()
	case Bytes:
		return r.AsString()
	case Bool:
		return r.AsBool()
	default:
		return r.GetBytes()
	}
}
