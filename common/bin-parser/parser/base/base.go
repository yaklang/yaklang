package base

import (
	"github.com/yaklang/yaklang/common/bin-parser/rules"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
)

type NodeConfigFun func(config *Config)

var parseMap = make(map[string]Parser)

func RegisterParser(name string, parser Parser) {
	parseMap[name] = parser
}

type Parser interface {
	Parse(data *BitReader, node *Node) error
	Generate(data any, node *Node) error
	OnRoot(node *Node) error
	Result(node *Node) (any, error)
}
type BaseParser struct {
	root *Node
}

func ParseRule(p string) (*Node, error) {
	ruleContent, err := rules.RuleFS.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var ruleMap yaml.MapSlice
	err = yaml.Unmarshal(ruleContent, &ruleMap)
	if err != nil {
		return nil, err
	}
	rootNode, err := NewNodeTree(ruleMap)
	if err != nil {
		return nil, err
	}
	if !rootNode.Cfg.Has("parser") {
		rootNode.Cfg.SetItem("parser", "default")
	}
	absDir, err := utils.GetFileAbsDir(p)
	if err != nil {
		return nil, err
	}
	rootNode.Ctx.SetItem("path", absDir)
	return rootNode, nil
}
