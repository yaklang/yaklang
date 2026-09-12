package stream_parser

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// These immutable definitions guard the optimization; runtime trees and their
// values are always local to an individual parse, including repeated fields.
var wsmpDefinitions = sync.OnceValues(func() (map[string]*base.Node, error) {
	root, err := base.ParseRule("wsmp.yaml")
	if err != nil {
		return nil, err
	}
	if err := InitNode(root); err != nil {
		return nil, err
	}
	definitions := make(map[string]*base.Node)
	for _, child := range root.Children {
		definitions[child.Name] = child
	}
	return definitions, nil
})

var wsmpSchemaKeys = []string{
	CfgType, CfgOperator, "out", "input", CfgLength, CfgIsList,
	CfgImport, CfgRefType, CfgDelimiter, CfgDelimiterOptional, CfgDel,
	CfgLengthFromField, CfgLengthForStartField, CfgLengthForField, CfgStopValue,
	CfgExceptionPlan, CfgNodeResult, CfgConsumedBits, CfgElementIndex,
	CfgLengthCacheMap, "additionInfo", "template", CfgInList,
}

func wsmpSameSchema(current, standard *base.Node, instance bool) bool {
	if current == nil || standard == nil || !reflect.DeepEqual(current.Origin, standard.Origin) ||
		len(current.Children) != len(standard.Children) || current.Cfg.GetString("parser") != "default" ||
		current.Cfg.GetString(CfgEndian) != "big" || !dicomByteUnit(current) ||
		NodeIsTerminal(current) != NodeIsTerminal(standard) {
		return false
	}
	for _, key := range wsmpSchemaKeys {
		if instance && key == CfgLength {
			continue
		}
		if current.Cfg.Has(key) != standard.Cfg.Has(key) || !reflect.DeepEqual(current.Cfg.GetItem(key), standard.Cfg.GetItem(key)) {
			return false
		}
	}
	for index, child := range current.Children {
		if child.Name != standard.Children[index].Name || child.Cfg.GetItem(CfgParent) != current || child.Ctx != current.Ctx ||
			child.Cfg.GetBool(CfgLastNode) != (index == len(current.Children)-1) ||
			!wsmpSameSchema(child, standard.Children[index], false) {
			return false
		}
	}
	return true
}

// parseWSMP replaces nested interpreter entry and repeated list length walks,
// not the schema or result contract. It stages one bounded message, validates
// the entire wire, fills copies of the original rule nodes, and only then
// commits the reader/writer transaction. Modified definitions use the YAML path.
func parseWSMP(node *base.Node, process func(*base.Node) (func(bool), error)) (bool, error) {
	definitions, err := wsmpDefinitions()
	if err != nil {
		return false, err
	}
	if !wsmpSameSchema(node, definitions["WSMPMessage"], true) {
		return false, nil
	}
	types, ok := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
	if !ok {
		return false, nil
	}
	for _, name := range []string{"WSMPMessage", "WSMPVersion2", "WSMPVersion3", "WSMPPSID", "WSMPLength", "WSMPExtensions", "WSMPInformationElements", "WSMPInformationElement", "WSMPLegacyElements", "WSMPLegacyElement", "WSMPSupplement", "WSMPPeekByte"} {
		if !wsmpSameSchema(types[name], definitions[name], false) || types[name].Ctx != node.Ctx {
			return false, nil
		}
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return true, err
	}
	if !bounded || bits%8 != 0 {
		return false, nil
	}
	if bits < 4*8 {
		return true, fmt.Errorf("wsmp: truncated minimum message")
	}
	if bits > 65535*8 {
		return true, fmt.Errorf("wsmp: message exceeds the 65535-byte implementation boundary")
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "WSMP staged wire", "raw", node.Ctx)
	if err != nil {
		return true, err
	}
	raw.Cfg.SetItem(CfgParent, node)
	raw.Cfg.SetItem(CfgLength, bits)
	finish, err := process(raw)
	committed := false
	if finish != nil {
		defer func() { finish(!committed) }()
	}
	if err != nil {
		return true, err
	}
	value, err := getNodeResult(raw, true)
	if err != nil {
		return true, err
	}
	plan, err := decodeWSMP(value.([]byte))
	if err != nil {
		return true, err
	}
	staged := node.Copy()
	if err := wsmpFillFields(staged, []wsmpField{plan}, GetNodeResultPos(raw)[0]); err != nil {
		return true, err
	}
	for _, child := range staged.Children {
		child.Cfg.SetItem(CfgParent, node)
	}
	node.Children = staged.Children
	node.Cfg.SetItem("additionInfo", map[string]any{"Application Message Decoded": false})
	committed = true
	return true, nil
}

func wsmpFillFields(parent *base.Node, fields []wsmpField, start uint64) error {
	for _, field := range fields {
		var node *base.Node
		if parent.Cfg.GetBool(CfgIsList) {
			var err error
			node, err = ListNodeNewElement(parent)
			if err != nil {
				return err
			}
		} else {
			for _, candidate := range parent.Children {
				if candidate.Name == field.name {
					node = candidate
					break
				}
			}
		}
		if node == nil {
			return fmt.Errorf("wsmp: field %q absent from guarded schema", field.name)
		}
		if field.length >= 0 {
			node.Cfg.SetItem(CfgLength, uint64(field.length)*8)
		}
		if field.info != nil {
			node.Cfg.SetItem("additionInfo", field.info)
		}
		if node.Cfg.Has(CfgRefType) {
			resolved, err := ParseRefNode(node)
			if err != nil {
				return err
			}
			*node = *resolved
			if err := InitNode(node); err != nil {
				return err
			}
		}
		if field.terminal {
			node.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(field.start), start + uint64(field.end)})
		} else if err := wsmpFillFields(node, field.children, start); err != nil {
			return err
		}
	}
	return nil
}
