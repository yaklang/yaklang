package xhtml

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/html"
	"strings"
)

type OutputPosType string

const (
	Tag       OutputPosType = "Tag"
	Text      OutputPosType = "Text"
	Attr      OutputPosType = "Attr"
	AttrHref  OutputPosType = "AttrHref"
	AttrOnxxx OutputPosType = "AttrOnxxx"
)

const (
	StructLarger  = "node1 contains node2"
	StructSmaller = "node2 contains node1"
)

type DiffInfo struct {
	OriginRaw string
	FuzzRaw   string
	XpathPos  string
	Reason    string
	Type      OutputPosType
	Node      *html.Node
}

/*
if diffInfo.Type == Attr {
	diffInfo.Node.Attr
}
*/

func node2str(node *html.Node) string {
	if node == nil {
		return ""
	}
	var rendered bytes.Buffer
	_ = html.Render(&rendered, node)
	return rendered.String()
}
func CompareHtml(htmlRaw1 interface{}, htmlRaw2 interface{}) ([]*DiffInfo, error) {
	//diff := []([]*html.Node){}
	diff := []*DiffInfo{}
	htmlRaw1s := utils.InterfaceToString(htmlRaw1)
	htmlRaw2s := utils.InterfaceToString(htmlRaw2)
	node1, err := html.Parse(strings.NewReader(htmlRaw1s))
	if err != nil {
		return nil, err
	}
	node2, err := html.Parse(strings.NewReader(htmlRaw2s))
	if err != nil {
		return nil, err
	}
	//广度
	checkEnd := func(cnode1 **html.Node, cnode2 **html.Node) bool {
		cnode1x := *cnode1
		cnode2x := *cnode2
		if cnode1x == nil && cnode2x == nil {
			return true
		} else if cnode1x != nil && cnode2x == nil {
			xpath := GenerateXPath(cnode1x.Parent)
			diff = append(diff, &DiffInfo{XpathPos: xpath, Reason: fmt.Sprint(StructLarger), Type: Tag, OriginRaw: node2str(cnode1x), FuzzRaw: node2str(cnode2x)})
			return true
		} else if cnode1x == nil && cnode2x != nil {
			xpath := GenerateXPath(cnode1x.Parent)
			diff = append(diff, &DiffInfo{XpathPos: xpath, Reason: fmt.Sprint(StructSmaller), Type: Tag, OriginRaw: node2str(cnode1x), FuzzRaw: node2str(cnode2x)})
			return true
		} else if cnode1x.Type != cnode2x.Type {
			return false
		} else if cnode1x.Type == html.ElementNode && cnode2x.Type == html.ElementNode && len(cnode1x.Attr) == len(cnode2x.Attr) {
			for i := 0; i < len(cnode1x.Attr); i++ {
				if cnode1x.Attr[i].Val != cnode2x.Attr[i].Val || cnode1x.Attr[i].Key != cnode2x.Attr[i].Key {
					xpath := GenerateXPath(cnode1x)
					xpath += fmt.Sprintf("/@%s", cnode1x.Attr[i].Key)
					diff = append(diff, &DiffInfo{XpathPos: xpath, Reason: fmt.Sprint("attrbutes is different"), Type: Attr, OriginRaw: node2str(cnode1x), FuzzRaw: node2str(cnode2x)})
					return true
				}
			}
		} else if cnode1x.Data != cnode2x.Data {
			var reason string
			switch cnode1x.Type {
			case html.ElementNode:
				reason = "tag is different"
				diff = append(diff, &DiffInfo{XpathPos: GenerateXPath(cnode2x), Reason: reason, Type: Tag, OriginRaw: node2str(cnode1x), FuzzRaw: node2str(cnode2x)})
			case html.CommentNode:
				reason = "comment is different"
				diff = append(diff, &DiffInfo{XpathPos: GenerateXPath(cnode2x.Parent) + "/comment()", Reason: reason, Type: Text, OriginRaw: node2str(cnode1x), FuzzRaw: node2str(cnode2x)})
			case html.TextNode:
				reason = "text is different"
				diff = append(diff, &DiffInfo{XpathPos: GenerateXPath(cnode2x.Parent) + "/text()", Reason: reason, Type: Text, OriginRaw: node2str(cnode1x), FuzzRaw: node2str(cnode2x)})
			}

			//diff = append(diff, []*html.Node{node1w, node2w})
			*cnode1 = cnode2x.NextSibling
			*cnode2 = cnode2x.NextSibling
			return true
		}
		return false
	}
	var walkNode func(wnode1 *html.Node, wnode2 *html.Node)
	walkNode = func(wnode1 *html.Node, wnode2 *html.Node) {
		if checkEnd(&wnode1, &wnode2) {
			return
		}
		walkNode(wnode1.FirstChild, wnode2.FirstChild)
		walkNode(wnode1.NextSibling, wnode2.NextSibling)
	}
	walkNode(node1, node2)
	return diff, nil
}
