package xhtml

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/html"
	"strings"
)

func _visitNode(node *html.Node, depth int, siblingIndex int, handle func(i *html.Node)) {

	if node == nil {
		return
	}
	handle(node)

	//prefix := strings.Repeat("  ", depth)

	switch node.Type {
	case html.CommentNode:
		log.Debugf("found comment: %s", node.Data)
	case html.DoctypeNode:
		// 一般符合标准的 HTML 头包含 Doctype 定义
		log.Debugf("skip doctype node: %s", node.Data)
	case html.DocumentNode:
		// Document 一般对应的是根文档
		log.Debugf("found docuemnt node: %s", node.Data)
	case html.ElementNode:
		var attrsVerbose []string
		for _, addr := range node.Attr {
			key := addr.Key
			if addr.Namespace != "" {
				key = fmt.Sprintf("%v:%v", addr.Namespace, addr.Key)
			}
			attrsVerbose = append(attrsVerbose, fmt.Sprintf("%v=\"%v\"", key, addr.Val))
		}
		//log.Infof("found element node: %s", node.Data)
		//println(
		//	prefix +
		//		node.Data +
		//		fmt.Sprintf(" [%v]", strings.Join(attrsVerbose, ", ")) +
		//		fmt.Sprintf(" XPATH: %v", GenerateXPath(node)))
	case html.TextNode:
		if strings.TrimSpace(node.Data) != "" {
			//println(prefix + "  TEXTNODE: " + fmt.Sprint(len(node.Data)))
		}
		//log.Infof("found text node: %s", node.Data)
	case html.RawNode:
		//log.Infof("raw node: %s", node.Data)
	default:
		//log.Infof("skip error node or unknown node: %s", node.Data)
	}
	_visitNode(node.FirstChild, depth+1, 0, handle)
	_visitNode(node.NextSibling, depth, siblingIndex+1, handle)
}

func Walker(h interface{}, handler func(node *html.Node)) error {
	raw := utils.InterfaceToBytes(h)
	var node, err = html.Parse(bytes.NewBuffer(raw))
	if err != nil {
		return utils.Errorf("parse html failed: %s", err)
	}
	_visitNode(node, 0, 0, handler)
	return nil
}
func WalkNode(node *html.Node, handler func(node *html.Node)) error {
	_visitNode(node, 0, 0, handler)
	return nil
}
