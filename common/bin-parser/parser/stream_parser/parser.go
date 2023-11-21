package stream_parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"os"
)

func init() {
	base.RegisterParser("default", &DefParser{})
}

func ParseRule(p string) (*base.Node, error) {
	ruleContent, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var ruleMap yaml.MapSlice
	err = yaml.Unmarshal(ruleContent, &ruleMap)
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
	absDir, err := utils.GetFileAbsDir(p)
	if err != nil {
		return nil, err
	}
	rootNode.Ctx.SetItem("path", absDir)
	return rootNode, nil
}
