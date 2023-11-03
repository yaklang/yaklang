package bin_parser

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"io"
	"os"
)

func NewDescriptorByRule(name string, rule any, opts []ConfigFunc) (*binx.PartDescriptor, error) {
	switch ret := rule.(type) {
	case string:
		opts1, err := parseTerminalNode(ret)
		if err != nil {
			return nil, fmt.Errorf("rule `%s` is invalid: %w", name, err)
		}
		cfg := NewConfig(append(opts, opts1...))
		desc := binx.NewPartDescriptor(cfg.dataType, cfg.length)
		desc.Identifier = name
		desc.ByteOrder = cfg.endian
		return desc, nil
	case yaml.MapItem:
		return NewDescriptorByRule(name, ret.Value, opts)
	case yaml.MapSlice:
		var desc = binx.NewListDescriptor()
		desc.SetIdentifier(name)
		desc.SubPartLength = uint64(len(ret))
		for _, v := range ret {
			opts, node := splitConfigAndNode(v.Value)
			subDesc, err := NewDescriptorByRule(utils.InterfaceToString(v.Key), node, opts)
			if err != nil {
				return nil, err
			}
			desc.SubPartDescriptor = append(desc.SubPartDescriptor, subDesc)
		}
		return desc, nil
	case []string:
		var desc = binx.NewListDescriptor()
		for index, v := range ret {
			subDesc, err := NewDescriptorByRule(fmt.Sprintf("%s_%d", name, index), v, opts)
			if err != nil {
				return nil, err
			}
			desc.SubPartDescriptor = append(desc.SubPartDescriptor, subDesc)
		}
		return desc, nil
	default:
		return nil, errors.New("rule `" + name + "` is invalid")
	}
}
func Parse(data io.Reader, rule string) (binx.ResultIf, error) {
	ruleContent, err := os.ReadFile("./rules/" + rule + ".yaml")
	if err != nil {
		return nil, err
	}
	var ruleMap yaml.MapSlice
	err = yaml.Unmarshal(ruleContent, &ruleMap)
	if err != nil {
		return nil, err
	}
	config, ruleMap1 := splitConfigAndNode(ruleMap)
	desc, err := NewDescriptorByRule("package", ruleMap1, config)
	if err != nil {
		return nil, err
	}

	result, err := binx.BinaryRead(data, desc)
	if err != nil {
		return nil, err
	}
	if len(result) != 1 {
		return nil, errors.New("result length is not 1")
	}
	return result[0], nil
}
