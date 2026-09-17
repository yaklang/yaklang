package stream_parser

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Immutable definitions are compatibility guards, never runtime node caches.
var nhrpClientDefinitions = sync.OnceValues(func() (map[string]*base.Node, error) {
	root, err := base.ParseRule("nhrp.yaml")
	if err != nil {
		return nil, err
	}
	definitions := make(map[string]*base.Node)
	for _, child := range root.Children {
		definitions[child.Name] = child
	}
	if definitions["NHRPClients"] == nil || definitions["NHRPClient"] == nil {
		return nil, fmt.Errorf("nhrp: built-in client definitions not found")
	}
	return definitions, nil
})

// parseNHRPClients avoids repeated preceding-sibling length walks only for the
// exact built-in scalar schema. It validates a bounded wire view, builds the
// original field tree off-tree, then commits it atomically. Caller-customized
// rules and Generate retain the existing YAML path; no mutable length cache is
// introduced. The process callback owns the reader/writer transaction.
func parseNHRPClients(node *base.Node, process func(*base.Node) (func(bool), error)) (bool, error) {
	if !node.Cfg.GetBool(CfgIsList) || node.Cfg.Has("template") || node.Cfg.Has(CfgLengthCacheMap) ||
		node.Cfg.Has(CfgNodeResult) || node.Cfg.Has(CfgConsumedBits) || node.Cfg.Has("additionInfo") ||
		node.Cfg.GetString("parser") != "default" || node.Cfg.GetString(CfgEndian) != "big" ||
		!dicomByteUnit(node) || len(node.Children) != 1 {
		return false, nil
	}
	for _, key := range []string{CfgType, "out", "input", CfgLengthFromField, CfgLengthForStartField, CfgDelimiter, CfgDel, CfgExceptionPlan} {
		if node.Cfg.Has(key) {
			return false, nil
		}
	}
	alias := node.Children[0]
	if alias.Name != "Client" || alias.Origin != "NHRPClient" || alias.Cfg.GetItem(CfgParent) != node ||
		alias.Cfg.GetString(CfgRefType) != "NHRPClient" || NodeIsTerminal(alias) ||
		alias.Cfg.GetString("parser") != "default" || alias.Cfg.GetString(CfgEndian) != "big" ||
		alias.Cfg.Has(CfgUnit) != node.Cfg.Has(CfgUnit) || !reflect.DeepEqual(alias.Cfg.GetItem(CfgUnit), node.Cfg.GetItem(CfgUnit)) {
		return false, nil
	}
	for _, key := range []string{CfgNodeResult, CfgConsumedBits, CfgElementIndex, CfgLength, CfgOperator, "out", "input", CfgImport, CfgDelimiter, CfgDel, CfgIsList, CfgLengthFromField, CfgLengthForStartField, CfgExceptionPlan} {
		if alias.Cfg.Has(key) {
			return false, nil
		}
	}
	definitions, err := nhrpClientDefinitions()
	if err != nil {
		return false, err
	}
	if node.Cfg.GetString(CfgOperator) != definitions["NHRPClients"].Cfg.GetString(CfgOperator) {
		return false, nil
	}
	types, ok := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
	definition := definitions["NHRPClient"]
	if !ok || types["NHRPClient"] == nil || !reflect.DeepEqual(types["NHRPClient"].Origin, definition.Origin) {
		return false, nil
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return false, err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > 65535*8 {
		return false, nil
	}
	staged := node.Copy()
	first, err := ListNodeNewElement(staged)
	if err != nil {
		return false, err
	}
	// CIEs have the same scalar-only schema constraints as the DICOM bridge.
	if !dicomStandardScalarStruct(first, definition) || first.Cfg.Has(CfgUnit) != node.Cfg.Has(CfgUnit) ||
		!reflect.DeepEqual(first.Cfg.GetItem(CfgUnit), node.Cfg.GetItem(CfgUnit)) {
		return false, nil
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "NHRP client staged wire", "raw", node.Ctx)
	if err != nil {
		return false, err
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
	records, err := decodeNHRPClientsBody(value.([]byte))
	if err != nil {
		return true, err
	}
	start := GetNodeResultPos(raw)[0]
	for index, record := range records {
		element := first
		if index != 0 {
			element, err = ListNodeNewElement(staged)
			if err != nil {
				return true, err
			}
		}
		offset := start + uint64(record.Offset)*8
		previous := uint64(0)
		for fieldIndex, end := range [...]uint64{8, 16, 32, 48, 64, 72, 80, 88, 96} {
			element.Children[fieldIndex].Cfg.SetItem(CfgNodeResult, [2]uint64{offset + previous, offset + end})
			previous = end
		}
		for addressIndex, length := range record.AddressLengths {
			if length > 0 {
				field := element.Children[9+addressIndex]
				field.Cfg.SetItem(CfgLength, uint64(length)*8)
				field.Cfg.SetItem(CfgNodeResult, [2]uint64{offset + previous, offset + previous + uint64(length)*8})
				previous += uint64(length) * 8
			}
		}
	}
	for _, element := range staged.Children {
		element.Cfg.SetItem(CfgParent, node)
	}
	template := staged.Cfg.GetItem("template").(*base.Node)
	template.Cfg.SetItem(CfgParent, node)
	node.Cfg.SetItem("template", template)
	node.Children = staged.Children
	// Yak normalizes the uint8 field value to int before AddInfo's any argument.
	node.Cfg.SetItem("additionInfo", map[string]any{"Client Count": len(records), "First Prefix Length": int(value.([]byte)[1])})
	committed = true
	return true, nil
}
