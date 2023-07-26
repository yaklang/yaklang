package fuzztagx

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

func DumpResult(data []Node) string {
	var dumpNodes func(tag []Node) string
	dumpNodes = func(nodes []Node) string {
		res := ""
		for _, inode := range nodes {
			switch node := inode.(type) {
			case *StringNode, *ExpressionNode:
				res += strings.Join(node.Strings(), "")
			case *Tag:
				tagName := "tag"
				if node.IsExpTag {
					tagName = "exp"
				}
				res += fmt.Sprintf(" %s{%s} ", tagName, dumpNodes(node.Nodes))
			case *FuzzTagMethod:
				res += fmt.Sprintf("%s(%s)", node.name, dumpNodes(node.params))
			}
		}
		return res
	}

	res := ""
	for _, d := range data {
		switch ret := d.(type) {
		case *StringNode:
			res += strings.Join(ret.Strings(), "")
		case *Tag:
			tagName := "tag"
			if ret.IsExpTag {
				tagName = "exp"
			}
			res += fmt.Sprintf(" %s{%s} ", tagName, dumpNodes(ret.Nodes))
		}
	}
	return res
}
func TestParse(t *testing.T) {
	for _, testCase := range [][2]string{
		//{
		//	"{{int::1({{list(aaa|ccc)}})}}{{int::1({{list(aaa|ccc|ddd)}})}}",
		//	"a",
		//},
		{
			"asd{{{ int({{ int(1) }})  }}}}}{}",
			"asd{ tag{int( tag{int(1)} )} }}}{}",
		},
		{
			"asd{{{= int({{= int(1) }})  }}}}}{}",
			"asd{ exp{ int({{= int(1) } )  }}}}}{}",
		},
		{
			"{{ " +
				"" +
				"int({{=1+1}}-{{=1+3}}) }}",
			" tag{int( exp{1+1} - exp{1+3} )} ",
		},
		{
			"{{{int(a)}{{int(a)}}",
			"{{{int(a)} tag{int(a)} ",
		},
		{
			"{{int({{int(1)}}{{int(2)}})}}",
			" tag{int( tag{int(1)}  tag{int(2)} )} ",
		},
	} {
		res, err := Parse(testCase[0], nil)
		if err != nil {
			panic(utils.Errorf("test data [%v] error: %v", testCase[0], err))
		}
		r := DumpResult(res)
		spew.Dump(r)
		if r != testCase[1] {
			panic(utils.Errorf("test data failed, expect: %v, got: %v", testCase[1], r))
		}
	}
}
