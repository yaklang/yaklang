package base

import (
	"sync"
	"unicode"

	"github.com/yaklang/yaklang/common/utils"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// Plans describe construction, not mutable Nodes or a running VM. Map offsets
// refer to the invocation's cloned YAML, preserving aliases between Origin and
// nested config values within that invocation. Config/child operations stay in
// source order: a later endian assignment must not affect an earlier child.
type rulePlan struct {
	name       string
	terminal   []ConfigItem
	operations []rulePlanOperation
	children   int
	nodes      int
	prefix     *configPrefix
}
type rulePlanOperation struct {
	index int
	key   string
	child *rulePlan
}
type cachedRulePlan struct {
	document yaml.MapSlice
	plan     *rulePlan
	err      error
}

var rulePlanCache sync.Map

func compileRulePlan(name string, value any) (*rulePlan, error) {
	defaults := NewEmptyConfig()
	defaults.SetItems(ConfigItem{"endian", "big"}, ConfigItem{"parser", "default"})
	return compileRulePlanWithConfig(name, value, defaults)
}

func compileRulePlanWithConfig(name string, value any, parent *Config) (*rulePlan, error) {
	plan := &rulePlan{name: name, nodes: 1}
	cfg := NewConfig(parent)
	if data, ok := value.(yaml.MapSlice); ok {
		plan.operations = make([]rulePlanOperation, 0, len(data))
		for i, item := range data {
			key := utils.InterfaceToString(item.Key)
			op := rulePlanOperation{index: i, key: key}
			if len(key) == 0 || !unicode.IsLower(rune(key[0])) {
				var err error
				op.child, err = compileRulePlanWithConfig(key, item.Value, cfg)
				if err != nil {
					return nil, err
				}
				plan.children++
				plan.nodes += op.child.nodes
			} else {
				cfg.SetItem(key, item.Value)
			}
			plan.operations = append(plan.operations, op)
		}
		if plan.children == 0 {
			cfg.SetItem(CfgIsTerminal, true)
			plan.prefix = immutableRulePrefix(cfg.data)
		}
		return plan, nil
	}
	// Reuse the complete terminal grammar once, including its error text and
	// repeated options. YAML scalars cannot install function-valued callbacks.
	node, err := newNodeTree(NewEmptyConfig(), name, value, nil)
	if err != nil {
		return nil, err
	}
	store := node.Cfg.data
	if store.configStoreLegacy == nil {
		for _, w := range store.writes {
			if w.replay {
				plan.terminal = append(plan.terminal, ConfigItem{compactConfigKeys[w.key], w.value})
			}
		}
	} else if i, ok := store.findLocked(CfgOptionFuns); ok {
		for _, w := range store.entries[i].value.(*configReplay).writes {
			plan.terminal = append(plan.terminal, ConfigItem{w.key, w.value})
		}
	}
	cfg.SetItems(plan.terminal...)
	plan.prefix = immutableRulePrefix(cfg.data)
	return plan, nil
}

func (p *rulePlan) instantiate(parent *Config, origin any, ctx *NodeContext, batch *nodeBatch) *Node {
	if p.prefix != nil {
		return batch.newNodeWithPrefix(p.name, origin, ctx, p.prefix)
	}
	if _, ok := origin.(yaml.MapSlice); !ok {
		return batch.NewNode(p.name, origin, parent, ctx, p.terminal...)
	}
	node := batch.NewNode(p.name, origin, parent, ctx)
	if p.children > 0 {
		node.Children = make([]*Node, 0, p.children)
	}
	data := origin.(yaml.MapSlice)
	for _, op := range p.operations {
		value := data[op.index].Value
		if op.child == nil {
			node.Cfg.SetItem(op.key, value)
			continue
		}
		child := op.child.instantiate(node.Cfg, value, ctx, batch)
		node.Children = append(node.Children, child)
		if p.name == "Package" {
			child.Cfg.SetItem("package-child", true)
		}
	}
	if len(node.Children) == 0 {
		node.Cfg.SetItem(CfgIsTerminal, true)
	}
	return node
}

// Freeze only compact, scalar construction state. Collections, custom values
// and unknown keys use the ordinary builder, retaining Origin/config aliases.
// Only leaves use this prefix: parent configuration changes must still execute
// in source order so that earlier and later children inherit the right values.
func immutableRulePrefix(store *configStore) *configPrefix {
	if store.configStoreLegacy != nil || store.compactWriteCount() >= 65534 {
		return nil
	}
	p := &configPrefix{count: uint16(store.compactWriteCount()), positions: store.positions, order: store.order, orderLen: store.orderLen}
	for i := 0; i < int(p.count); i++ {
		w := store.compactWrite(i)
		if !w.replay {
			return nil
		}
		switch w.value.(type) {
		case nil, bool, string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		default:
			return nil
		}
		p.writes = append(p.writes, w)
	}
	return p
}

func (s *configStore) usePrefix(p *configPrefix) {
	s.prefix = p
	s.positions, s.order, s.orderLen, s.historyCount = p.positions, p.order, p.orderLen, p.count
}

func instantiateRuleDocument(path string, document yaml.MapSlice) (*Node, error) {
	var cached *cachedRulePlan
	if value, ok := rulePlanCache.Load(path); ok {
		candidate := value.(*cachedRulePlan)
		// Document-cache replacement (used by cache isolation tests) invalidates
		// the construction plan too. An empty document has no varying content.
		if len(document) == len(candidate.document) && (len(document) == 0 || &document[0] == &candidate.document[0]) {
			cached = candidate
		}
	}
	if cached == nil {
		plan, err := compileRulePlan("root", document)
		cached = &cachedRulePlan{document, plan, err}
		rulePlanCache.Store(path, cached)
	}
	if cached.err != nil {
		return nil, cached.err
	}
	defaults := NewEmptyConfig()
	defaults.SetItems(ConfigItem{"endian", "big"}, ConfigItem{"parser", "default"})
	ctx := &NodeContext{BaseKV{&configStore{}}}
	ctx.SetItem(ctxParserRuntimeMap, newParserRuntimeMap())
	root := cached.plan.instantiate(defaults, cloneRuleDocumentValue(document), ctx, NewNodeBatch(cached.plan.nodes))
	ctx.SetItem("root", root)
	root.Cfg.SetItem("isRoot", true)
	return root, nil
}
