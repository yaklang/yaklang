/*
Package htmlquery provides extract data from HTML documents using XPath expression.
*/
package htmlquery

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/golang/groupcache/lru"

	"github.com/antchfx/xpath"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// DisableSelectorCache will disable caching for the query selector if value is true.
var DisableSelectorCache = false

// SelectorCacheMaxEntries allows how many selector object can be caching. Default is 50.
// Will disable caching if SelectorCacheMaxEntries <= 0.
var SelectorCacheMaxEntries = 50

var (
	cacheOnce  sync.Once
	cache      *lru.Cache
	cacheMutex sync.Mutex
)

func getQuery(expr string) (*xpath.Expr, error) {
	if DisableSelectorCache || SelectorCacheMaxEntries <= 0 {
		return xpath.Compile(expr)
	}
	cacheOnce.Do(func() {
		cache = lru.New(SelectorCacheMaxEntries)
	})
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	if v, ok := cache.Get(expr); ok {
		return v.(*xpath.Expr), nil
	}
	v, err := xpath.Compile(expr)
	if err != nil {
		return nil, err
	}
	cache.Add(expr, v)
	return v, nil
}

var _ xpath.NodeNavigator = &NodeNavigator{}

// CreateXPathNavigator 根据传入的节点创建一个新的 XPath 导航器，使用该导航器的方法来遍历该节点及其子节点
// 参数:
//   - top: 作为导航起点的节点
//
// 返回值:
//   - 可用于遍历节点树的 XPath 导航器
//
// Example:
// ```
// // VARS: 基于文档创建导航器
// doc = xpath.LoadHTMLDocument(`<div>x</div>`)~
// nav = xpath.CreateXPathNavigator(doc)
// // assert: 成功创建导航器
// assert nav != nil, "navigator should be created"
// ```
func CreateXPathNavigator(top *html.Node) *NodeNavigator {
	return &NodeNavigator{curr: top, root: top, attr: -1}
}

// Find 根据传入的 XPath 表达式从传入的节点开始查找匹配的节点，返回节点数组
// 如果表达式解析出错会 panic
// 参数:
//   - top: 查询的起始节点
//   - expr: XPath 表达式
//
// 返回值:
//   - 所有匹配节点组成的数组
//
// Example:
// ```
// // VARS: 查找所有匹配的 div
// doc = xpath.LoadHTMLDocument(`<div class="c">a</div><div class="c">b</div>`)~
// nodes = xpath.Find(doc, "//div[@class='c']")
// // STDOUT: 打印命中数量
// println(len(nodes))   // OUT: 2
// // assert: 锁定结论
// assert len(nodes) == 2, "should find two div nodes"
// ```
func Find(top *html.Node, expr string) []*html.Node {
	nodes, err := QueryAll(top, expr)
	if err != nil {
		panic(err)
	}
	return nodes
}

// FindOne 根据传入的 XPath 表达式从传入的节点开始查找第一个匹配的节点
// 如果表达式解析出错会 panic
// 参数:
//   - top: 查询的起始节点
//   - expr: XPath 表达式
//
// 返回值:
//   - 第一个匹配的节点，未匹配时为 nil
//
// Example:
// ```
// // VARS: 查找第一个 div
// doc = xpath.LoadHTMLDocument(`<div class="c">hello</div>`)~
// node = xpath.FindOne(doc, "//div[@class='c']")
// // STDOUT: 打印节点文本
// println(xpath.InnerText(node))   // OUT: hello
// // assert: 锁定结论
// assert xpath.InnerText(node) == "hello", "should find the div text"
// ```
func FindOne(top *html.Node, expr string) *html.Node {
	node, err := Query(top, expr)
	if err != nil {
		panic(err)
	}
	return node
}

// QueryAll 根据传入的 XPath 表达式从传入的节点开始查找匹配的节点，返回节点数组与错误
// 参数:
//   - top: 查询的起始节点
//   - expr: XPath 表达式
//
// 返回值:
//   - 所有匹配节点组成的数组
//   - 表达式解析失败时返回的错误
//
// Example:
// ```
// // VARS: 查询所有 div
// doc = xpath.LoadHTMLDocument(`<div>a</div><div>b</div>`)~
// nodes = xpath.QueryAll(doc, "//div")~
// // STDOUT: 打印命中数量
// println(len(nodes))   // OUT: 2
// // assert: 锁定结论
// assert len(nodes) == 2, "should query two div nodes"
// ```
func QueryAll(top *html.Node, expr string) ([]*html.Node, error) {
	exp, err := getQuery(expr)
	if err != nil {
		return nil, err
	}
	nodes := QuerySelectorAll(top, exp)
	return nodes, nil
}

