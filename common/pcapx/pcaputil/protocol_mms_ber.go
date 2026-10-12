package pcaputil

import (
	"bytes"
	"encoding/hex"
)

type mmsBER struct {
	tag       byte
	body, raw []byte
	children  []mmsBER
}
type mmsBERBudget struct{ left, depth int }

func (b *mmsBERBudget) list(w []byte, depth int) ([]mmsBER, error) {
	if depth > b.depth {
		return nil, protocolError(ErrResourceExceeded, "MMS BER depth")
	}
	var nodes []mmsBER
	for at := 0; at < len(w); {
		if b.left <= 0 {
			return nil, protocolError(ErrResourceExceeded, "MMS BER collection")
		}
		b.left--
		start := at
		tag := w[at]
		at++
		if tag&31 == 31 {
			return nil, protocolError(ErrUnsupportedFeature, "MMS high-tag-number profile")
		}
		if len(w)-at < 1 {
			return nil, protocolError(ErrMalformedMessage, "MMS BER length is truncated")
		}
		n := uint64(w[at])
		at++
		if n&128 != 0 {
			c := int(n & 127)
			if c == 0 {
				return nil, protocolError(ErrUnsupportedFeature, "MMS indefinite BER length")
			}
			if c > 4 || c > len(w)-at {
				return nil, protocolError(ErrMalformedMessage, "MMS BER long length")
			}
			n = 0
			for i := 0; i < c; i++ {
				n = n<<8 | uint64(w[at+i])
			}
			at += c
		}
		if n > uint64(len(w)-at) {
			return nil, protocolError(ErrMalformedMessage, "MMS BER value is truncated")
		}
		v := mmsBER{tag: tag, body: w[at : at+int(n)]}
		at += int(n)
		v.raw = w[start:at]
		if tag&32 != 0 {
			var err error
			v.children, err = b.list(v.body, depth+1)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, v)
	}
	return nodes, nil
}
func mmsOne(w []byte, b ParserBudget) (mmsBER, error) {
	q := mmsBERBudget{b.MaxCollectionElements, b.MaxRecursionDepth}
	n, err := q.list(w, 1)
	if err != nil {
		return mmsBER{}, err
	}
	if len(n) != 1 {
		return mmsBER{}, protocolError(ErrMalformedMessage, "MMS wrapper must contain exactly one BER value")
	}
	return n[0], nil
}
func mmsChild(n mmsBER, tag byte) (mmsBER, error) {
	var found *mmsBER
	for i := range n.children {
		if n.children[i].tag == tag {
			if found != nil {
				return mmsBER{}, protocolError(ErrMalformedMessage, "MMS duplicate mandatory BER field")
			}
			found = &n.children[i]
		}
	}
	if found == nil {
		return mmsBER{}, protocolError(ErrMalformedMessage, "MMS mandatory BER field missing")
	}
	return *found, nil
}
func mmsUInt(n mmsBER, bits uint) (uint32, error) {
	w := n.body
	if len(w) == 0 || len(w) > 5 || w[0]&128 != 0 || len(w) > 1 && w[0] == 0 && w[1]&128 == 0 {
		return 0, protocolError(ErrMalformedMessage, "MMS nonnegative integer encoding")
	}
	var value uint64
	for _, v := range w {
		value = value<<8 | uint64(v)
	}
	if value > (uint64(1)<<bits)-1 {
		return 0, protocolError(ErrMalformedMessage, "MMS integer exceeds selected width")
	}
	return uint32(value), nil
}
func mmsBits(n mmsBER) error {
	if len(n.body) < 1 || n.body[0] > 7 || len(n.body) == 1 && n.body[0] != 0 {
		return protocolError(ErrMalformedMessage, "MMS bit-string padding")
	}
	if len(n.body) > 1 && n.body[len(n.body)-1]&byte((1<<n.body[0])-1) != 0 {
		return protocolError(ErrMalformedMessage, "MMS bit-string unused bits are nonzero")
	}
	return nil
}
func mmsTree(n mmsBER) map[string]any {
	out := map[string]any{"Tag": n.tag, "Value": bytes.Clone(n.body)}
	if n.tag&32 != 0 {
		cs := make([]map[string]any, len(n.children))
		for i, c := range n.children {
			cs[i] = mmsTree(c)
		}
		out["Children"] = cs
	}
	return out
}

var mmsPDUNames = map[byte]string{0xa0: "Confirmed Request", 0xa1: "Confirmed Response", 0xa2: "Confirmed Error", 0xa8: "Initiate Request", 0xa9: "Initiate Response", 0x8b: "Conclude Request", 0x8c: "Conclude Response"}

type mmsDecoded struct {
	fields                   map[string]any
	tag, service             byte
	invoke                   uint32
	calling, called, version uint32
	items                    int
	valueDepth               uint32
	services                 []byte
}

