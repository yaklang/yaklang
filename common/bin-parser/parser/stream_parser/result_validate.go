package stream_parser

import (
	"errors"
	"reflect"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
)

// This is a discard operation, not a result cache. Every call reads the live
// tree and runs the same validation and custom output/formatter callbacks.
// Private wrappers and pure scalar/byte conversions of discarded data are
// omitted; the same projection boundary checker still reads the live context.
func (d *DefParser) ValidateResult(node *base.Node) error {
	if node.Cfg.Has("out") {
		_, err := d.Result(node)
		return err
	}
	formatter := "default"
	if d.ctx.Has("formatter") {
		formatter = d.ctx.GetString("formatter")
	}
	if formatter != "default" ||
		reflect.ValueOf(formatters["default"]).Pointer() != reflect.ValueOf(ToMap).Pointer() {
		_, err := d.Result(node)
		return err
	}
	settings := node.Cfg.ResultSettings()
	if settings.Present {
		span := settings.Position.([2]uint64)
		var source nodeResultSource
		_, _, err := source.checkedSpan(node.Ctx, span)
		if settings.List && !settings.Terminal && len(node.Children) == 0 && span[0] == span[1] {
			return err
		}
		// Normal scalar projection logs a bad span and returns a nil value;
		// empty-list projection above propagates it. Preserve that distinction.
		if err != nil {
			log.Errorf("get node result error: %v", err)
		}
		return nil
	}
	children := node.Children
	if !settings.List {
		// ToMap snapshots flattened struct children BEFORE invoking any child
		// Result. An out callback may mutate Children, so keep that ordering.
		children = collectResultChildren(nil, node)
	}
	found := false
	for _, child := range children {
		if err := child.ValidateResult(); err != nil {
			if errors.Is(err, noResultError) {
				continue
			}
			return err
		}
		found = true
	}
	if !found {
		return noResultError
	}
	return nil
}

func collectResultChildren(children []*base.Node, node *base.Node) []*base.Node {
	for _, sub := range node.Children {
		if sub.Cfg.GetBool(CfgIsRefType) || sub.Cfg.GetBool("unpack") ||
			sub.Name == "Package" && sub.Cfg.GetItem(CfgParent) == sub.Ctx.GetItem("root") {
			children = collectResultChildren(children, sub)
		} else {
			children = append(children, sub)
		}
	}
	return children
}
