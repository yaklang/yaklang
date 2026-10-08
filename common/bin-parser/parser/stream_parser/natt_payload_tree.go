package stream_parser

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Immutable definitions are compatibility guards, not shared runtime trees.
var nattPayloadDefinitions = sync.OnceValues(func() (map[string]*base.Node, error) {
	root, err := base.ParseRule("nat_t.yaml")
	if err != nil {
		return nil, err
	}
	definitions := make(map[string]*base.Node)
	for _, child := range root.Children {
		definitions[child.Name] = child
	}
	if definitions["NATTIKEPayloads"] == nil || definitions["NATTIKEPayload"] == nil {
		return nil, fmt.Errorf("nat-t: built-in payload definitions not found")
	}
	return definitions, nil
})

// parseNATTPayloads replaces repeated preceding-sibling length walks only for
// the exact built-in scalar payload list. Decode and tree construction occur
// off-tree; the callback owns the staged reader/writer transaction. Customized
// schemas and Generate keep using the YAML operator via the caller's mode gate.
func parseNATTPayloads(node *base.Node, process func(*base.Node) (func(bool), error)) (bool, error) {
	if !node.Cfg.GetBool(CfgIsList) || NodeIsTerminal(node) || node.Cfg.Has("template") || node.Cfg.Has(CfgLengthCacheMap) ||
		node.Cfg.Has(CfgNodeResult) || node.Cfg.Has(CfgConsumedBits) || node.Cfg.Has("additionInfo") ||
		node.Cfg.GetString("parser") != "default" || node.Cfg.GetString(CfgEndian) != "big" ||
		!dicomByteUnit(node) || len(node.Children) != 1 {
		return false, nil
	}
	for _, key := range []string{CfgType, "out", "input", CfgImport, CfgRefType, CfgLengthFromField, CfgLengthForStartField, CfgLengthForField, CfgDelimiter, CfgDelimiterOptional, CfgDel, CfgStopValue, CfgExceptionPlan} {
		if node.Cfg.Has(key) {
			return false, nil
		}
	}
	alias := node.Children[0]
	if alias.Name != "Payload" || alias.Origin != "NATTIKEPayload" || alias.Cfg.GetItem(CfgParent) != node ||
		alias.Ctx != node.Ctx || len(alias.Children) != 0 || alias.Cfg.GetString(CfgType) != "NATTIKEPayload" ||
		alias.Cfg.GetString(CfgRefType) != "NATTIKEPayload" || NodeIsTerminal(alias) ||
		alias.Cfg.GetString("parser") != "default" || alias.Cfg.GetString(CfgEndian) != "big" ||
		alias.Cfg.Has(CfgUnit) != node.Cfg.Has(CfgUnit) || !reflect.DeepEqual(alias.Cfg.GetItem(CfgUnit), node.Cfg.GetItem(CfgUnit)) {
		return false, nil
	}
	for _, key := range []string{CfgNodeResult, CfgConsumedBits, CfgElementIndex, CfgLength, CfgOperator, "out", "input", CfgImport, CfgDelimiter, CfgDelimiterOptional, CfgDel, CfgIsList, CfgLengthFromField, CfgLengthForStartField, CfgLengthForField, CfgStopValue, CfgExceptionPlan, CfgLengthCacheMap, "template", "additionInfo"} {
		if alias.Cfg.Has(key) {
			return false, nil
		}
	}
	definitions, err := nattPayloadDefinitions()
	if err != nil {
		return false, err
	}
	if node.Cfg.GetString(CfgOperator) != definitions["NATTIKEPayloads"].Cfg.GetString(CfgOperator) {
		return false, nil
	}
	types, ok := node.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)
	definition := definitions["NATTIKEPayload"]
	if !ok || types["NATTIKEPayload"] == nil || !reflect.DeepEqual(types["NATTIKEPayload"].Origin, definition.Origin) {
		return false, nil
	}
	major, majorOK := node.Ctx.GetItem("natt_ike_major").(int)
	firstType, typeOK := node.Ctx.GetItem("natt_payload_type").(int)
	if !majorOK || !typeOK || (major != 1 && major != 2) || firstType < 0 || firstType > 255 {
		return false, nil
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return false, err
	}
	if !bounded || bits == 0 || bits%8 != 0 || bits > 65527*8 {
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
	for _, candidate := range append([]*base.Node{first}, first.Children...) {
		if candidate.Ctx != node.Ctx {
			return false, nil
		}
		for _, key := range []string{CfgLengthCacheMap, "additionInfo", "template", CfgLengthForField, CfgStopValue, CfgDelimiterOptional} {
			if candidate.Cfg.Has(key) {
				return false, nil
			}
		}
	}
	raw, err := base.NewNodeTreeWithConfig(node.Cfg, "NAT-T payload staged wire", "raw", node.Ctx)
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
	body := value.([]byte)
	records, err := decodeNATTPayloads(body, major, firstType)
	if err != nil {
		return true, err
	}
	start := GetNodeResultPos(raw)[0]
	end := 0
	for index, record := range records {
		element := first
		if index != 0 {
			element, err = ListNodeNewElement(staged)
			if err != nil {
				return true, err
			}
		}
		nattFillPayloadTree(element, record, major, start)
		end = record.Offset + record.Length
	}
	if end < len(body) {
		// Match NewUnknownNode, including raw Origin, inherited unit and
		// normal AppendNode initialization. Padding is not a list record.
		padding := &base.Node{Name: "Unknown", Origin: "raw", Cfg: base.NewConfig(staged.Cfg), Ctx: staged.Ctx}
		if err := appendNode(staged, padding); err != nil {
			return true, err
		}
		padding = staged.Children[len(staged.Children)-1]
		padding.Name = "ISAKMP Padding"
		padding.Cfg.SetItem(CfgLength, uint64(len(body)-end)*8)
		padding.Cfg.SetItem(CfgNodeResult, [2]uint64{start + uint64(end)*8, start + uint64(len(body))*8})
	}
	for _, element := range staged.Children {
		element.Cfg.SetItem(CfgParent, node)
	}
	template := staged.Cfg.GetItem("template").(*base.Node)
	template.Cfg.SetItem(CfgParent, node)
	node.Cfg.SetItem("template", template)
	node.Children = staged.Children
	node.Cfg.SetItem("additionInfo", map[string]any{"Payload Count": len(records)})
	node.Ctx.SetItem("natt_payload_type", 0)
	node.Ctx.SetItem("natt_payload_index", len(records)-1)
	committed = true
	return true, nil
}

