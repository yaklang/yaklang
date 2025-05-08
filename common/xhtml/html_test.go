package xhtml

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/html"
)

func TestHtml(t *testing.T) {
	rootNode, err := html.Parse(strings.NewReader(testBody))
	if err != nil {
		panic(err)
	}
	WalkNode(rootNode, func(node *html.Node) {
		if len(node.Attr) > 0 {

		}
	})
}

func TestRandStr(t *testing.T) {
	for i := 0; i < 10; i++ {
		s := utils.RandStringBytes(5)
		println(s)
	}
}

func TestFind(t *testing.T) {
	nodeInfo := FindNodeFromHtml(testBody, "hacker123")
	for _, info := range nodeInfo {
		_ = info
		//println()
	}
	//a := make(chan int)
}
