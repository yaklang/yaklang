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
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// nav = xpath.CreateXPathNavigator(doc)
// nav.MoveToChild()
// println(nav.String())
// ```
func CreateXPathNavigator(top *html.Node) *NodeNavigator {
	return &NodeNavigator{curr: top, root: top, attr: -1}
}

// Find 根据传入的 XPath 表达式从传入的节点开始查找匹配的节点，返回节点数组
// 如果表达式解析出错会 panic
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// nodes = xpath.Find(doc, "//div[@class='content']/text()")
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
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// node = xpath.FindOne(doc, "//div[@class='content']/text()")
// ```
func FindOne(top *html.Node, expr string) *html.Node {
	node, err := Query(top, expr)
	if err != nil {
		panic(err)
	}
	return node
}

// QueryAll 根据传入的 XPath 表达式从传入的节点开始查找匹配的节点，返回节点数组与错误
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// nodes, err = xpath.QueryAll(doc, "//div[@class='content']/text()")
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
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// node, err = xpath.Query(doc, "//div[@class='content']/text()")
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
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// node = xpath.FindOne(doc, "//div[@class='content']")
// text = xpath.InnerText(node)
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
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// node = xpath.FindOne(doc, "//div[@class='content']")
// attr = xpath.SelectAttr(node, "class")
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

// ExistsAttr 判断传入节点是否存在指定名称的属性并返回布尔值
// Example:
// ```
// doc, err = xpath.LoadHTMLDocument(htmlText)
// node = xpath.FindOne(doc, "//div[@class='content']")
// existed = xpath.ExistsAttr(node, "class") // true
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
