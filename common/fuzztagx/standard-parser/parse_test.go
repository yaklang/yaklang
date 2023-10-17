package standard_parser

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestSearch(t *testing.T) {
	res := IndexAllSubstrings("ababac", "aba")
	for _, pos := range res {
		fmt.Printf("%d: %v\n", pos[0], pos[1])
	}
}
func TestGenerator(t *testing.T) {
	nodes, err := Parse("aaa{{int(a)}}aa", NewTagDefine("fuzztag", "{{", "}}", &FuzzTag{}))
	if err != nil {
		t.Fatal(err)
	}
	generator := NewGenerator(nodes, map[string]TagMethod{
		"int": func(s string) ([]FuzzResult, error) {
			return []FuzzResult{FuzzResult(s)}, nil
		},
	})
	for {
		if ok, err := generator.Generate(); ok {
			if err != nil {
				t.Fatal(err)
			}
			println(generator.Result())
		} else {
			break
		}
	}
}
func TestNewTagDefine(t *testing.T) {
	tagDefine := NewTagDefine("fuzztag", "=>", "<=", &FuzzTag{})
	tag1 := tagDefine.NewTag()
	tag1.AddData(StringNode("aa"))
	tag2 := tagDefine.NewTag()
	spew.Dump(tag1, tag2)
}
