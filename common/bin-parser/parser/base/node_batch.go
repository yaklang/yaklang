package base

import "strings"

type nodeConfigAllocation struct {
	node   Node
	config Config
	store  configStore
}

// NewNodeBatch reserves independently owned nodes/configs for a single result
// tree. It is an allocation helper, not a pool: retained nodes keep their
// storage alive and are never reused by another parse. It must be used serially.
func NewNodeBatch(count int) *nodeBatch {
	return &nodeBatch{nodes: make([]nodeConfigAllocation, count), writes: make([]compactConfigWrite, count*16)}
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
	slot.store.writes = b.writes[i*16 : i*16 : (i+1)*16]
	inherited, count := parent.data.inheritedItems()
	for _, item := range inherited[:count] {
		slot.store.setConfigItemLocked(item.key, item.value, 16)
	}
	for _, item := range items {
		slot.store.setConfigItemLocked(item.Key, item.Value, 16)
	}
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
