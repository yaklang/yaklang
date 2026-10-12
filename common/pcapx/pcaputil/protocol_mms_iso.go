package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const mmsISOProfile = "mms-iso-ber"

type mmsPresentationContext struct {
	id        uint32
	syntax    []byte
	transfers [][]byte
}
type mmsISOCarrier struct {
	valueElements int
	elements      int
	application   *mmsBER
	fields        map[string]any
	code          byte
	src, dst      uint16
	tpduSize      int
	end           bool
	data          []byte
	session       byte
	offer         []mmsPresentationContext
	results       []bool
	acse          byte
	acseResult    uint32
	hasResult     bool
	pdu           []byte
}

func mmsFrameSize(w []byte, max int) (int, error) {
	if len(w) < 4 {
		return 0, nil
	}
	if w[0] != 3 || w[1] != 0 {
		return 0, protocolError(ErrMalformedMessage, "MMS RFC1006 version/reserved header")
	}
	n := int(binary.BigEndian.Uint16(w[2:]))
	if n < 7 {
		return 0, protocolError(ErrMalformedMessage, "MMS TPKT length below seven")
	}
	if n > max {
		return 0, protocolError(ErrResourceExceeded, "MMS TPKT frame budget")
	}
	return n, nil
}
func parseMMSCOTP(w []byte, b ParserBudget) (mmsISOCarrier, error) {
	d := mmsISOCarrier{tpduSize: 65531}
	n, err := mmsFrameSize(w, b.MaxFrameBytes)
	if err != nil {
		return d, err
	}
	if n == 0 || n != len(w) {
		return d, protocolError(ErrMalformedMessage, "MMS exact TPKT boundary")
	}
	t := w[4:]
	li := int(t[0]) + 1
	if li < 3 || li > len(t) {
		return d, protocolError(ErrMalformedMessage, "MMS COTP length indicator")
	}
	d.code = t[1] & 0xf0
	cotp := map[string]any{"Code": d.code, "Credit Bits": t[1] & 15, "Length Indicator": t[0]}
	d.fields = map[string]any{"TPKT Version": w[0], "TPKT Length": n, "COTP": cotp}
	switch d.code {
	case 0xe0, 0xd0:
		if li < 7 {
			return d, protocolError(ErrMalformedMessage, "MMS COTP connection fixed fields")
		}
		d.dst = binary.BigEndian.Uint16(t[2:])
		d.src = binary.BigEndian.Uint16(t[4:])
		cotp["Destination Reference"], cotp["Source Reference"], cotp["Class Options"] = d.dst, d.src, t[6]
		if t[6]&0xf0 != 0 {
			return d, protocolError(ErrUnsupportedFeature, "MMS selected COTP class0")
		}
		var ps []map[string]any
		seen := map[byte]bool{}
		for at := 7; at < li; {
			if len(ps) >= b.MaxCollectionElements {
				return d, protocolError(ErrResourceExceeded, "MMS COTP parameter budget")
			}
			if li-at < 2 {
				return d, protocolError(ErrMalformedMessage, "MMS COTP parameter header")
			}
			id, size := t[at], int(t[at+1])
			at += 2
			if size > li-at {
				return d, protocolError(ErrMalformedMessage, "MMS COTP parameter length")
			}
			if seen[id] {
				return d, protocolError(ErrMalformedMessage, "MMS COTP repeated negotiation parameter")
			}
			seen[id] = true
			v := t[at : at+size]
			at += size
			ps = append(ps, map[string]any{"ID": id, "Value": bytes.Clone(v)})
			if id == 0xc0 {
				if len(v) != 1 || v[0] < 7 || v[0] > 16 {
					return d, protocolError(ErrMalformedMessage, "MMS COTP TPDU size")
				}
				d.tpduSize = min(65531, 1<<v[0])
			}
		}
		d.elements = len(ps)
		cotp["Parameters"] = ps
		cotp["TPDU Size"] = d.tpduSize
		if li < len(t) {
			return d, protocolError(ErrUnsupportedFeature, "MMS COTP initial user data requires separate carrier profile")
		}
	case 0xf0:
		if li != 3 {
			return d, protocolError(ErrUnsupportedFeature, "MMS selected class0 DT header")
		}
		d.end = t[2]&128 != 0
		cotp["End Of TSDU"], cotp["TPDU Number Bits"] = d.end, t[2]&127
		d.data = t[li:]
		cotp["Fragment Bytes"] = bytes.Clone(d.data)
	default:
		return d, protocolError(ErrUnsupportedFeature, "MMS COTP TPDU outside selected connection/data carrier")
	}
	return d, nil
}
func parseMMSSession(w []byte, b ParserBudget, d *mmsISOCarrier, extended bool) error {
	if len(w) < 2 {
		return protocolError(ErrMalformedMessage, "MMS session SPDU header")
	}
	var user []byte
	var params []map[string]any
	if bytes.HasPrefix(w, []byte{1, 0, 1, 0}) {
		d.session = 1
		user = w[4:]
	} else {
		d.session = w[0]
		at := 2
		n := int(w[1])
		if n == 255 {
			if len(w)-at < 2 {
				return protocolError(ErrMalformedMessage, "MMS session extended length")
			}
			n = int(binary.BigEndian.Uint16(w[at:]))
			at += 2
		}
		if n != len(w)-at {
			return protocolError(ErrMalformedMessage, "MMS exact session SPDU length")
		}
		if d.session != 0x0d && d.session != 0x0e && d.session != 9 && d.session != 10 {
			return protocolError(ErrUnsupportedFeature, "MMS selected session connect/accept/data/finish/disconnect")
		}
		for at < len(w) {
			if len(params) >= b.MaxCollectionElements {
				return protocolError(ErrResourceExceeded, "MMS session parameter count")
			}
			if len(w)-at < 2 {
				return protocolError(ErrMalformedMessage, "MMS session parameter header")
			}
			id := w[at]
			n := int(w[at+1])
			at += 2
			if n == 255 {
				if len(w)-at < 2 {
					return protocolError(ErrMalformedMessage, "MMS session parameter extended length")
				}
				n = int(binary.BigEndian.Uint16(w[at:]))
				at += 2
			}
			if n > len(w)-at {
				return protocolError(ErrMalformedMessage, "MMS session parameter value")
			}
			v := w[at : at+n]
			at += n
			params = append(params, map[string]any{"ID": id, "Value": bytes.Clone(v)})
			if id == 0xc1 {
				if user != nil {
					return protocolError(ErrMalformedMessage, "MMS repeated session user-data")
				}
				user = v
			}
		}
	}
	d.fields["Session SPDU"] = map[string]any{"Code": d.session, "Parameters": params}
	if len(user) == 0 {
		return protocolError(ErrContextRequired, "MMS presentation user data missing")
	}
	remaining := b.MaxCollectionElements - len(params) - d.elements
	if remaining <= 0 {
		return protocolError(ErrResourceExceeded, "MMS shared session/BER budget")
	}
	q := mmsBERBudget{remaining, b.MaxRecursionDepth}
	nodes, err := q.list(user, 1)
	d.valueElements = q.left
	if err != nil {
		return err
	}
	if len(nodes) != 1 {
		return protocolError(ErrMalformedMessage, "MMS one presentation value required")
	}
	pres := nodes[0]
	d.fields["Presentation BER"] = mmsTree(pres)
	if d.session == 0x0d || d.session == 0x0e {
		if pres.tag != 0x31 {
			return protocolError(ErrMalformedMessage, "MMS presentation connect/accept SET")
		}
		mode, err := mmsChild(pres, 0xa0)
		if err != nil {
			return err
		}
		v, err := mmsChild(mode, 0x80)
		if err != nil {
			return err
		}
		m, err := mmsUInt(v, 31)
		if err != nil {
			return err
		}
		if m != 1 {
			return protocolError(ErrUnsupportedFeature, "MMS normal presentation mode required")
		}
		normal, err := mmsChild(pres, 0xa2)
		if err != nil {
			return err
		}
		if d.session == 0x0d {
			defs, err := mmsChild(normal, 0xa4)
			if err != nil {
				return err
			}
			seen := map[uint32]bool{}
			for _, c := range defs.children {
				if c.tag != 0x30 || len(c.children) != 3 || c.children[0].tag != 2 || c.children[1].tag != 6 || c.children[2].tag != 0x30 {
					return protocolError(ErrMalformedMessage, "MMS presentation context definition")
				}
				id, err := mmsUInt(c.children[0], 31)
				if err != nil {
					return err
				}
				if id == 0 || id%2 == 0 || seen[id] {
					return protocolError(ErrMalformedMessage, "MMS presentation context ID")
				}
				seen[id] = true
				ctx := mmsPresentationContext{id: id, syntax: bytes.Clone(c.children[1].body)}
				for _, tr := range c.children[2].children {
					if tr.tag != 6 {
						return protocolError(ErrMalformedMessage, "MMS transfer syntax OID")
					}
					ctx.transfers = append(ctx.transfers, bytes.Clone(tr.body))
				}
				d.offer = append(d.offer, ctx)
			}
		} else {
			results, err := mmsChild(normal, 0xa5)
			if err != nil {
				return err
			}
			for _, c := range results.children {
				if c.tag != 0x30 {
					return protocolError(ErrMalformedMessage, "MMS presentation result must be SEQUENCE")
				}
				if e := mmsSchema(c, []byte{0x80}, []byte{0x81, 0x82}); e != nil {
					return e
				}
				v, err := mmsChild(c, 0x80)
				if err != nil {
					return err
				}
				result, err := mmsUInt(v, 31)
				if err != nil {
					return err
				}
				if result > 2 {
					return protocolError(ErrMalformedMessage, "MMS presentation context result")
				}
				accepted := result == 0
				if accepted {
					v, err = mmsChild(c, 0x81)
					if err != nil {
						return err
					}
					if !bytes.Equal(v.body, []byte{0x51, 1}) {
						return protocolError(ErrUnsupportedFeature, "MMS selected BER transfer syntax")
					}
				}
				d.results = append(d.results, accepted)
			}
		}
		pres, err = mmsChild(normal, 0x61)
		if err != nil {
			return err
		}
	}
	if pres.tag != 0x61 || len(pres.children) != 1 {
		return protocolError(ErrUnsupportedFeature, "MMS one fully-encoded PDV list selected")
	}
	pdv := pres.children[0]
	if pdv.tag != 0x30 || len(pdv.children) != 2 || pdv.children[0].tag != 2 {
		return protocolError(ErrUnsupportedFeature, "MMS PDV with integer context and single ASN.1 value required")
	}
	id, err := mmsUInt(pdv.children[0], 31)
	if err != nil {
		return err
	}
	single := pdv.children[1]
	if single.tag != 0xa0 || len(single.children) != 1 {
		return protocolError(ErrUnsupportedFeature, "MMS single-ASN1 PDV data")
	}
	d.fields["Presentation Context ID"] = id
	body := single.children[0]
	if d.session == 1 {
		d.pdu = body.raw
		d.application = &body
		return nil
	}
	d.acse = body.tag
	if d.session == 13 && d.acse != 0x60 || d.session == 14 && d.acse != 0x61 || d.session == 9 && d.acse != 0x62 || d.session == 10 && d.acse != 0x63 {
		return protocolError(ErrMalformedMessage, "MMS session/ACSE phase mismatch")
	}
	for _, c := range body.children {
		if c.tag == 0x8a || c.tag == 0x8b || c.tag == 0xac {
			return protocolError(ErrUnsupportedFeature, "MMS authenticated ACSE requires dedicated validation")
		}
	}
	if body.tag != 0x60 && body.tag != 0x61 && body.tag != 0x62 && body.tag != 0x63 {
		return protocolError(ErrUnsupportedFeature, "MMS selected ACSE association/release")
	}
	d.fields["ACSE BER"] = mmsTree(body)
	seen := map[byte]bool{}
	for _, c := range body.children {
		if seen[c.tag] {
			return protocolError(ErrMalformedMessage, "MMS repeated ACSE field")
		}
		seen[c.tag] = true
	}
	for _, c := range body.children {
		if c.tag == 0x80 && (body.tag == 0x60 || body.tag == 0x61) {
			if err := mmsBits(c); err != nil {
				return err
			}
			if !bytes.Equal(c.body, []byte{7, 128}) {
				return protocolError(ErrUnsupportedVersion, "MMS selected ACSE version1")
			}
		}
	}
	if body.tag == 0x62 || body.tag == 0x63 {
		return nil
	}
	app, err := mmsChild(body, 0xa1)
	if err != nil {
		return err
	}
	oid, err := mmsChild(app, 6)
	if err != nil {
		return err
	}
	if !mmsOID(oid, "28ca220203") {
		return protocolError(ErrContextRequired, "MMS application-context OID is not selected")
	}
	if body.tag == 0x61 {
		v, err := mmsChild(body, 0xa2)
		if err != nil {
			return err
		}
		v, err = mmsChild(v, 2)
		if err != nil {
			return err
		}
		d.acseResult, err = mmsUInt(v, 31)
		if err != nil {
			return err
		}
		if d.acseResult > 2 {
			return protocolError(ErrMalformedMessage, "MMS ACSE result enumeration")
		}
		d.hasResult = true
		diag, e := mmsChild(body, 0xa3)
		if e != nil {
			return e
		}
		source, e := mmsSingleChild(diag)
		if e != nil {
			return e
		}
		if source.tag != 0xa1 && source.tag != 0xa2 {
			return protocolError(ErrMalformedMessage, "MMS ACSE diagnostic source")
		}
		value, e := mmsSingleChild(source)
		if e != nil {
			return e
		}
		if value.tag != 2 {
			return protocolError(ErrMalformedMessage, "MMS ACSE diagnostic INTEGER")
		}
		n, e := mmsUInt(value, 31)
		if e != nil {
			return e
		}
		if d.acseResult == 0 && n != 0 {
			return protocolError(ErrMalformedMessage, "MMS accepted association with nonzero diagnostic")
		}
	}
	ui, err := mmsChild(body, 0xbe)
	if err != nil {
		return err
	}
	if len(ui.children) != 1 || ui.children[0].tag != 0x28 {
		return protocolError(ErrUnsupportedFeature, "MMS one ACSE EXTERNAL user-data required")
	}
	ex := ui.children[0]
	if !extended && len(ex.children) != 2 {
		return protocolError(ErrUnsupportedFeature, "MMS EXTERNAL indirect-reference/single-ASN1 selected")
	}
	if extended {
		seen := map[byte]bool{}
		last := -1
		rank := map[byte]int{6: 0, 2: 1, 7: 2, 0xa0: 3}
		for _, v := range ex.children {
			order, ok := rank[v.tag]
			if !ok {
				return protocolError(ErrUnsupportedFeature, "MMS EXTERNAL selected references/descriptor/single-ASN1")
			}
			if seen[v.tag] || order < last {
				return protocolError(ErrMalformedMessage, "MMS EXTERNAL field order/duplicate")
			}
			seen[v.tag] = true
			last = order
			if v.tag == 6 {
				if !bytes.Equal(v.body, []byte{0x51, 1}) {
					return protocolError(ErrUnsupportedFeature, "MMS EXTERNAL selected BER direct reference")
				}
				if d.valueElements < 2 {
					return protocolError(ErrResourceExceeded, "MMS shared direct reference budget")
				}
				d.valueElements -= 2
				d.fields["EXTERNAL Direct Reference"] = map[string]any{"OID": "2.1.1", "Bytes": bytes.Clone(v.body)}
			}
			if v.tag == 7 {
				d.fields["EXTERNAL Descriptor Bytes"] = bytes.Clone(v.body)
			}
		}
	}
	ref, err := mmsChild(ex, 2)
	if err != nil {
		return err
	}
	indirect, err := mmsUInt(ref, 31)
	if err != nil {
		return err
	}
	enc, err := mmsChild(ex, 0xa0)
	if err != nil {
		return err
	}
	if len(enc.children) != 1 {
		return protocolError(ErrMalformedMessage, "MMS EXTERNAL single application value")
	}
	d.fields["MMS Context ID"] = indirect
	d.pdu = enc.children[0].raw
	d.application = &enc.children[0]
	return nil
}
func mmsContextRole(cs []mmsPresentationContext, id uint32) (string, bool) {
	for _, c := range cs {
		if c.id != id {
			continue
		}
		ber := false
		for _, t := range c.transfers {
			ber = ber || bytes.Equal(t, []byte{0x51, 1})
		}
		if !ber {
			return "", false
		}
		switch hex.EncodeToString(c.syntax) {
		case "52010001":
			return "acse", true
		case "28ca220201":
			return "mms", true
		}
		return "", false
	}
	return "", false
}
func (f *binFlow) frameMMSISO(w []byte) (int, *binSpec, error) {
	n, err := mmsFrameSize(w, min(f.a.budget.MaxFrameBytes, f.a.config.MaxMessageBytes))
	if err != nil {
		return 0, nil, err
	}
	if n > 0 && n <= len(w) {
		if err = f.mmsReserve(f.mms.storage() + 32768 + f.mms.projectionFactor(f.a.budget)*int64(n)); err != nil {
			return 0, nil, err
		}
	}
	return n, &binSpec{}, nil
}
func WithProtocolMMSISO(port uint16) CaptureOption {
	return func(c *CaptureConfig) error {
		if port == 0 {
			return fmt.Errorf("MMS ISO port must be nonzero")
		}
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.MMSISOPorts = append(c.binParserConfig.MMSISOPorts, port)
		return nil
	}
}
func WithSessionMMSISO() ProtocolSessionOption {
	return func(s *captureSession) error {
		if s.transport == "udp" {
			return fmt.Errorf("MMS ISO requires TCP")
		}
		s.f.protocol = "mms"
		s.f.mms = newBinMMS()
		return nil
	}
}
