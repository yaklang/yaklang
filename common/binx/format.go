package binx

import (
	"bytes"
	"io"
	"reflect"

	"github.com/yaklang/yaklang/common/utils"
)

// Read 从字节数据或IO流中按照指定的数据类型描述符读取二进制数据，解析成结构化的结果
// @param {[]byte|io.Reader} data 二进制数据或支持读取的流对象
// @param {PartDescriptor} descriptors 数据类型描述符，可以是toUint16()等类型描述符
// @return {[]ResultIf} 解析结果，可通过索引访问各字段值
// @return {error} 错误信息
// Example:
// ```
// // 解析二进制数据
// data := codec.DecodeHex("1234ABCD")~
// result = bin.Read(data, bin.toUint16("magic"), bin.toUint16("version"))~
//
// // 访问解析结果
// magic := result[0].AsUint16()
// version := result[1].AsUint16()
// println("Magic:", magic, "Version:", version)
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
