package stream_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// parseZigbeeFrame is an explicit parse-only operation. It never guesses the
// application protocol of arbitrary IEEE 802.15.4 input. A failed staged read
// or decode leaves the caller's existing children, configuration and IO intact.
func parseZigbeeFrame(node *base.Node, process func(*base.Node) (func(bool), error), hasFCS bool) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	minimum, maximum := uint64(3), uint64(125)
	if hasFCS {
		minimum, maximum = 5, 127
	}
	if !bounded || bits%8 != 0 || bits < minimum*8 || bits > maximum*8 {
		return fmt.Errorf("zigbee: explicit byte frame boundary must be %d..%d bytes", minimum, maximum)
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "Zigbee staged frame", "raw", node.Ctx)
	if err != nil {
		return err
	}
	raw.Cfg.SetItem(CfgParent, node)
	raw.Cfg.SetItem(CfgLength, bits)
	finish, err := process(raw)
	committed := false
	if finish != nil {
		defer func() { finish(!committed) }()
	}
	if err != nil {
		return err
	}
	value, err := getNodeResult(raw, true)
	if err != nil {
		return err
	}
	wire, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("zigbee: staged frame is not raw bytes")
	}
	fields, info, err := decodeZigbeeFrame(wire, hasFCS)
	if err != nil {
		return err
	}
	start := GetNodeResultPos(raw)[0]
	staged := &base.Node{Name: node.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(node.Cfg), Ctx: node.Ctx}
	staged.Cfg.SetItem(CfgEndian, "little")
	var fill func(*base.Node, []zigbeeField) error
	fill = func(parent *base.Node, fields []zigbeeField) error {
		for index, field := range fields {
			if field.Start < 0 || field.End < field.Start || uint64(field.End)*8 > bits {
				return fmt.Errorf("zigbee: invalid decoded field span")
			}
			var child *base.Node
			if field.Type == "" {
				child = &base.Node{Name: field.Name, Origin: yaml.MapSlice{}, Cfg: base.NewConfig(parent.Cfg), Ctx: node.Ctx}
				child.Cfg.SetItem(CfgIsList, field.List)
			} else {
				switch field.Type {
				case "raw", "uint8", "uint16", "uint32":
				default:
					return fmt.Errorf("zigbee: unsupported field type")
				}
				child, err = base.NewNodeTreeWithConfig(parent.Cfg, field.Name, field.Type, node.Ctx)
				if err != nil {
					return err
				}
				child.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(field.Start)*8, start + uint64(field.End)*8})
			}
			child.Cfg.SetItem(CfgLength, uint64(field.End-field.Start)*8)
			child.Cfg.SetItem(CfgParent, parent)
			if parent.Cfg.GetBool(CfgIsList) {
				child.Cfg.SetItem(CfgElementIndex, index)
			}
			if err := fill(child, field.Children); err != nil {
				return err
			}
			parent.Children = append(parent.Children, child)
		}
		return nil
	}
	if err = fill(staged, fields); err != nil {
		return err
	}
	if err = InitNode(staged); err != nil {
		return err
	}
	for _, child := range staged.Children {
		child.Cfg.SetItem(CfgParent, node)
	}
	node.Children = staged.Children
	node.Cfg.SetItem("additionInfo", info)
	committed = true
	return nil
}
