package stream_parser

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

var dicomUserDefinitions = sync.OnceValues(func() (map[string]*base.Node, error) {
	root, err := base.ParseRule("application-layer/dicom.yaml")
	if err != nil {
		return nil, err
	}
	definitions := make(map[string]*base.Node)
	for _, child := range root.Children {
		if child.Name == "DICOMUserItem" || child.Name == "DICOMUID" {
			definitions[child.Name] = child
		}
	}
	if len(definitions) != 2 {
		return nil, fmt.Errorf("dicom: built-in user information definitions not found")
	}
	return definitions, nil
})

// This rule-specific operation validates the entire bounded user-information
// body and builds the legacy field tree off-tree before a single commit. It
// does not memoize generic node lengths or change structured generation.
func parseDICOMUserInformation(node *base.Node, process func(*base.Node) (func(bool), error)) (bool, error) {
	if !node.Cfg.GetBool(CfgIsList) || node.Cfg.Has("template") || node.Cfg.Has(CfgLengthCacheMap) ||
		node.Cfg.Has(CfgNodeResult) || node.Cfg.Has(CfgConsumedBits) || node.Cfg.GetString("parser") != "default" ||
		node.Cfg.GetString(CfgEndian) != "big" || !dicomByteUnit(node) || len(node.Children) != 1 {
		return false, nil
	}
	alias := node.Children[0]
	if alias.Name != "User Item" || alias.Origin != "DICOMUserItem" || alias.Cfg.GetItem(CfgParent) != node ||
		alias.Cfg.GetString(CfgRefType) != "DICOMUserItem" || NodeIsTerminal(alias) ||
		alias.Cfg.GetString("parser") != "default" || alias.Cfg.GetString(CfgEndian) != "big" ||
		alias.Cfg.Has(CfgUnit) != node.Cfg.Has(CfgUnit) || !reflect.DeepEqual(alias.Cfg.GetItem(CfgUnit), node.Cfg.GetItem(CfgUnit)) {
		return false, nil
	}
	for _, key := range []string{CfgNodeResult, CfgConsumedBits, CfgElementIndex, CfgLength, CfgOperator, "out", "input", CfgImport, CfgDelimiter, CfgDel, CfgIsList, CfgLengthFromField} {
		if alias.Cfg.Has(key) {
			return false, nil
		}
	}
	// The built-in PDU initializes this counter with a Yak integer, and each
	// item adds integer one. Nonstandard caller state keeps the YAML semantics.
	initialCount, ok := node.Ctx.GetItem("dicomItemCount").(int)
	if !ok || initialCount < 0 || initialCount > 4096 {
		return false, nil
	}
	definitions, err := dicomUserDefinitions()
	if err != nil {
		return false, err
	}
	types, ok := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
	if !ok {
		return false, nil
	}
	for name, definition := range definitions {
		if types[name] == nil || !reflect.DeepEqual(types[name].Origin, definition.Origin) {
			return false, nil
		}
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return false, err
	}
	// The enclosing association item carries a uint16 body length.
	if !bounded || bits == 0 || bits%8 != 0 || bits > 65535*8 {
		return false, nil
	}
	staged := node.Copy()
	first, err := ListNodeNewElement(staged)
	if err != nil {
		return false, err
	}
	if !dicomStandardScalarStruct(first, definitions["DICOMUserItem"]) || first.Cfg.Has(CfgUnit) != node.Cfg.Has(CfgUnit) ||
		!reflect.DeepEqual(first.Cfg.GetItem(CfgUnit), node.Cfg.GetItem(CfgUnit)) {
		return false, nil
	}
	// ProcessByType appends an initialized clone from Origin, not the mutable
	// definition tree. Check the exact nested construction before reading wire.
	probe := first.Copy()
	if err := appendNode(probe, types["DICOMUID"]); err != nil {
		return false, err
	}
	if !dicomStandardScalarStruct(probe.Children[len(probe.Children)-1], definitions["DICOMUID"]) {
		return false, nil
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "DICOM user information staged wire", "raw", node.Ctx)
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
	records, err := decodeDICOMUserInformationBody(value.([]byte), uint64(initialCount))
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
		for fieldIndex, span := range [...][2]uint64{{offset, offset + 8}, {offset + 8, offset + 16}, {offset + 16, offset + 32}} {
			element.Children[fieldIndex].Cfg.SetItem(CfgNodeResult, span)
		}
		valueStart := start + uint64(record.ValueOffset)*8
		valueSpan := [2]uint64{valueStart, valueStart + uint64(record.ValueLength)*8}
		switch record.Type {
		case 81:
			element.Children[3].Cfg.SetItem(CfgNodeResult, valueSpan)
		case 82:
			if err := appendNode(element, types["DICOMUID"]); err != nil {
				return true, err
			}
			uid := element.Children[len(element.Children)-1]
			uid.Name = "Implementation Class"
			uid.Children[0].Cfg.SetItem(CfgLength, uint64(record.ValueLength)*8)
			uid.Children[0].Cfg.SetItem(CfgNodeResult, valueSpan)
		case 85:
			element.Children[4].Cfg.SetItem(CfgLength, uint64(record.ValueLength)*8)
			element.Children[4].Cfg.SetItem(CfgNodeResult, valueSpan)
		default:
			if record.ValueLength != 0 {
				element.Children[5].Cfg.SetItem(CfgLength, uint64(record.ValueLength)*8)
				element.Children[5].Cfg.SetItem(CfgNodeResult, valueSpan)
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
	node.Ctx.SetItem("dicomItemCount", initialCount+len(records))
	committed = true
	return true, nil
}
