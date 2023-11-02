package parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"io"
	"path/filepath"
	"strings"
)

func ParseBinaryWithConfig(data io.Reader, rule string, config map[string]any, keys ...string) (*base.Node, error) {
	splits := strings.Split(rule, ".")
	if len(splits) > 0 {
		splits[len(splits)-1] = splits[len(splits)-1] + ".yaml"
	}
	p := filepath.Join(splits...)
	rootNode, err := base.ParseRule(p)
	if err != nil {
		return nil, err
	}
	for k, v := range config {
		rootNode.Ctx.SetItem(k, v)
	}
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		err = rootNode.Parse(base.NewBitReader(data))
		if err != nil {
			return nil, err
		}
		return rootNode, err
	} else {
		err = rootNode.ParseSubNode(base.NewBitReader(data), strings.Join(keys, "."))
		if err != nil {
			return nil, err
		}
		return base.GetNodeByPath(rootNode, "@"+strings.Join(keys, ".")), nil
	}
}
func ParseBinary(data io.Reader, rule string, keys ...string) (*base.Node, error) {
	splits := strings.Split(rule, ".")
	if len(splits) > 0 {
		splits[len(splits)-1] = splits[len(splits)-1] + ".yaml"
	}
	p := filepath.Join(splits...)
	rootNode, err := base.ParseRule(p)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		err = rootNode.Parse(base.NewBitReader(data))
		if err != nil {
			return nil, err
		}
		return rootNode, err
	} else {
		err = rootNode.ParseSubNode(base.NewBitReader(data), strings.Join(keys, "."))
		if err != nil {
			return nil, err
		}
		return base.GetNodeByPath(rootNode, "@"+strings.Join(keys, ".")), nil
	}
}

func GenerateBinary(data any, rule string, keys ...string) (*base.Node, error) {
	splits := strings.Split(rule, ".")
	if len(splits) > 0 {
		splits[len(splits)-1] = splits[len(splits)-1] + ".yaml"
	}
	p := filepath.Join(splits...)
	rootNode, err := base.ParseRule(p)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		err = rootNode.Generate(data)
		if err != nil {
			return nil, err
		}
		return rootNode, err
	} else {
		err = rootNode.GenerateSubNode(data, strings.Join(keys, "."))
		if err != nil {
			return nil, err
		}
		return base.GetNodeByPath(rootNode, "@"+strings.Join(keys, ".")), nil
	}
}
