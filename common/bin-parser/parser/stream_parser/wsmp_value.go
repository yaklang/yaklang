package stream_parser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// A private decode plan contains only byte/bit positions and bounded schema
// selections. It never caches public NodeValues or application interpretations.
type wsmpField struct {
	name     string
	start    int
	end      int
	terminal bool
	length   int // Explicit byte boundary, or -1 when the YAML leaves it inherited.
	info     map[string]any
	children []wsmpField
}

func wsmpBranch(name string) wsmpField { return wsmpField{name: name, length: -1} }

type wsmpDecoder struct {
	wire []byte
	pos  int
}

func (d *wsmpDecoder) take(name string, size int) (wsmpField, error) {
	if size < 0 || size > len(d.wire)-d.pos {
		return wsmpField{}, fmt.Errorf("wsmp: truncated %s", name)
	}
	f := wsmpField{name: name, start: d.pos * 8, end: (d.pos + size) * 8, terminal: true, length: -1}
	d.pos += size
	return f, nil
}

func (d *wsmpDecoder) scalar(parent *wsmpField, name string, size int) (int, error) {
	f, err := d.take(name, size)
	if err != nil {
		return 0, err
	}
	parent.children = append(parent.children, f)
	value := 0
	for _, octet := range d.wire[f.start/8 : f.end/8] {
		value = value<<8 | int(octet)
	}
	return value, nil
}

func (d *wsmpDecoder) raw(parent *wsmpField, name string, size int) error {
	f, err := d.take(name, size)
	if err != nil {
		return err
	}
	f.length = size
	parent.children = append(parent.children, f)
	return nil
}

func (d *wsmpDecoder) psid(parent *wsmpField) error {
	if d.pos == len(d.wire) {
		return fmt.Errorf("wsmp: truncated PSID")
	}
	first, size := d.wire[d.pos], 1
	switch {
	case first < 128:
	case first < 192:
		size = 2
	case first < 224:
		size = 3
	case first < 240:
		size = 4
	default:
		return fmt.Errorf("wsmp: invalid PSID encoding prefix")
	}
	f := wsmpBranch("PSID")
	if err := d.raw(&f, "Encoded PSID", size); err != nil {
		return err
	}
	parent.children = append(parent.children, f)
	return nil
}

func (d *wsmpDecoder) length(parent *wsmpField, name string) (int, error) {
	f := wsmpBranch(name)
	first, err := d.scalar(&f, "Length First Octet", 1)
	if err != nil {
		return 0, err
	}
	if first >= 192 {
		return 0, fmt.Errorf("wsmp: invalid length or count encoding prefix")
	}
	value := first
	if first >= 128 {
		second, err := d.scalar(&f, "Length Second Octet", 1)
		if err != nil {
			return 0, err
		}
		value = (first&63)<<8 | second
		if value < 128 {
			return 0, fmt.Errorf("wsmp: nonminimal length or count encoding")
		}
	}
	parent.children = append(parent.children, f)
	return value, nil
}

func wsmpNetworkField(id int) string {
	switch id {
	case 4:
		return "Transmit Power"
	case 15:
		return "Channel Number"
	case 16:
		return "Data Rate"
	case 23:
		return "Channel Load"
	}
	return ""
}

