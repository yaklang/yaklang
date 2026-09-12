package parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"io"
	"path"
	"strings"
)

// inputBitLengthReader is an opt-in contract for callers that need rules to
// treat the supplied reader as an exact, bounded message. Plain bytes.Reader
// values intentionally keep the historical unbounded-root behavior.
type inputBitLengthReader interface {
	InputBitLength() uint64
}

func setKnownInputLength(rootNode *base.Node, data io.Reader) {
	reader, ok := data.(inputBitLengthReader)
	if !ok {
		return
	}
	rootNode.Cfg.SetItem(base.CfgLength, reader.InputBitLength())
}

func ParseBinaryWithConfig(data io.Reader, rule string, config map[string]any, keys ...string) (*base.Node, error) {
	splits := strings.Split(rule, ".")
	if len(splits) > 0 {
		splits[len(splits)-1] = splits[len(splits)-1] + ".yaml"
	}
	// Embedded RuleFS names use slash separators on every host OS.
	p := path.Join(splits...)
	rootNode, err := base.ParseRule(p)
	if err != nil {
		return nil, err
	}
	setKnownInputLength(rootNode, data)
	inputConfig := make(map[string]any, len(config))
	for k, v := range config {
		rootNode.Ctx.SetItem(k, v)
		inputConfig[k] = v
	}
	// Snapshot the key/value association, retaining intentionally shared state
	// objects (for example a caller-owned IPFIX template table).
	rootNode.Ctx.SetItem(base.CtxInputConfig, inputConfig)
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
	p := path.Join(splits...)
	rootNode, err := base.ParseRule(p)
	if err != nil {
		return nil, err
	}
	setKnownInputLength(rootNode, data)
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
	p := path.Join(splits...)
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
