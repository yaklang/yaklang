package xml_tools

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/utils/yakxml"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/net/html/charset"
)

// Escape 对输入进行 XML 转义，把 < > & 等特殊字符替换为实体
// 参数:
//   - s: 待转义的内容(字符串或字节切片)
//
// 返回值:
//   - 转义后的字符串
//
// Example:
// ```
// // VARS: 转义尖括号
// result = xml.Escape("<a>")
// // STDOUT: 打印转义结果
// println(result)   // OUT: &lt;a&gt;
// // assert: 锁定结论
// assert result == "&lt;a&gt;", "Escape should encode angle brackets"
// ```
func XmlEscape(s []byte) string {
	var w strings.Builder
	yakxml.Escape(&w, s)
	return w.String()
}

type XmlDumpConfig struct {
	escapeHTML bool
}

type XmlDumpOptions func(*XmlDumpConfig)

// escape 生成一个 dumps 配置项，控制序列化时是否对 HTML 特殊字符进行转义
// 参数:
//   - escape: true 表示转义 HTML 特殊字符，false 表示保持原样
//
// 返回值:
//   - 可传给 xml.dumps 的配置项
//
// Example:
// ```
// // 关闭 HTML 转义后序列化(结果为 XML 文本，作示意)
// data = xml.dumps({"a": "<b>"}, xml.escape(false))
// println(string(data))
// ```
func WithHTMLEscape(escape bool) XmlDumpOptions {
	return func(c *XmlDumpConfig) {
		c.escapeHTML = escape
	}
}

func NewXmlDumpConfig() *XmlDumpConfig {
	return &XmlDumpConfig{
		escapeHTML: true,
	}
}

// dumps 将一个对象(通常是 map)序列化为 XML 格式的字节切片
// 参数:
//   - v: 待序列化的对象，可为 map 或有序映射
//   - opts: 可选配置项，如 xml.escape(false) 关闭 HTML 转义
//
// 返回值:
//   - 序列化后的 XML 字节切片
//
// Example:
// ```
// // VARS: 把 map 序列化为 XML
// out = xml.dumps({"name": "yak"})
// text = string(out)
// // assert: 输出包含对应元素
// assert str.Contains(text, "<name>yak</name>"), "dumps should encode the map as xml element"
// ```
func XmlDumps(v interface{}, opts ...XmlDumpOptions) []byte {
	config := NewXmlDumpConfig()
	for _, opt := range opts {
		opt(config)
	}
	var b bytes.Buffer
	var data *orderedmap.OrderedMap
	switch ret := v.(type) {
	case orderedmap.OrderedMap:
		data = &ret
	case *orderedmap.OrderedMap:
		data = ret
	default:
		data = orderedmap.New(utils.InterfaceToGeneralMap(v))
	}
	enc := yakxml.NewEncoderWithEscape(&b, config.escapeHTML)
	enc.Indent("", "  ")
	err := enc.Encode(data)
	if err != nil {
		log.Errorf("xml encode error: %v", err)
	}

	return b.Bytes()
}

func XmlLoadsOmap(v interface{}) (*orderedmap.OrderedMap, error) {
	i := orderedmap.New()
	buf := bytes.NewBufferString(fmt.Sprintf("<root>%s</root>", utils.InterfaceToString(v)))
	decoder := yakxml.NewDecoder(buf)
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		e, _ := charset.Lookup(label)
		if e != nil {
			return e.NewDecoder().Reader(input), nil
		}
		return input, nil // default to utf-8
	}
	err := decoder.Decode(&i)
	return i, err
}

// loads 将 XML 文本解析为嵌套的 map 结构
// 参数:
//   - v: 待解析的 XML 内容(字符串或字节切片)
//
// 返回值:
//   - 解析得到的 map，键为元素名，值为元素内容或子结构
//
// Example:
// ```
// // VARS: 解析单个元素
// m = xml.loads("<name>yak</name>")
// // STDOUT: 打印解析出的元素文本
// println(m["name"])   // OUT: yak
// // assert: 锁定结论
// assert m["name"] == "yak", "loads should parse the element text"
// ```
func XmlLoads(v interface{}) map[string]any {
	i, err := XmlLoadsOmap(v)
	if err != nil {
		log.Debugf("xml decode error: %v", err)
	}
	return i.ToStringMap()
}

// Prettify 将 XML 内容重新格式化为带缩进的可读形式
// 参数:
//   - b: 待格式化的 XML 内容(字符串或字节切片)
//
// 返回值:
//   - 带缩进换行的格式化 XML 字符串
//
// Example:
// ```
// // VARS: 美化压缩的 XML
// result = xml.Prettify("<root><name>yak</name></root>")
// // assert: 格式化后仍包含原有元素(多行输出用 Contains 判断)
// assert str.Contains(result, "<name>yak</name>"), "prettify should keep the element"
// ```
func XmlPrettify(b []byte) string {
	v, _ := XmlLoadsOmap(b)
	return string(XmlDumps(v, WithHTMLEscape(false)))
}
