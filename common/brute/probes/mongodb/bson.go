// Package mongodb 实现最小化 MongoDB 认证探针。
//
// 包含：最小 BSON 编解码（仅覆盖命令所需类型）、OP_MSG 组帧、
// hello 探测与 SCRAM-SHA-1/SCRAM-SHA-256 (RFC 5802) 认证。
// 不引入 mongo-driver、连接池、拓扑与游标。
package mongodb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
)

// BSON 元素类型。
const (
	bsonDouble     = 0x01
	bsonString     = 0x02
	bsonDoc        = 0x03
	bsonArray      = 0x04
	bsonBinary     = 0x05
	bsonBool       = 0x08
	bsonNull       = 0x0A
	bsonDate       = 0x09 // UTC datetime：8 字节载荷
	bsonInt32      = 0x10
	bsonUTC        = 0x11
	bsonInt64      = 0x12
	bsonDecimal    = 0x13
	bsonObjectID   = 0x07
	bsonRegex      = 0x0B
	bsonDBPointer  = 0x0C
	bsonCode       = 0x0D
	bsonSymbol     = 0x0E
	bsonCodeWScope = 0x0F
	bsonMinKey     = 0xFF
	bsonMaxKey     = 0x7F
	bsonTimestamp  = 0x11
)

// cstringLen 返回 cstring（含 NUL）的字节长度。
func cstringLen(data []byte) (int, error) {
	for i, b := range data {
		if b == 0 {
			return i + 1, nil
		}
	}
	return 0, errBSON
}

var errBSON = errors.New("bson: malformed document")

// DElement 是有序文档元素（命令对字段顺序敏感，命令名必须居首）。
type DElement struct {
	Key   string
	Value interface{}
}

// D 是有序 BSON 文档。
type D []DElement

// S 快捷构造元素。
func S(key string, value interface{}) DElement { return DElement{Key: key, Value: value} }

// EncodeD 编码有序文档。
func EncodeD(doc D) []byte {
	buf := make([]byte, 4, 64)
	for _, e := range doc {
		buf = appendBSONElement(buf, e)
	}
	buf = append(buf, 0)
	binary.LittleEndian.PutUint32(buf, uint32(len(buf)))
	return buf
}

func appendBSONElement(buf []byte, e DElement) []byte {
	switch v := e.Value.(type) {
	case string:
		buf = append(buf, bsonString)
		buf = append(buf, e.Key...)
		buf = append(buf, 0)
		// BSON string: int32 长度（含 NUL）+ UTF-8 + NUL
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(v)+1))
		buf = append(buf, v...)
		return append(buf, 0)
	case int32:
		buf = append(buf, bsonInt32)
		buf = append(buf, e.Key...)
		buf = append(buf, 0)
		return binary.LittleEndian.AppendUint32(buf, uint32(v))
	case int:
		return appendBSONElement(buf, DElement{e.Key, int32(v)})
	case int64:
		buf = append(buf, bsonInt64)
		buf = append(buf, e.Key...)
		buf = append(buf, 0)
		return binary.LittleEndian.AppendUint64(buf, uint64(v))
	case bool:
		buf = append(buf, bsonBool)
		buf = append(buf, e.Key...)
		buf = append(buf, 0)
		b := byte(0)
		if v {
			b = 1
		}
		return append(buf, b)
	case float64:
		buf = append(buf, bsonDouble)
		buf = append(buf, e.Key...)
		buf = append(buf, 0)
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
		return append(buf, b[:]...)
	case []byte:
		buf = append(buf, bsonBinary)
		buf = append(buf, e.Key...)
		buf = append(buf, 0)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(v)))
		buf = append(buf, 0) // subtype 0
		return append(buf, v...)
	case D:
		buf = append(buf, bsonDoc)
		buf = append(buf, e.Key...)
		buf = append(buf, 0)
		return append(buf, EncodeD(v)...)
	case nil:
		buf = append(buf, bsonNull)
		buf = append(buf, e.Key...)
		return append(buf, 0)
	default:
		panic(fmt.Sprintf("bson: unsupported type %T", e.Value))
	}
}

func appendCString(buf []byte, s string) []byte {
	buf = append(buf, s...)
	return append(buf, 0)
}

// DecodeD 解析 BSON 文档为有序元素列表（键序保留）。
func DecodeD(data []byte) (D, error) {
	if len(data) < 5 {
		return nil, errBSON
	}
	length := int(binary.LittleEndian.Uint32(data))
	if length < 5 || length > len(data) {
		return nil, errBSON
	}
	pos := 4
	var out D
	for pos < length-1 {
		if pos >= len(data) {
			return nil, errBSON
		}
		elemType := data[pos]
		pos++
		// key cstring
		keyEnd := -1
		for i := pos; i < len(data); i++ {
			if data[i] == 0 {
				keyEnd = i
				break
			}
		}
		if keyEnd < 0 {
			return nil, errBSON
		}
		key := string(data[pos:keyEnd])
		pos = keyEnd + 1

		val, n, err := decodeValue(data[pos:], elemType)
		if err != nil {
			return nil, err
		}
		pos += n
		out = append(out, S(key, val))
	}
	return out, nil
}