// Query 根据传入的 XPath 表达式从传入的节点开始查找第一个匹配的节点，返回节点与错误
// 参数:
//   - top: 查询的起始节点
//   - expr: XPath 表达式
//
// 返回值:
//   - 第一个匹配的节点，未匹配时为 nil
//   - 表达式解析失败时返回的错误
//
// Example:
// ```
// // VARS: 查询第一个 div
// doc = xpath.LoadHTMLDocument(`<div>hello</div>`)~
// node = xpath.Query(doc, "//div")~
// // assert: 锁定结论
// assert xpath.InnerText(node) == "hello", "should query the first div"
// ```
func Query(top *html.Node, expr string) (*html.Node, error) {
	exp, err := getQuery(expr)
	if err != nil {
		return nil, err
	}
	return QuerySelector(top, exp), nil
}

// QuerySelector returns the first matched html.Node by the specified XPath selector.
func QuerySelector(top *html.Node, selector *xpath.Expr) *html.Node {
	t := selector.Select(CreateXPathNavigator(top))
	if t.MoveNext() {
		return GetCurrentNode(t.Current().(*NodeNavigator))
	}
	return nil
}

// QuerySelectorAll searches all of the html.Node that matches the specified XPath selectors.
func QuerySelectorAll(top *html.Node, selector *xpath.Expr) []*html.Node {
	var elems []*html.Node
	t := selector.Select(CreateXPathNavigator(top))
	for t.MoveNext() {
		nav := t.Current().(*NodeNavigator)
		n := GetCurrentNode(nav)
		elems = append(elems, n)
	}
	return elems
}

// LoadURL loads the HTML document from the specified URL.
func LoadURL(url string) (*html.Node, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	r, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	return html.Parse(r)
}

// LoadDoc loads the HTML document from the specified file path.
func LoadDoc(path string) (*html.Node, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return html.Parse(bufio.NewReader(f))
}

func GetCurrentNode(n *NodeNavigator) *html.Node {
	if n.NodeType() == xpath.AttributeNode {
		childNode := &html.Node{
			Type: html.TextNode,
			Data: n.Value(),
		}
		return &html.Node{
			Type:       html.ElementNode,
			Data:       n.LocalName(),
			FirstChild: childNode,
			LastChild:  childNode,
		}

	}
	return n.curr
}

// Parse returns the parse tree for the HTML from the given Reader.
func Parse(r io.Reader) (*html.Node, error) {
	return html.Parse(r)
}

// InnerText 返回指定节点及其子节点的字符串
// 参数:
//   - n: 要提取文本的节点
//
// 返回值:
//   - 节点及其子节点拼接后的纯文本
//
// Example:
// ```
// // VARS: 提取节点文本
// doc = xpath.LoadHTMLDocument(`<div>hello</div>`)~
// node = xpath.FindOne(doc, "//div")
// text = xpath.InnerText(node)
// // STDOUT: 打印文本
// println(text)   // OUT: hello
// // assert: 锁定结论
// assert text == "hello", "InnerText should extract node text"
// ```
func InnerText(n *html.Node) string {
	var output func(*bytes.Buffer, *html.Node)
	output = func(buf *bytes.Buffer, n *html.Node) {
		switch n.Type {
		case html.TextNode:
			buf.WriteString(n.Data)
			return
		case html.CommentNode:
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			output(buf, child)
		}
	}

	var buf bytes.Buffer
	output(&buf, n)
	return buf.String()
}

// SelectAttr 返回传入节点指定名称的属性值
// 参数:
//   - n: 目标节点
//   - name: 属性名
//
// 返回值:
//   - 属性值，不存在时返回空字符串
//
// Example:
// ```
// // VARS: 读取 class 属性
// doc = xpath.LoadHTMLDocument(`<div class="content">x</div>`)~
// node = xpath.FindOne(doc, "//div")
// attr = xpath.SelectAttr(node, "class")
// // STDOUT: 打印属性值
// println(attr)   // OUT: content
// // assert: 锁定结论
// assert attr == "content", "SelectAttr should read the class attribute"
// ```
func SelectAttr(n *html.Node, name string) (val string) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode && n.Parent == nil && name == n.Data {
		return InnerText(n)
	}
	for _, attr := range n.Attr {
		if attr.Key == name {
			val = attr.Val
			break
		}
	}
	return
}

