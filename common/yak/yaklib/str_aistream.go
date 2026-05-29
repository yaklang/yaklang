package yaklib

import (
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/utils"
)

// NewUTF8Reader 包装一个 Reader，使每次读取都只返回完整的 UTF-8 字符，
// 避免在按字节/小缓冲读取数据流（如 jsonstream 字段流）时把一个多字节字符（中文等）从中间截断。
// Example:
// ```
// // 配合 jsonstream 字段流逐块读取中文内容时避免乱码
// r = str.NewUTF8Reader(rawReader)
// buf = make([]byte, 4)
// n, err = r.Read(buf) // buf[:n] 一定是完整的 UTF-8 字符序列
// ```
func NewUTF8Reader(r io.Reader) io.Reader {
	return utils.UTF8Reader(r)
}

// NewJSONStringReader 包装一个 JSON 字符串值的 Reader，流式解码其中的转义（\n、\t、\uXXXX、\xNN 等），
// 返回去引号、去转义后的真实字符串内容。内部已用 UTF-8 安全读取，遇到非标准/畸形数据会自动回退为原始透传。
// 常用于 jsonstream 的 onField 回调：字段流给出的是带引号和转义的原始值，用它解码即可拿到真实内容。
// Example:
// ```
// jsonstream.Extract(`{"content": "Hello\nYak \u4f60\u597d"}`,
//
//	jsonstream.onField("content", func(key, reader, parents) {
//	    data = io.ReadAll(str.NewJSONStringReader(reader))~
//	    println(string(data)) // Hello(换行)Yak 你好
//	}),
//
// )
// ```
func NewJSONStringReader(r io.Reader) io.Reader {
	return utils.JSONStringReader(r)
}

// JsonStringDecode 解码一个 JSON 字符串值（一次性版本），去掉首尾引号并还原转义（\n、\t、\uXXXX、\xNN 等），
// 返回真实字符串内容。相比 str.Unquote 更容错：遇到非标准/畸形数据会回退返回原始内容而不是报错。
// 适合处理 jsonstream onField 给出的带引号原始值。
// Example:
// ```
// str.JsonStringDecode(`"Hello\nYak"`)            // Hello(换行)Yak
// str.JsonStringDecode(`"\u4f60\u597d"`)          // 你好
// ```
func JsonStringDecode(raw string) string {
	data, _ := io.ReadAll(utils.JSONStringReader(strings.NewReader(raw)))
	return string(data)
}

// CalcTokenCount 计算文本的 token 数量（基于 Qwen BPE 词表），会识别特殊 token（如 <|im_start|>、<|im_end|>、<|endoftext|>）。
// 常用于大模型上下文预算估算、长文本裁剪等 AI 处理场景。
// Example:
// ```
// n = str.CalcTokenCount("Hello, Yak!")   // 英文 token 数
// n = str.CalcTokenCount("你好，世界")      // 中文 token 数
// ```
func CalcTokenCount(text string) int {
	return ytoken.CalcTokenCount(text)
}

// CalcOrdinaryTokenCount 计算文本的 token 数量（基于 Qwen BPE 词表），但不对特殊 token 做识别处理。
// Example:
// ```
// n = str.CalcOrdinaryTokenCount("Hello, Yak!")
// ```
func CalcOrdinaryTokenCount(text string) int {
	return ytoken.CalcOrdinaryTokenCount(text)
}

// EncodeTokens 将文本编码为 Qwen BPE token id 列表（会识别特殊 token）。
// Example:
// ```
// ids = str.EncodeTokens("Hello, Yak!")
// println(len(ids))
// ```
func EncodeTokens(text string) []int {
	return ytoken.Encode(text)
}

// EncodeOrdinaryTokens 将文本编码为 Qwen BPE token id 列表（不识别特殊 token）。
// Example:
// ```
// ids = str.EncodeOrdinaryTokens("Hello, Yak!")
// ```
func EncodeOrdinaryTokens(text string) []int {
	return ytoken.EncodeOrdinary(text)
}

// DecodeTokens 将 Qwen BPE token id 列表解码还原为文本，是 EncodeTokens 的逆操作。
// Example:
// ```
// ids = str.EncodeTokens("Hello, Yak!")
// text = str.DecodeTokens(ids) // Hello, Yak!
// ```
func DecodeTokens(tokens []int) string {
	return ytoken.Decode(tokens)
}
