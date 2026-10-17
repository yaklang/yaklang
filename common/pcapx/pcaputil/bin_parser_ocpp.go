package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Selected OCPP-J 1.6/2.0.1 BootNotification/Heartbeat observations. Association is
// local to a successfully negotiated WebSocket; no station authorization or
// reconnect identity is inferred from an HTTP path or a successful response.
type ocppCall struct {
	action string
	event  uint64
}
type binOCPP struct {
	version string
	seen    [2]map[string]ocppCall
	pending [2]map[string]ocppCall
}

func (o *binOCPP) retainedBytes() int64 {
	if o == nil {
		return 0
	}
	n := int64(256)
	for dir := range o.seen {
		for id, call := range o.seen[dir] {
			n += int64(128 + len(id) + len(call.action))
		}
		for id, call := range o.pending[dir] {
			n += int64(128 + len(id) + len(call.action))
		}
	}
	return n
}

// Charge every JSON member/entry before retaining it, including CALLERROR
// details. Reject ambiguous duplicate keys instead of silently choosing a value.
func ocppJSON(w []byte, elements, maxDepth int) (any, error) {
	if !utf8.Valid(w) {
		return nil, fmt.Errorf("OCPP JSON is not UTF-8")
	}
	if !ocppUnicodeEscapes(w) {
		return nil, fmt.Errorf("OCPP unpaired Unicode escape cannot preserve field identity")
	}
	d := json.NewDecoder(bytes.NewReader(w))
	d.UseNumber()
	count := 0
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		t, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("OCPP JSON syntax")
		}
		open, collection := t.(json.Delim)
		if !collection {
			return t, nil
		}
		if depth > maxDepth {
			return nil, protocolError(ErrResourceExceeded, "OCPP JSON depth")
		}
		if open != '[' && open != '{' {
			return nil, fmt.Errorf("OCPP JSON delimiter")
		}
		obj := map[string]any{}
		array := []any{}
		for d.More() {
			count++
			if count > elements {
				return nil, protocolError(ErrResourceExceeded, "OCPP JSON elements")
			}
			key := ""
			if open == '{' {
				k, e := d.Token()
				if e != nil {
					return nil, fmt.Errorf("OCPP JSON key")
				}
				var ok bool
				key, ok = k.(string)
				if !ok {
					return nil, fmt.Errorf("OCPP JSON key type")
				}
				if _, ok := obj[key]; ok {
					return nil, fmt.Errorf("OCPP duplicate JSON key")
				}
			}
			v, e := read(depth + 1)
			if e != nil {
				return nil, e
			}
			if open == '{' {
				obj[key] = v
			} else {
				array = append(array, v)
			}
		}
		end, e := d.Token()
		if e != nil || open == '{' && end != json.Delim('}') || open == '[' && end != json.Delim(']') {
			return nil, fmt.Errorf("OCPP JSON closing delimiter")
		}
		if open == '{' {
			return obj, nil
		}
		return array, nil
	}
	v, err := read(1)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, fmt.Errorf("OCPP trailing JSON")
	}
	return v, nil
}