func nattFillPayloadTree(element *base.Node, record nattPayloadRecord, major int, start uint64) {
	element.Cfg.SetItem(CfgLength, uint64(record.Length)*8)
	offset := start + uint64(record.Offset)*8
	setSpan := func(index, begin, end int) {
		element.Children[index].Cfg.SetItem(CfgNodeResult, [2]uint64{offset + uint64(begin)*8, offset + uint64(end)*8})
	}
	setRaw := func(index, begin, end int) {
		if begin < end {
			element.Children[index].Cfg.SetItem(CfgLength, uint64(end-begin)*8)
			setSpan(index, begin, end)
		}
	}
	setSpan(nattNextPayload, 0, 1)
	setSpan(nattPayloadFlags, 1, 2)
	setSpan(nattPayloadLength, 2, 4)
	reserved := record.Flags & 127
	if major == 1 {
		reserved = record.Flags
	}
	info := map[string]any{"Payload Type": record.Type, "Critical": major == 2 && record.Flags&128 != 0,
		"Reserved Flags": reserved, "Body Semantics Decoded": false}
	layout := "opaque"
	switch record.DataField {
	case nattEncryptedPayloadData:
		layout = "encrypted"
		info["Inner Next Payload"] = record.Next
		info["Inner Payloads Decoded"] = false
		if record.Type == 53 {
			setSpan(nattFragmentNumber, 4, 6)
			setSpan(nattTotalFragments, 6, 8)
		}
	case nattNotificationData:
		layout = "notification"
		baseOffset := 4
		if major == 1 {
			setSpan(nattDomainOfInterpretation, 4, 8)
			baseOffset = 8
		}
		setSpan(nattProtocolID, baseOffset, baseOffset+1)
		setSpan(nattSPISize, baseOffset+1, baseOffset+2)
		setSpan(nattNotifyType, baseOffset+2, baseOffset+4)
		setRaw(nattNotificationSPI, baseOffset+4, baseOffset+4+record.SPILength)
	case nattKeyExchangeData:
		layout = "key-exchange"
		setSpan(nattDHGroup, 4, 6)
		setSpan(nattKEReserved, 6, 8)
	case nattNonceData:
		layout = "nonce"
	case nattVendorID:
		layout = "vendor-id"
	}
	setRaw(record.DataField, record.DataOffset, record.Length)
	info["Body Layout"] = layout
	element.Cfg.SetItem("additionInfo", info)
}
