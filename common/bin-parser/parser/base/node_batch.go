package base

import "strings"

type nodeConfigAllocation struct {
	node   Node
	config Config
	store  configStore
}

const nodeBatchConfigReserve = 6

// NewNodeBatch reserves independently owned nodes/configs for a single result
// tree. It is an allocation helper, not a pool: retained nodes keep their
// storage alive and are never reused by another parse. It must be used serially.
func NewNodeBatch(count int) *nodeBatch {
	return &nodeBatch{nodes: make([]nodeConfigAllocation, count), writes: make([]compactConfigWrite, count*nodeBatchConfigReserve)}
}

type nodeBatch struct {
	nodes  []nodeConfigAllocation
	writes []compactConfigWrite
	next   int
}

func (b *nodeBatch) NewNode(name string, origin any, parent *Config, ctx *NodeContext, items ...ConfigItem) *Node {
	if b.next == len(b.nodes) {
		return NewEmptyNode(name, origin, NewConfigWithItems(parent, items...), ctx)
	}
	i := b.next
	b.next++
	slot := &b.nodes[i]
	slot.config.data = &slot.store
	slot.store.writes = b.writes[i*nodeBatchConfigReserve : i*nodeBatchConfigReserve : (i+1)*nodeBatchConfigReserve]
	inherited, count := parent.data.inheritedItems()
	skip, used := slot.store.initializePrefix(inherited, count, items)
	if !used {
		for _, item := range inherited[:count] {
			slot.store.setConfigItemLocked(item.key, item.value, nodeBatchConfigReserve)
		}
	}
	for _, item := range items[skip:] {
		slot.store.setConfigItemLocked(item.Key, item.Value, nodeBatchConfigReserve)
	}

	slot.node = Node{Name: name, Origin: origin, Cfg: &slot.config, Ctx: ctx}
	return &slot.node
}

// A construction plan may share an immutable configuration prefix, but never
// its Node, Config, write storage, Origin, or publicly exposed history slice.
func (b *nodeBatch) newNodeWithPrefix(name string, origin any, ctx *NodeContext, prefix *configPrefix) *Node {
	if b.next == len(b.nodes) {
		cfg := NewEmptyConfig()
		cfg.data.usePrefix(prefix)
		return NewEmptyNode(name, origin, cfg, ctx)
	}
	i := b.next
	b.next++
	slot := &b.nodes[i]
	slot.config.data = &slot.store
	slot.store.writes = b.writes[i*nodeBatchConfigReserve : i*nodeBatchConfigReserve : (i+1)*nodeBatchConfigReserve]
	slot.store.usePrefix(prefix)
	slot.node = Node{Name: name, Origin: origin, Cfg: &slot.config, Ctx: ctx}
	return &slot.node
}

// NewNodeTreeWithConfigItems retains the ordinary description parser for
// complex types; bare leaves prepare all ordered assignments in one allocation.
func (b *nodeBatch) NewNodeTreeWithConfigItems(parent *Config, name string, data any, ctx *NodeContext, items ...ConfigItem) (*Node, error) {
	if typ, ok := data.(string); ok && !strings.ContainsAny(typ, ",;:") {
		var local [12]ConfigItem
		ordered := append(local[:0], ConfigItem{CfgIsTerminal, true})
		if strings.HasSuffix(typ, "...") {
			typ = strings.TrimSuffix(typ, "...")
			ordered = append(ordered, ConfigItem{CfgIsList, true})
		}
		ordered = append(ordered, ConfigItem{CfgType, typ})
		ordered = append(ordered, items...)
		return b.NewNode(name, data, parent, ctx, ordered...), nil
	}
	return NewNodeTreeWithConfigItems(parent, name, data, ctx, items...)
}

// NewNodeTreeWithTypeItems avoids repeatedly boxing immutable builtin type
// names when a decoder already supplies a string. Complex descriptions retain
// the original grammar and error path.
func (b *nodeBatch) NewNodeTreeWithTypeItems(parent *Config, name, typ string, ctx *NodeContext, items ...ConfigItem) (*Node, error) {
	for i, candidate := range configPrefixTypes {
		if i > 0 && candidate == typ {
			// The value in this immutable prefix is the same string as the
			// descriptor, shared only as an immutable scalar, never as a Node.
			origin := configPrefixes[0][0][i].writes[3].value
			var local [12]ConfigItem
			ordered := append(local[:0], ConfigItem{CfgIsTerminal, true}, ConfigItem{CfgType, origin})
			ordered = append(ordered, items...)
			return b.NewNode(name, origin, parent, ctx, ordered...), nil
		}
	}
	return b.NewNodeTreeWithConfigItems(parent, name, typ, ctx, items...)
}