// encoding/json replaces unpaired surrogate escapes with U+FFFD. RFC8259
// section 8.2 describes their unpredictable interoperability; this selected
// profile refuses them before decoding so distinct IDs cannot collapse.
func ocppUnicodeEscapes(w []byte) bool {
	quoted := false
	unit := func(at int) (uint64, bool) {
		if at+6 > len(w) || w[at] != '\\' || w[at+1] != 'u' {
			return 0, false
		}
		n, err := strconv.ParseUint(string(w[at+2:at+6]), 16, 16)
		return n, err == nil
	}
	for i := 0; i < len(w); i++ {
		if w[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || w[i] != '\\' {
			continue
		}
		n, valid := unit(i)
		if valid && n >= 0xd800 && n <= 0xdfff {
			if n >= 0xdc00 {
				return false
			}
			low, ok := unit(i + 6)
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 11
		} else if valid {
			i += 5
		} else {
			i++
		}
	}
	return true
}

var ocppDatePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}[Tt][0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?([Zz]|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)

func ocppDate(v any) bool {
	s, ok := v.(string)
	if !ok || !ocppDatePattern.MatchString(s) {
		return false
	}
	s = strings.ToUpper(s)
	// RFC3339 permits leap seconds. This syntax check cannot establish that
	// the advertised timestamp corresponds to an actual UTC insertion event.
	if s[17:19] == "60" {
		s = s[:17] + "59" + s[19:]
	}
	_, err := time.Parse(time.RFC3339Nano, s)
	return err == nil
}

func ocppPayload(action string, response bool, p map[string]any) error {
	allowed := map[string]int{}
	required := []string{}
	if !response && action == "BootNotification" {
		allowed = map[string]int{"chargePointVendor": 20, "chargePointModel": 20, "chargePointSerialNumber": 25, "chargeBoxSerialNumber": 25, "firmwareVersion": 50, "iccid": 20, "imsi": 20, "meterType": 25, "meterSerialNumber": 25}
		required = []string{"chargePointVendor", "chargePointModel"}
	} else if response {
		allowed["currentTime"] = -1
		required = []string{"currentTime"}
		if action == "BootNotification" {
			allowed["status"] = -1
			allowed["interval"] = -1
			required = append(required, "status", "interval")
		}
	}
	for _, k := range required {
		if _, ok := p[k]; !ok {
			return fmt.Errorf("OCPP missing %s", k)
		}
	}
	for k, v := range p {
		limit, ok := allowed[k]
		if !ok {
			return fmt.Errorf("OCPP additional property %s", k)
		}
		if limit >= 0 {
			s, ok := v.(string)
			if !ok || utf8.RuneCountInString(s) > limit {
				return fmt.Errorf("OCPP invalid %s", k)
			}
		}
	}
	if response {
		if !ocppDate(p["currentTime"]) {
			return fmt.Errorf("OCPP invalid currentTime")
		}
		if action == "BootNotification" {
			if p["status"] != "Accepted" && p["status"] != "Pending" && p["status"] != "Rejected" {
				return fmt.Errorf("OCPP invalid status")
			}
			n, ok := p["interval"].(json.Number)
			if !ok || !semtechExactInteger(string(n)) {
				return fmt.Errorf("OCPP interval must be an exact integer")
			}
		}
	}
	return nil
}

func (f *binFlow) consumeOCPP(dir int, e *ProtocolEvent) error {
	ws := f.ws
	if ws == nil || ws.ocpp == nil {
		return nil
	}
	if e.Session["Opcode"] == uint64(8) {
		ws.ocpp = nil
		return nil
	}
	w, complete := e.Session["Application"].([]byte)
	if !complete || e.Session["Opcode"] != uint64(1) {
		return nil
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	e.ocpp = true
	info := map[string]any{"Version": ws.ocpp.version, "Context Status": "malformed"}
	e.Session["OCPP"] = info
	diagnostic := func(status, reason string) error { info["Context Status"], info["Reason"] = status, reason; return nil }
	// Reserve bounded transient JSON/projection space before allocating a tree.
	if err := f.reserveSession(ws.retainedBytes() + int64(8*len(w))); err != nil {
		return err
	}
	v, err := ocppJSON(w, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	if err != nil {
		if p, ok := err.(*ProtocolError); ok {
			return p
		}
		ws.ocpp.pending[1-dir] = nil
		return diagnostic("malformed", err.Error())
	}
	a, ok := v.([]any)
	if !ok || len(a) < 2 {
		return diagnostic("malformed", "OCPP RPC must be an array")
	}
	typ, ok := a[0].(json.Number)
	number, numberErr := typ.Float64()
	if !ok || numberErr != nil || !semtechExactInteger(string(typ)) || number != 2 && number != 3 && number != 4 {
		return diagnostic("unsupported", "OCPP message type")
	}
	typ = json.Number(fmt.Sprint(int(number)))
	id, ok := a[1].(string)
	if !ok || utf8.RuneCountInString(id) > 36 {
		return diagnostic("malformed", "OCPP unique ID type or length")
	}
	info["Unique ID"], info["Message Type"] = id, string(typ)
	o := ws.ocpp
	if typ == "2" {
		if len(a) != 4 {
			return diagnostic("malformed", "CALL needs four elements")
		}
		action, ok := a[2].(string)
		if !ok {
			return diagnostic("malformed", "CALL action must be a string")
		}
		info["Action"] = action
		if _, used := o.seen[dir][id]; used {
			delete(o.pending[dir], id)
			return diagnostic("context-required", "CALL ID was already used by this sender")
		}
		if len(o.seen[dir]) >= f.a.budget.MaxCollectionElements {
			return protocolError(ErrResourceExceeded, "OCPP retained call IDs")
		}
		if err := f.reserveSession(ws.retainedBytes() + int64(256+2*len(id)+2*len(action))); err != nil {
			return err
		}
		if o.seen[dir] == nil {
			o.seen[dir] = map[string]ocppCall{}
		}
		o.seen[dir][id] = ocppCall{action, e.ID}
		if action != "BootNotification" && action != "Heartbeat" {
			return diagnostic("unsupported", "OCPP action is outside the selected profile")
		}
		if dir != ws.client {
			return diagnostic("malformed", "selected charge-point CALL has wrong direction")
		}
		p, ok := a[3].(map[string]any)
		if !ok {
			return diagnostic("malformed", "CALL payload must be an object")
		}
		info["Payload"] = p
		if err := o.payload(action, false, p); err != nil {
			return diagnostic("malformed", err.Error())
		}
		if o.pending[dir] == nil {
			o.pending[dir] = map[string]ocppCall{}
		}
		o.pending[dir][id] = ocppCall{action, e.ID}
		e.TransactionID = e.ID
		info["Context Status"] = "observed"
		return nil
	}
	call, found := o.pending[1-dir][id]
	if !found {
		return diagnostic("context-required", "response has no observed opposite-direction CALL")
	}
	// A response with this ID retires the pending request even when invalid.
	// The seen-ID tombstone prevents a late reply from binding to a reused ID.
	delete(o.pending[1-dir], id)
	info["Action"] = call.action
	if typ == "3" {
		if len(a) != 3 {
			return diagnostic("malformed", "CALLRESULT needs three elements")
		}
		p, ok := a[2].(map[string]any)
		if !ok {
			return diagnostic("malformed", "CALLRESULT payload must be an object")
		}
		info["Payload"] = p
		if err := o.payload(call.action, true, p); err != nil {
			return diagnostic("malformed", err.Error())
		}
	} else {
		if len(a) != 5 {
			return diagnostic("malformed", "CALLERROR needs five elements")
		}
		code, ok := a[2].(string)
		if !ok {
			return diagnostic("malformed", "CALLERROR code type")
		}
		if !o.errorCode(code) {
			return diagnostic("malformed", "CALLERROR unknown code")
		}
		description, ok := a[3].(string)
		if !ok {
			return diagnostic("malformed", "CALLERROR description type")
		}
		details, ok := a[4].(map[string]any)
		if !ok {
			return diagnostic("malformed", "CALLERROR details must be an object")
		}
		info["Error Code"], info["Error Description"], info["Error Details"] = code, description, details
	}
	e.ResponseTo, e.TransactionID = call.event, call.event
	info["Context Status"] = "matched"
	return nil
}

func (o *binOCPP) payload(action string, response bool, p map[string]any) error {
	if o.version == "2.0.1" {
		return ocpp201Payload(action, response, p)
	}
	return ocppPayload(action, response, p)
}

func (o *binOCPP) errorCode(code string) bool {
	switch code {
	case "NotImplemented", "NotSupported", "InternalError", "ProtocolError", "SecurityError", "PropertyConstraintViolation", "TypeConstraintViolation", "GenericError":
		return true
	case "FormationViolation", "OccurenceConstraintViolation":
		return o.version == "1.6"
	case "FormatViolation", "OccurrenceConstraintViolation", "MessageTypeNotSupported", "RpcFrameworkError":
		return o.version == "2.0.1"
	}
	return false
}
