package codec

import "github.com/yaklang/yaklang/common/gmsm/sm3"

// SM3 计算输入数据的国密 SM3 摘要，返回 32 字节摘要(注意是字节切片，打印前需用 codec.EncodeToHex 转可读)
// 参数:
//   - raw: 待计算摘要的数据，可为 string、[]byte 等
//
// 返回值:
//   - SM3 摘要字节切片(32 字节，转 hex 后长度 64)
//
// Example:
// ```
// // VARS: SM3 返回字节，需 EncodeToHex 转可读
// result = codec.EncodeToHex(codec.Sm3("abc"))
// // STDOUT: 打印可观察输出
// println(result)   // OUT: 66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0
// // assert: 锁定结论(已知摘要 + 固定长度)
// assert result == "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0", "Sm3 should match known digest"
// assert len(result) == 64, "Sm3 hex length should be 64"
// ```
func SM3(raw interface{}) []byte {
	return sm3.Sm3Sum(interfaceToBytes(raw))
}