func (d *wsmpDecoder) extensions(parent *wsmpField, network bool) error {
	name := "Transport Extensions"
	if network {
		name = "Network Extensions"
	}
	f := wsmpBranch(name)
	f.length, f.info = len(d.wire)-d.pos, map[string]any{"Network Fields": network}
	count, err := d.length(&f, "Element Count")
	if err != nil {
		return err
	}
	if count > 1024 {
		return fmt.Errorf("wsmp: extension count exceeds the 1024-element implementation limit")
	}
	if count*2 > len(d.wire)-d.pos {
		return fmt.Errorf("wsmp: extension count exceeds message boundary")
	}
	if count > 0 {
		list := wsmpBranch("Information Elements")
		list.length, list.info = len(d.wire)-d.pos, map[string]any{"Count": count, "Network Fields": network}
		list.children = make([]wsmpField, 0, count)
		for index := 0; index < count; index++ {
			element := wsmpBranch("Element")
			element.length, element.info = len(d.wire)-d.pos, map[string]any{"Network Fields": network}
			id, err := d.scalar(&element, "Element ID", 1)
			if err != nil {
				return err
			}
			length, err := d.length(&element, "Element Length")
			if err != nil {
				return err
			}
			if length > len(d.wire)-d.pos {
				return fmt.Errorf("wsmp: information element exceeds message boundary")
			}
			field := wsmpNetworkField(id)
			if network && field != "" {
				if length != 1 {
					return fmt.Errorf("wsmp: known network element requires one value octet")
				}
				if _, err := d.scalar(&element, field, 1); err != nil {
					return err
				}
			} else if length > 0 {
				if err := d.raw(&element, "Element Data", length); err != nil {
					return err
				}
			}
			list.children = append(list.children, element)
		}
		f.children = append(f.children, list)
	}
	parent.children = append(parent.children, f)
	return nil
}

func (d *wsmpDecoder) version3(f *wsmpField) error {
	first := d.wire[0]
	for _, field := range []struct {
		name       string
		start, end int
	}{
		{"Subtype", 0, 4}, {"Network Extension Present", 4, 5}, {"Version", 5, 8},
	} {
		f.children = append(f.children, wsmpField{name: field.name, start: field.start, end: field.end, terminal: true, length: -1})
	}
	d.pos = 1
	if first>>4 != 0 {
		return fmt.Errorf("wsmp: unsupported network subtype")
	}
	if first&8 != 0 {
		if err := d.extensions(f, true); err != nil {
			return err
		}
	}
	tpid, err := d.scalar(f, "TPID", 1)
	if err != nil {
		return err
	}
	if tpid > 3 {
		return fmt.Errorf("wsmp: unsupported transport identifier")
	}
	if tpid < 2 {
		if err := d.psid(f); err != nil {
			return err
		}
	} else {
		if _, err := d.scalar(f, "Source ITS Port", 2); err != nil {
			return err
		}
		if _, err := d.scalar(f, "Destination ITS Port", 2); err != nil {
			return err
		}
	}
	if tpid&1 != 0 {
		if err := d.extensions(f, false); err != nil {
			return err
		}
	}
	length, err := d.length(f, "WSM Length")
	if err != nil {
		return err
	}
	if length != len(d.wire)-d.pos {
		return fmt.Errorf("wsmp: WSM length does not match message boundary")
	}
	if length > 0 {
		return d.raw(f, "WSM Data", length)
	}
	return nil
}

func (d *wsmpDecoder) version2(f *wsmpField) error {
	if _, err := d.scalar(f, "Version", 1); err != nil {
		return err
	}
	if err := d.psid(f); err != nil {
		return err
	}
	list := wsmpBranch("Legacy Extensions")
	list.length = len(d.wire) - d.pos
	for {
		if d.pos == len(d.wire) {
			return fmt.Errorf("wsmp: missing legacy WAVE element identifier")
		}
		id := int(d.wire[d.pos])
		if id == 128 || id == 129 || id == 130 {
			break
		}
		if id != 4 && id != 15 && id != 16 {
			return fmt.Errorf("wsmp: unsupported legacy extension identifier")
		}
		if len(list.children) >= 256 {
			return fmt.Errorf("wsmp: too many legacy extension fields")
		}
		element := wsmpBranch("Element")
		if _, err := d.scalar(&element, "Element ID", 1); err != nil {
			return err
		}
		length, err := d.scalar(&element, "Element Length", 1)
		if err != nil {
			return err
		}
		if length != 1 {
			return fmt.Errorf("wsmp: legacy network element requires one value octet")
		}
		if _, err := d.scalar(&element, wsmpNetworkField(id), 1); err != nil {
			return err
		}
		list.children = append(list.children, element)
	}
	if len(list.children) > 0 {
		list.info = map[string]any{"Legacy Extension Count": len(list.children)}
		f.children = append(f.children, list)
	}
	id, err := d.scalar(f, "WAVE Element ID", 1)
	if err != nil {
		return err
	}
	length, err := d.scalar(f, "WSM Length", 2)
	if err != nil {
		return err
	}
	if length != len(d.wire)-d.pos {
		return fmt.Errorf("wsmp: WSM length does not match message boundary")
	}
	if id == 129 {
		supplement := wsmpBranch("Supplement")
		supplement.length = length
		for {
			if len(supplement.children) >= 256 {
				return fmt.Errorf("wsmp: supplement exceeds the 256-octet implementation limit")
			}
			if d.pos == len(d.wire) {
				return fmt.Errorf("wsmp: supplement has no terminating octet")
			}
			octet, err := d.scalar(&supplement, "Octet", 1)
			if err != nil {
				return err
			}
			if octet&128 == 0 {
				break
			}
		}
		f.children = append(f.children, supplement)
	}
	if d.pos < len(d.wire) {
		return d.raw(f, "WSM Data", len(d.wire)-d.pos)
	}
	return nil
}