func decodeMMSPDU(w []byte, b ParserBudget) (mmsDecoded, error) {
	n, err := mmsOne(w, b)
	if err != nil {
		return mmsDecoded{}, err
	}
	return decodeMMSNode(n, nil)
}
func decodeMMSNode(n mmsBER, values *mmsValueContext) (mmsDecoded, error) {
	w := n.raw
	var err error
	name, ok := mmsPDUNames[n.tag]
	if !ok {
		return mmsDecoded{}, protocolError(ErrUnsupportedFeature, "MMS PDU outside selected service grammar")
	}
	d := mmsDecoded{tag: n.tag, items: -1}
	d.fields = map[string]any{"PDU Tag": n.tag, "PDU Name": name, "Raw PDU": bytes.Clone(w)}
	switch n.tag {
	case 0xa8, 0xa9:
		a, e := mmsChild(n, 0x81)
		if e != nil {
			return d, e
		}
		d.calling, e = mmsUInt(a, 15)
		if e != nil {
			return d, protocolError(ErrMalformedMessage, "MMS outstanding calling limit")
		}
		a, e = mmsChild(n, 0x82)
		if e != nil {
			return d, e
		}
		d.called, e = mmsUInt(a, 15)
		if e != nil {
			return d, protocolError(ErrMalformedMessage, "MMS outstanding called limit")
		}
		detail, e := mmsChild(n, 0xa4)
		if e != nil {
			return d, e
		}
		v, e := mmsChild(detail, 0x80)
		if e != nil {
			return d, e
		}
		d.version, e = mmsUInt(v, 15)
		if e != nil {
			return d, e
		}
		if d.version != 1 {
			return d, protocolError(ErrUnsupportedVersion, "MMS version1 is selected")
		}
		for _, tag := range []byte{0x81, 0x82} {
			v, e := mmsChild(detail, tag)
			if e != nil {
				return d, e
			}
			if e = mmsBits(v); e != nil {
				return d, e
			}
		}
		for _, c := range n.children {
			if c.tag == 0x80 {
				if _, e = mmsUInt(c, 31); e != nil {
					return d, e
				}
			}
			if c.tag == 0x83 {
				if _, e = mmsUInt(c, 7); e != nil {
					return d, e
				}
			}
		}
	case 0xa0, 0xa1:
		if len(n.children) != 2 || n.children[0].tag != 2 {
			return d, protocolError(ErrUnsupportedFeature, "MMS confirmed modifiers/details need their selected profile")
		}
		d.invoke, err = mmsUInt(n.children[0], 32)
		if err != nil {
			return d, err
		}
		s := n.children[1]
		d.service = s.tag & 31
		if (s.tag != 0xa4 && s.tag != 0xa6) && (values == nil || !mmsSelectedService(n.tag, s.tag)) {
			return d, protocolError(ErrUnsupportedFeature, "MMS selected Read/GetVariableAccessAttributes service")
		}
		d.fields["Invoke ID"], d.fields["Service Tag"], d.fields["Service Name"] = d.invoke, s.tag, map[byte]string{0: "Status", 2: "Identify", 4: "Read", 6: "Get Variable Access Attributes"}[d.service]
		if d.service == 4 && d.tag == 0xa0 {
			va, e := mmsChild(s, 0xa1)
			if e != nil {
				return d, e
			}
			ls, e := mmsChild(va, 0xa0)
			if e != nil {
				return d, protocolError(ErrContextRequired, "MMS named variable-list cardinality requires definition context")
			}
			d.items = len(ls.children)
			for _, v := range ls.children {
				if v.tag != 0x30 {
					return d, protocolError(ErrMalformedMessage, "MMS variable entry must be a sequence")
				}
			}
		}
		if d.service == 4 && d.tag == 0xa1 {
			ls, e := mmsChild(s, 0xa1)
			if e != nil {
				return d, e
			}
			d.items = len(ls.children)
		}
	case 0xa2:
		a, e := mmsChild(n, 0x80)
		if e != nil {
			return d, e
		}
		d.invoke, e = mmsUInt(a, 32)
		if e != nil {
			return d, e
		}
		if _, e = mmsChild(n, 0xa2); e != nil {
			return d, e
		}
		d.fields["Invoke ID"] = d.invoke
	case 0x8b, 0x8c:
		if len(n.body) != 0 {
			return d, protocolError(ErrMalformedMessage, "MMS conclude NULL has content")
		}
	}
	d.fields["BER"] = mmsTree(n)
	if err = mmsSelectedFields(n, &d, values); err != nil {
		return d, err
	}
	return d, nil
}
func mmsOID(n mmsBER, want string) bool { return n.tag == 6 && hex.EncodeToString(n.body) == want }
