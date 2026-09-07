package stream_parser

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// This private definition is only a compatibility guard, never a shared
// runtime tree. Caller-modified PDV definitions keep using their YAML operator.
var dicomPDVDefinition = sync.OnceValues(func() (*base.Node, error) {
	root, err := base.ParseRule("application-layer/dicom.yaml")
	if err != nil {
		return nil, err
	}
	for _, child := range root.Children {
		if child.Name == "DICOMPDV" {
			return child, nil
		}
	}
	return nil, fmt.Errorf("dicom: built-in PDV definition not found")
})

// Match a built-in scalar-field struct without sharing its runtime Nodes.
// PDVs, user items and their nested UID struct use this same schema guard.
func dicomStandardScalarStruct(prototype, definition *base.Node) bool {
	if prototype.Cfg.GetString(CfgOperator) != definition.Cfg.GetString(CfgOperator) ||
		prototype.Cfg.GetString(CfgEndian) != "big" || prototype.Cfg.GetString("parser") != "default" ||
		!dicomByteUnit(prototype) || NodeIsTerminal(prototype) || prototype.Cfg.GetBool(CfgIsList) || len(prototype.Children) != len(definition.Children) {
		return false
	}
	for _, key := range []string{CfgType, CfgConsumedBits, CfgElementIndex, "out", "input", CfgNodeResult, CfgLength, CfgLengthCacheMap, CfgLengthFromField, CfgLengthForStartField, CfgImport, CfgRefType, CfgDelimiter, CfgDel, CfgExceptionPlan} {
		if prototype.Cfg.Has(key) {
			return false
		}
	}
	for i, field := range prototype.Children {
		standard := definition.Children[i]
		if field.Name != standard.Name || !reflect.DeepEqual(field.Origin, standard.Origin) || len(field.Children) != 0 || !NodeIsTerminal(field) {
			return false
		}
		if field.Cfg.GetItem(CfgParent) != prototype || field.Cfg.GetBool(CfgLastNode) != (i == len(prototype.Children)-1) {
			return false
		}
		if field.Cfg.Has(CfgUnit) != prototype.Cfg.Has(CfgUnit) || !reflect.DeepEqual(field.Cfg.GetItem(CfgUnit), prototype.Cfg.GetItem(CfgUnit)) {
			return false
		}
		for _, key := range []string{CfgConsumedBits, CfgElementIndex, "out", "input", CfgIsList} {
			if field.Cfg.Has(key) {
				return false
			}
		}
		for _, key := range []string{CfgType, CfgLength, CfgEndian, "parser", CfgOperator, CfgImport, CfgRefType, CfgDelimiter, CfgDel, CfgLengthFromField, CfgLengthForStartField, CfgExceptionPlan, CfgNodeResult} {
			if field.Cfg.Has(key) != standard.Cfg.Has(key) || !reflect.DeepEqual(field.Cfg.GetItem(key), standard.Cfg.GetItem(key)) {
				return false
			}
		}
	}
	return true
}

// The built-in transport imports explicitly inherit "byte"; a standalone
// rule's absent unit has the same meaning in getMulti. Preserve either form.
func dicomByteUnit(node *base.Node) bool {
	return !node.Cfg.Has(CfgUnit) || node.Cfg.GetItem(CfgUnit) == "byte"
}

// parseDICOMPDVList is an explicit rule operation, not a general length cache.
// It stages both the wire read and the entire result tree. The callback owns a
// normal parser transaction, including bit-reader replay and writer rollback.
func parseDICOMPDVList(node *base.Node, process func(*base.Node) (func(bool), error)) (bool, error) {
	if !node.Cfg.GetBool(CfgIsList) || node.Cfg.Has("template") || node.Cfg.Has(CfgLengthCacheMap) ||
		node.Cfg.Has(CfgNodeResult) || node.Cfg.Has(CfgConsumedBits) || node.Cfg.GetString("parser") != "default" ||
		node.Cfg.GetString(CfgEndian) != "big" || !dicomByteUnit(node) || len(node.Children) != 1 {
		return false, nil
	}
	alias := node.Children[0]
	if alias.Name != "PDV" || alias.Origin != "DICOMPDV" || alias.Cfg.GetItem(CfgParent) != node ||
		alias.Cfg.GetString(CfgRefType) != "DICOMPDV" || NodeIsTerminal(alias) ||
		alias.Cfg.GetString("parser") != "default" || alias.Cfg.GetString(CfgEndian) != "big" ||
		alias.Cfg.Has(CfgUnit) != node.Cfg.Has(CfgUnit) || !reflect.DeepEqual(alias.Cfg.GetItem(CfgUnit), node.Cfg.GetItem(CfgUnit)) {
		return false, nil
	}
	for _, key := range []string{CfgNodeResult, CfgConsumedBits, CfgElementIndex, CfgLength, CfgOperator, "out", "input", CfgImport, CfgDelimiter, CfgDel, CfgIsList, CfgLengthFromField} {
		if alias.Cfg.Has(key) {
			return false, nil
		}
	}
	definition, err := dicomPDVDefinition()
	if err != nil {
		return false, err
	}
	types, ok := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
	if !ok || types["DICOMPDV"] == nil || !reflect.DeepEqual(types["DICOMPDV"].Origin, definition.Origin) {
		return false, nil
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return false, err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > 16777216*8 {
		return false, nil
	}
	staged := node.Copy()
	first, err := ListNodeNewElement(staged)
	if err != nil {
		return false, err
	}
	if !dicomStandardScalarStruct(first, definition) || first.Cfg.Has(CfgUnit) != node.Cfg.Has(CfgUnit) ||
		!reflect.DeepEqual(first.Cfg.GetItem(CfgUnit), node.Cfg.GetItem(CfgUnit)) {
		return false, nil
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "DICOM PDV staged wire", "raw", node.Ctx)
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
	records, err := decodeDICOMPDVBody(value.([]byte))
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
		element.Cfg.SetItem(CfgLength, (uint64(record.Length)+4)*8)
		offset := start + uint64(record.Offset)*8
		ends := [...]uint64{32, 40, 46, 47, 48}
		previous := uint64(0)
		for fieldIndex, end := range ends {
			element.Children[fieldIndex].Cfg.SetItem(CfgNodeResult, [2]uint64{offset + previous, offset + end})
			previous = end
		}
		if record.FragmentLength > 0 {
			fragment := element.Children[5]
			fragment.Cfg.SetItem(CfgLength, uint64(record.FragmentLength)*8)
			fragment.Cfg.SetItem(CfgNodeResult, [2]uint64{offset + 48, offset + 48 + uint64(record.FragmentLength)*8})
		}
	}
	// No field span for the staged raw body is installed: only the original
	// list/PDV/terminal shape contributes to Result and consumed-length queries.
	for _, element := range staged.Children {
		element.Cfg.SetItem(CfgParent, node)
	}
	template := staged.Cfg.GetItem("template").(*base.Node)
	template.Cfg.SetItem(CfgParent, node)
	node.Cfg.SetItem("template", template)
	node.Children = staged.Children
	committed = true
	return true, nil
}
