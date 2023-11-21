package parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"gopkg.in/yaml.v2"
)

type Session struct {
	Cfg   *base.Config
	rules []*base.Node
}

func ParseSession(ruleContent []byte) (*base.Node, error) {
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