func decodeValue(data []byte, t byte) (interface{}, int, error) {
	need := func(n int) error {
		if len(data) < n {
			return errBSON
		}
		return nil
	}
	switch t {
	case bsonDouble:
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(data)), 8, nil
	case bsonString:
		if err := need(4); err != nil {
			return nil, 0, err
		}
		n := int(int32(binary.LittleEndian.Uint32(data)))
		// BSON 字符串至少包含结尾 NUL（n >= 1）
		if n < 1 || n > 64<<20 {
			return nil, 0, errBSON
		}
		if err := need(4 + n); err != nil {
			return nil, 0, err
		}
		return string(data[4 : 4+n-1]), 4 + n, nil // 去掉结尾 NUL
	case bsonDoc, bsonArray:
		if err := need(4); err != nil {
			return nil, 0, err
		}
		n := int(binary.LittleEndian.Uint32(data))
		if n < 5 || n > len(data) {
			return nil, 0, errBSON
		}
		sub, err := DecodeD(data[:n])
		if err != nil {
			return nil, 0, err
		}
		return sub, n, nil
	case bsonBinary:
		if err := need(5); err != nil {
			return nil, 0, err
		}
		n := int(binary.LittleEndian.Uint32(data))
		subtype := data[4]
		if err := need(5 + n); err != nil {
			return nil, 0, err
		}
		_ = subtype
		return append([]byte{}, data[5:5+n]...), 5 + n, nil
	case bsonBool:
		if err := need(1); err != nil {
			return nil, 0, err
		}
		return data[0] == 1, 1, nil
	case bsonDate: // UTC datetime：跳过 8 字节
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return nil, 8, nil
	case bsonNull:
		return nil, 0, nil
	case bsonInt32:
		if err := need(4); err != nil {
			return nil, 0, err
		}
		return int32(binary.LittleEndian.Uint32(data)), 4, nil
	case bsonUTC:
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return int64(binary.LittleEndian.Uint64(data)), 8, nil
	case bsonInt64:
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return int64(binary.LittleEndian.Uint64(data)), 8, nil
	case bsonDecimal:
		if err := need(16); err != nil {
			return nil, 0, err
		}
		return nil, 16, nil // decimal128 不需要解析
	case bsonObjectID: // 0x07：hello 响应的 topologyVersion/serviceId
		if err := need(12); err != nil {
			return nil, 0, err
		}
		return append([]byte{}, data[:12]...), 12, nil
	case bsonRegex: // 0x0B: pattern cstring + options cstring
		n1, err := cstringLen(data)
		if err != nil {
			return nil, 0, err
		}
		if err := need(n1); err != nil {
			return nil, 0, err
		}
		n2, err := cstringLen(data[n1:])
		if err != nil {
			return nil, 0, err
		}
		return string(data[:n1-1]), n1 + n2, nil
	case bsonDBPointer: // 0x0C: string + 12 bytes
		if err := need(4); err != nil {
			return nil, 0, err
		}
		sl := int(int32(binary.LittleEndian.Uint32(data)))
		if sl < 1 || sl > 64<<20 {
			return nil, 0, errBSON
		}
		if err := need(4 + sl + 12); err != nil {
			return nil, 0, err
		}
		return nil, 4 + sl + 12, nil
	case bsonCode, bsonSymbol: // 0x0D / 0x0E: string
		if err := need(4); err != nil {
			return nil, 0, err
		}
		n := int(int32(binary.LittleEndian.Uint32(data)))
		if n < 1 || n > 64<<20 {
			return nil, 0, errBSON
		}
		if err := need(4 + n); err != nil {
			return nil, 0, err
		}
		return string(data[4 : 4+n-1]), 4 + n, nil
	case bsonCodeWScope: // 0x0F: int32 total + string + doc
		if err := need(4); err != nil {
			return nil, 0, err
		}
		total := int(int32(binary.LittleEndian.Uint32(data)))
		if total < 4 || total > len(data) {
			return nil, 0, errBSON
		}
		return nil, total, nil
	case bsonMinKey, bsonMaxKey: // 0xFF / 0x7F：无载荷
		return nil, 0, nil
	default:
		return nil, 0, fmt.Errorf("%w: unknown element type 0x%02x", errBSON, t)
	}
}

// Get 从有序文档取值。
func (d D) Get(key string) (interface{}, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// GetInt 取数值（兼容 int32/int64/float64）。
func (d D) GetInt(key string) (int64, bool) {
	v, ok := d.Get(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	}
	return 0, false
}

// GetString 取字符串。
func (d D) GetString(key string) (string, bool) {
	v, ok := d.Get(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetBool 取布尔。
func (d D) GetBool(key string) bool {
	v, _ := d.Get(key)
	b, _ := v.(bool)
	return b
}

// GetDoc 取子文档。
func (d D) GetDoc(key string) (D, bool) {
	v, ok := d.Get(key)
	if !ok {
		return nil, false
	}
	sub, ok := v.(D)
	return sub, ok
}

// GetBinary 取二进制。
func (d D) GetBinary(key string) ([]byte, bool) {
	v, ok := d.Get(key)
	if !ok {
		return nil, false
	}
	b, ok := v.([]byte)
	return b, ok
}

// Keys 返回全部键（有序，测试用）。
func (d D) Keys() []string {
	out := make([]string, len(d))
	for i, e := range d {
		out[i] = e.Key
	}
	return out
}

// SortedStringKeys 供确定性比较。
func (d D) SortedStringKeys() []string {
	keys := d.Keys()
	sort.Strings(keys)
	return keys
}

// GetInt32 取 int32 值。
func (d D) GetInt32(key string) (int32, bool) {
	v, ok := d.Get(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int32:
		return n, true
	case int64:
		return int32(n), true
	case float64:
		return int32(n), true
	}
	return 0, false
}

// IsOK 判断命令响应的 ok 字段（真实 MongoDB 返回 double 1.0）。
func (d D) IsOK() bool {
	if v, ok := d.GetInt("ok"); ok {
		return v == 1
	}
	return d.GetBool("ok")
}

// DecodeDForFuzz 仅供模糊测试导出。
func DecodeDForFuzz(data []byte) (D, error) {
	return DecodeD(data)
}
