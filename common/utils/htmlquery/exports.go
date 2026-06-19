package htmlquery

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"golang.org/x/net/html"
)

// LoadHTMLDocument 解析传入的 HTML 文本，返回根节点结构体引用与错误
// 参数:
//   - htmlText: 待解析的 HTML 文本(字符串或字节切片)
//
// 返回值:
//   - 解析得到的根节点
//   - 解析失败时返回的错误
//
// Example:
// ```
// // VARS: 解析 HTML 并查询
// doc = xpath.LoadHTMLDocument(`<div class="content">hello</div>`)~
// node = xpath.FindOne(doc, "//div")
// // assert: 文档可被正常查询
// assert xpath.InnerText(node) == "hello", "loaded document should be queryable"
// ```
func LoadHTMLDocument(htmlText any) (*html.Node, error) {
	return Parse(strings.NewReader(codec.AnyToString(htmlText)))
}

// OutputHTML 将传入的节点结构体引用转换为 HTML 文本(不含节点自身标签，仅子节点)
// 参数:
//   - doc: 要渲染的节点
//
// 返回值:
//   - 渲染出的 HTML 文本(节点内部内容)
//
// Example:
// ```
// // VARS: 渲染节点内部 HTML
// doc = xpath.LoadHTMLDocument(`<div class="c">hello</div>`)~
// node = xpath.FindOne(doc, "//div")
// // STDOUT: 打印内部内容
// println(xpath.OutputHTML(node))   // OUT: hello
// // assert: 锁定结论
// assert xpath.OutputHTML(node) == "hello", "OutputHTML should render the inner content"
// ```
func outputHTML(doc *html.Node) string {
	return OutputHTML(doc, false)
}

// OutputHTMLSelf 将传入的节点结构体引用转换为 HTML 文本，包含自身节点
// 参数:
//   - doc: 要渲染的节点
//
// 返回值:
//   - 渲染出的 HTML 文本(包含节点自身的标签)
//
// Example:
// ```
// // VARS: 渲染包含自身标签的 HTML
// doc = xpath.LoadHTMLDocument(`<div class="c">hello</div>`)~
// node = xpath.FindOne(doc, "//div")
// out = xpath.OutputHTMLSelf(node)
// // assert: 输出包含节点自身标签
// assert str.Contains(out, "<div"), "OutputHTMLSelf should include the node tag itself"
// ```
func outputHTMLSelf(doc *html.Node) string {
	return OutputHTML(doc, true)
}

var Exports = map[string]interface{}{
	"LoadHTMLDocument":     LoadHTMLDocument,
	"Find":                 Find,
	"FindOne":              FindOne,
	"QueryAll":             QueryAll,
	"Query":                Query,
	"InnerText":            InnerText,
	"SelectAttr":           SelectAttr,
	"ExistedAttr":          ExistsAttr,
	"CreateXPathNavigator": CreateXPathNavigator,

	"OutputHTML":     outputHTML,
	"OutputHTMLSelf": outputHTMLSelf,
}
