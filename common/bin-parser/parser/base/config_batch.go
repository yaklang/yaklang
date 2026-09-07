package base

import "strings"

// ConfigItem is one ordered assignment, including assignments of present nil.
type ConfigItem struct {
	Key   string
	Value any
}

// SetItems has the same ordered writes and history as consecutive SetItem calls,
// with one lock for the batch. It does not execute callbacks. An invalid history
// still panics after writing the first affected item; later items are not written.
func (c *Config) SetItems(items ...ConfigItem) {
	c.data.setConfigItems(items)
}

// NewConfigWithItems inherits endian/parser/unit, then applies ordered items.
// The new store is private until return, so construction needs no write locks.
// Mutable values retain the same shallow sharing as NewConfig and SetItem.
func NewConfigWithItems(parent *Config, items ...ConfigItem) *Config {
	inherited, count := parent.data.inheritedItems()
	res := NewEmptyConfig()
	// Keep growth headroom: InitNode and caller state append more assignments
	// after construction. Exact sizing caused another allocation immediately
	// after many small configs, and enlarged the total allocated byte count.
	skip, used := res.data.initializePrefix(inherited, count, items)
	remaining := count + len(items)
	if used {
		remaining = len(items) - skip
	}
	capacity := 4
	for capacity < remaining {
		capacity *= 2
	}
	if !used {
		for _, item := range inherited[:count] {
			res.data.setConfigItemLocked(item.key, item.value, capacity)
		}
	}
	for _, item := range items[skip:] {
		res.data.setConfigItemLocked(item.Key, item.Value, capacity)
	}

	return res
}

// NewNodeTreeWithConfigItems applies items after the node description. Bare
// leaves can prepare their entire config together. Complex descriptions keep
// the general parser and its errors; no partial node is returned on failure.
func NewNodeTreeWithConfigItems(parent *Config, name string, data any, ctx *NodeContext, items ...ConfigItem) (*Node, error) {
	if typ, ok := data.(string); ok && !strings.ContainsAny(typ, ",;:") {
		var local [12]ConfigItem
		ordered := append(local[:0], ConfigItem{CfgIsTerminal, true})
		if strings.HasSuffix(typ, "...") {
			typ = strings.TrimSuffix(typ, "...")
			ordered = append(ordered, ConfigItem{CfgIsList, true})
		}
		ordered = append(ordered, ConfigItem{CfgType, typ})
		ordered = append(ordered, items...)
		return NewEmptyNode(name, data, NewConfigWithItems(parent, ordered...), ctx), nil
	}
	node, err := newNodeTree(parent, name, data, ctx)
	if err != nil {
		return nil, err
	}
	node.Cfg.SetItems(items...)
	return node, nil
}
