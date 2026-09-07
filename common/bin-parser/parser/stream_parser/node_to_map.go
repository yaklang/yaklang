package stream_parser

import (
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
)

// NodeToMap projects current fields without executing custom out expressions.
// Each node reads live settings once; results and mutable values are not cached.
func NodeToMap(node *base.Node) any {
	settings := node.Cfg.ResultSettings()
	if settings.Present {
		// Match Result's collection type for an observed zero-width list.
		// Other result-bearing nodes keep their historical scalar behavior.
		if settings.List && !settings.Terminal && len(node.Children) == 0 {
			span := settings.Position.([2]uint64)
			if span[0] == span[1] {
				// NodeToMap historically bypasses custom out expressions.
				value, err := ToMap(node)
				if err == nil && value != nil && value.IsList() {
					return []any{}
				}
			}
		}
		value, err := getNodeResultWithSettings(node, false, settings)
		if err != nil {
			log.Errorf("get node result error: %v", err)
		}
		return value
	}
	if settings.List {
		res := make([]any, 0, len(node.Children))
		for _, sub := range node.Children {
			d := NodeToMap(sub)
			if d != nil {
				res = append(res, d)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	} else {
		res := make(map[string]any, len(node.Children))
		for _, sub := range node.Children {
			d := NodeToMap(sub)
			if d != nil {
				res[sub.Name] = d
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	}
}