func decodeWSMP(wire []byte) (wsmpField, error) {
	if len(wire) < 4 {
		return wsmpField{}, fmt.Errorf("wsmp: truncated minimum message")
	}
	if len(wire) > 65535 {
		return wsmpField{}, fmt.Errorf("wsmp: message exceeds the 65535-byte implementation boundary")
	}
	f := wsmpBranch("Version 2 Message")
	f.length = len(wire)
	d := wsmpDecoder{wire: wire}
	var err error
	if wire[0] == 2 {
		err = d.version2(&f)
	} else if wire[0]&7 == 3 {
		f.name = "Version 3 Message"
		err = d.version3(&f)
	} else {
		err = fmt.Errorf("wsmp: unsupported version")
	}
	if err != nil {
		return wsmpField{}, err
	}
	return f, nil
}

// Only these exact built-in expressions bypass the general expression VM.
// Results retain Yak's int type, and callers still wrap them with the original
// node. Modified expressions or value shapes use the normal evaluator.
const wsmpPSIDOut = "encoded = data.Value[0].Value\nvalue = 0\nfor _, octet = range encoded { value = (value << 8) | int(octet) }\nif len(encoded) == 2 { return (value & 16383) + 128 }\nif len(encoded) == 3 { return (value & 2097151) + 16512 }\nif len(encoded) == 4 { return (value & 268435455) + 2113664 }\nreturn value\n"
const wsmpLengthOut = "first = int(data.Value[0].Value)\nif first < 128 { return first }\nreturn ((first & 63) << 8) | int(data.Value[1].Value)\n"

func evalWSMPScalarOut(code string, data *base.NodeValue) (any, bool) {
	if data == nil || (code != wsmpPSIDOut && code != wsmpLengthOut) {
		return nil, false
	}
	children, ok := data.Value.([]*base.NodeValue)
	if !ok || len(children) == 0 || children[0] == nil {
		return nil, false
	}
	if code == wsmpPSIDOut {
		encoded, ok := children[0].Value.([]byte)
		if !ok || len(children) != 1 || len(encoded) < 1 || len(encoded) > 4 {
			return nil, false
		}
		value := 0
		for _, octet := range encoded {
			value = value<<8 | int(octet)
		}
		switch len(encoded) {
		case 2:
			value = value&16383 + 128
		case 3:
			value = value&2097151 + 16512
		case 4:
			value = value&268435455 + 2113664
		}
		return value, true
	}
	first, ok := children[0].Value.(uint8)
	if !ok {
		return nil, false
	}
	if first < 128 {
		return int(first), true
	}
	if len(children) != 2 || children[1] == nil {
		return nil, false
	}
	second, ok := children[1].Value.(uint8)
	if !ok {
		return nil, false
	}
	return (int(first)&63)<<8 | int(second), true
}
