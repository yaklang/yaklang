package parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	_default "github.com/yaklang/yaklang/common/bin-parser/parser/default"
	"gopkg.in/yaml.v2"
)

func init() {
	base.RegisterParser("default", &_default.DefParser{})
}

func ParseRule(ruleContent []byte) (*base.Node, error) {
	var ruleMap yaml.MapSlice
	err := yaml.Unmarshal(ruleContent, &ruleMap)
	if err != nil {
		return nil, err
	}
	rootNode, err := base.NewNodeTree(ruleMap)
	if err != nil {
		return nil, err
	}
	if !rootNode.Cfg.Has("parser") {
		rootNode.Cfg.SetItem("parser", "default")
	}
	return rootNode, nil
}
