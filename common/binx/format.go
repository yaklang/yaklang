package binx

import (
	"bytes"
	"io"
	"reflect"

	"github.com/yaklang/yaklang/common/utils"
)

// Read 从字节数据或IO流中按照指定的数据类型描述符读取二进制数据，解析成结构化的结果
// 参数:
//   - data: 二进制数据或支持读取的流对象（[]byte、string 或 io.Reader）
//   - descriptors: 一个或多个数据类型描述符，可以是 toUint16() 等类型描述符
//
// 返回值:
//   - 解析结果切片，可通过索引访问各字段值
//   - 错误信息
//
// Example:
// ```
// // 解析二进制数据：前两字节为 magic(uint16)，后一字节为 version(uint8)
// data = codec.DecodeHex("123405")~
// result = bin.Read(data, bin.toUint16("magic"), bin.toUint8("version"))~
// println(result[0].AsUint16())   // OUT: 4660
// assert result[0].AsUint16() == 4660, "magic should be parsed as 0x1234"
// assert result[1].AsUint8() == 5, "version should be parsed as 5"
// ```
func BinaryRead(data any, descriptors ...*PartDescriptor) ([]ResultIf, error) {
	var reader io.Reader
	switch ret := data.(type) {
	case io.Reader:
		reader = ret
	case []byte:
		reader = bytes.NewBuffer(ret)
	case string:
		reader = bytes.NewBufferString(ret)
	case []rune:
		reader = bytes.NewBufferString(string(ret))
	default:
		return nil, utils.Errorf("unexpected type for input: %v", reflect.TypeOf(ret))
	}
	var results []ResultIf
	var ctx = make([]ResultIf, 0)
	var ret []ResultIf
	var err error
	for _, des := range descriptors {
		ret, _, ctx, err = read(ctx, des, reader, 0)
		if err != nil {
			return results, err
		}
		results = append(results, ret...)
	}
	return results, nil
}