// ExistedAttr 判断传入节点是否存在指定名称的属性并返回布尔值
// 参数:
//   - n: 目标节点
//   - name: 属性名
//
// 返回值:
//   - 节点是否存在该属性
//
// Example:
// ```
// // VARS: 判断属性是否存在
// doc = xpath.LoadHTMLDocument(`<div class="content">x</div>`)~
// node = xpath.FindOne(doc, "//div")
// // STDOUT: class 属性存在
// println(xpath.ExistedAttr(node, "class"))   // OUT: true
// // assert: 不存在的属性返回 false
// assert xpath.ExistedAttr(node, "id") == false, "missing attribute should report false"
// ```
func ExistsAttr(n *html.Node, name string) bool {
	if n == nil {
		return false
	}
	for _, attr := range n.Attr {
		if attr.Key == name {
			return true
		}
	}
	return false
}

// OutputHTML returns the text including tags name.
func OutputHTML(n *html.Node, self bool) string {
	var buf bytes.Buffer
	if self {
		html.Render(&buf, n)
	} else {
		for n := n.FirstChild; n != nil; n = n.NextSibling {
			html.Render(&buf, n)
		}
	}
	return buf.String()
}

type NodeNavigator struct {
	root, curr *html.Node
	attr       int
}

func (h *NodeNavigator) Current() *html.Node {
	return GetCurrentNode(h)
}

func (h *NodeNavigator) NodeType() xpath.NodeType {
	switch h.curr.Type {
	case html.CommentNode:
		return xpath.CommentNode
	case html.TextNode:
		return xpath.TextNode
	case html.DocumentNode:
		return xpath.RootNode
	case html.ElementNode:
		if h.attr != -1 {
			return xpath.AttributeNode
		}
		return xpath.ElementNode
	case html.DoctypeNode:
		// ignored <!DOCTYPE HTML> declare and as Root-Node type.
		return xpath.RootNode
	}
	panic(fmt.Sprintf("unknown HTML node type: %v", h.curr.Type))
}

func (h *NodeNavigator) LocalName() string {
	if h.attr != -1 {
		return h.curr.Attr[h.attr].Key
	}
	return h.curr.Data
}

func (*NodeNavigator) Prefix() string {
	return ""
}

func (h *NodeNavigator) Value() string {
	switch h.curr.Type {
	case html.CommentNode:
		return h.curr.Data
	case html.ElementNode:
		if h.attr != -1 {
			return h.curr.Attr[h.attr].Val
		}
		return InnerText(h.curr)
	case html.TextNode:
		return h.curr.Data
	}
	return ""
}

func (h *NodeNavigator) Copy() xpath.NodeNavigator {
	n := *h
	return &n
}

func (h *NodeNavigator) MoveToRoot() {
	h.curr = h.root
}

func (h *NodeNavigator) MoveToParent() bool {
	if h.attr != -1 {
		h.attr = -1
		return true
	} else if node := h.curr.Parent; node != nil {
		h.curr = node
		return true
	}
	return false
}

func (h *NodeNavigator) MoveToNextAttribute() bool {
	if h.attr >= len(h.curr.Attr)-1 {
		return false
	}
	h.attr++
	return true
}

func (h *NodeNavigator) MoveToChild() bool {
	if h.attr != -1 {
		return false
	}
	if node := h.curr.FirstChild; node != nil {
		h.curr = node
		return true
	}
	return false
}

func (h *NodeNavigator) MoveToFirst() bool {
	if h.attr != -1 || h.curr.PrevSibling == nil {
		return false
	}
	for {
		node := h.curr.PrevSibling
		if node == nil {
			break
		}
		h.curr = node
	}
	return true
}

func (h *NodeNavigator) String() string {
	return h.Value()
}

func (h *NodeNavigator) MoveToNext() bool {
	if h.attr != -1 {
		return false
	}
	if node := h.curr.NextSibling; node != nil {
		h.curr = node
		return true
	}
	return false
}

func (h *NodeNavigator) MoveToPrevious() bool {
	if h.attr != -1 {
		return false
	}
	if node := h.curr.PrevSibling; node != nil {
		h.curr = node
		return true
	}
	return false
}

func (h *NodeNavigator) MoveTo(other xpath.NodeNavigator) bool {
	node, ok := other.(*NodeNavigator)
	if !ok || node.root != h.root {
		return false
	}

	h.curr = node.curr
	h.attr = node.attr
	return true
}
