package base

import (
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/rules"
	"github.com/yaklang/yaklang/common/utils"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

type NodeConfigFun func(config *Config)

var parseMap = make(map[string]Parser)

// RuleFS is immutable after it is embedded into the binary. Cache only the
// decoded YAML document; every ParseRule call clones it before building a fresh
// node tree, configuration set and context. Node.Origin and nested config values
// are exported and mutable, so they must never expose the cached document.
var ruleDocumentCache sync.Map

// cloneRuleDocumentValue copies the mutable types emitted by orderedyaml's
// decoder. YAML mappings (including mapping keys) become MapSlice, sequences
// become []any, and scalar values can be shared safely.
func cloneRuleDocumentValue(value any) any {
	switch value := value.(type) {
	case yaml.MapSlice:
		if value == nil {
			return yaml.MapSlice(nil)
		}
		result := make(yaml.MapSlice, len(value))
		for i, item := range value {
			result[i] = yaml.MapItem{
				Key:   cloneRuleDocumentValue(item.Key),
				Value: cloneRuleDocumentValue(item.Value),
			}
		}
		return result
	case []any:
		if value == nil {
			return []any(nil)
		}
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = cloneRuleDocumentValue(item)
		}
		return result
	default:
		return value
	}
}

func RegisterParser(name string, parser Parser) {
	parseMap[name] = parser
}

type Parser interface {
	Parse(data *BitReader, node *Node) error
	Generate(data any, node *Node) error
	OnRoot(node *Node) error
	Result(node *Node) (*NodeValue, error)
}
type BaseParser struct {
	root *Node
}

func ParseRule(p string) (*Node, error) {
	var ruleMap yaml.MapSlice
	if cached, ok := ruleDocumentCache.Load(p); ok {
		ruleMap = cached.(yaml.MapSlice)
	} else {
		ruleContent, err := rules.RuleFS.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(ruleContent, &ruleMap); err != nil {
			return nil, err
		}
		ruleDocumentCache.Store(p, ruleMap)
	}
	rootNode, err := NewNodeTree(cloneRuleDocumentValue(ruleMap).(yaml.MapSlice))
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
