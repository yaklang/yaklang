package pcaputil

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// Sparkplug 3.0 selected node scalar observations. MQTT delivery acknowledgments
// are independent of the birth/metric sequence. A broker-forwarded publication
// cannot borrow this connection's client Will to prove another node's birth.
type sparkMetricDef struct {
	typ uint64
}

type sparkNode struct {
	key     string
	bdSeq   uint64
	born    bool
	seq     uint64
	metrics map[string]sparkMetricDef
	aliases map[uint64]string
	devices map[string]*sparkDevice
}

func (s *sparkNode) retainedBytes() int64 {
	if s == nil {
		return 0
	}
	n := int64(128 + len(s.key))
	for name := range s.metrics {
		n += int64(96 + len(name))
	}
	for id, d := range s.devices {
		n += int64(128 + len(id) + 64*len(d.aliases))
		for name := range d.metrics {
			n += int64(96 + len(name))
		}
	}
	return n + int64(64*len(s.aliases))
}

// Count every wire field (including unknown fields/groups) before allocating
// metric collections. Length-delimited unknown extensions remain opaque.
func sparkWalk(w []byte, depth, maxDepth, maxFields int, count *int, visit func(protowire.Number, protowire.Type, []byte) error) error {
	if depth > maxDepth {
		return protocolError(ErrResourceExceeded, "Sparkplug protobuf nesting")
	}
	for len(w) > 0 {
		num, typ, n := protowire.ConsumeTag(w)
		if n < 0 || num <= 0 || num > protowire.MaxValidNumber {
			return fmt.Errorf("Sparkplug invalid protobuf tag")
		}
		wire := w
		w = w[n:]
		*count++
		if *count > maxFields {
			return protocolError(ErrResourceExceeded, "Sparkplug protobuf fields")
		}
		length := 0
		switch typ {
		case protowire.VarintType:
			_, length = protowire.ConsumeVarint(w)
		case protowire.Fixed32Type:
			_, length = protowire.ConsumeFixed32(w)
		case protowire.Fixed64Type:
			_, length = protowire.ConsumeFixed64(w)
		case protowire.BytesType:
			_, length = protowire.ConsumeBytes(w)
		case protowire.StartGroupType:
			length = sparkGroup(w, num, depth+1, maxDepth, maxFields, count)
		default:
			return fmt.Errorf("Sparkplug invalid protobuf wire type")
		}
		if length < 0 {
			if length == -100 {
				return protocolError(ErrResourceExceeded, "Sparkplug protobuf group budget")
			}
			return fmt.Errorf("Sparkplug truncated protobuf field")
		}
		if visit != nil {
			if err := visit(num, typ, wire[:n+length]); err != nil {
				return err
			}
		}
		w = w[length:]
	}
	return nil
}

func sparkGroup(w []byte, group protowire.Number, depth, maxDepth, maxFields int, count *int) int {
	if depth > maxDepth {
		return -100
	}
	at := 0
	for at < len(w) {
		num, typ, n := protowire.ConsumeTag(w[at:])
		if n < 0 || num <= 0 || num > protowire.MaxValidNumber {
			return -1
		}
		at += n
		*count++
		if *count > maxFields {
			return -100
		}
		if typ == protowire.EndGroupType {
			if num == group {
				return at
			}
			return -1
		}
		var length int
		switch typ {
		case protowire.StartGroupType:
			length = sparkGroup(w[at:], num, depth+1, maxDepth, maxFields, count)
		case protowire.VarintType:
			_, length = protowire.ConsumeVarint(w[at:])
		case protowire.Fixed32Type:
			_, length = protowire.ConsumeFixed32(w[at:])
		case protowire.Fixed64Type:
			_, length = protowire.ConsumeFixed64(w[at:])
		case protowire.BytesType:
			_, length = protowire.ConsumeBytes(w[at:])
		default:
			return -1
		}
		if length < 0 {
			return length
		}
		at += length
	}
	return -1
}

func sparkDecode(w []byte, maxDepth, maxFields int) (map[string]any, error) {
	_, err := sparkPreflight(w, maxDepth, maxFields)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	metrics := []map[string]any{}
	unknown := []byte{}
	count := 0
	err = sparkWalk(w, 1, maxDepth, maxFields, &count, func(n protowire.Number, typ protowire.Type, v []byte) error {
		raw := v
		_, _, tag := protowire.ConsumeTag(v)
		v = v[tag:]
		switch {
		case (n == 1 || n == 3) && typ == protowire.VarintType:
			u, _ := protowire.ConsumeVarint(v)
			name := "timestamp"
			if n == 3 {
				name = "seq"
			}
			fields[name] = u
		case n == 2 && typ == protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			m, err := sparkDecodeMetric(b, maxDepth, maxFields)
			if err != nil {
				return err
			}
			metrics = append(metrics, m)
		case (n == 4 || n == 5) && typ == protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			if n == 4 {
				if !utf8.Valid(b) {
					return fmt.Errorf("Sparkplug invalid UTF-8 uuid")
				}
				fields["uuid"] = string(b)
			} else {
				fields["body"] = append([]byte(nil), b...)
			}
		default:
			unknown = append(unknown, raw...)
		}
		return nil
	})
	if len(metrics) > 0 {
		fields["metrics"] = metrics
	}
	if len(unknown) > 0 {
		fields["unknown_hex"] = hex.EncodeToString(unknown)
	}
	return fields, err
}

func sparkDecodeMetric(w []byte, maxDepth, maxFields int) (map[string]any, error) {
	fields := map[string]any{}
	names := map[protowire.Number]string{1: "name", 2: "alias", 3: "timestamp", 4: "datatype", 5: "is_historical", 6: "is_transient", 7: "is_null", 8: "metadata", 9: "properties", 10: "int_value", 11: "long_value", 12: "float_value", 13: "double_value", 14: "boolean_value", 15: "string_value", 16: "bytes_value", 17: "dataset_value", 18: "template_value", 19: "extension_value"}
	unknown := []byte{}
	count := 0
	err := sparkWalk(w, 2, maxDepth, maxFields, &count, func(n protowire.Number, typ protowire.Type, v []byte) error {
		raw := v
		_, _, tag := protowire.ConsumeTag(v)
		v = v[tag:]
		name, known := names[n]
		want := protowire.VarintType
		if n == 1 || n == 8 || n == 9 || n >= 15 {
			want = protowire.BytesType
		} else if n == 12 {
			want = protowire.Fixed32Type
		} else if n == 13 {
			want = protowire.Fixed64Type
		}
		if !known || typ != want {
			unknown = append(unknown, raw...)
			return nil
		}
		// Scalar oneofs replace the prior member. Repeated occurrences of the
		// same message-valued DataSet member merge their encoded bodies; a
		// different correctly typed oneof clears them. Wrong wire types stay unknown.
		if n >= 10 && !(n == 17 && fields["dataset_value"] != nil) {
			for k := protowire.Number(10); k <= 19; k++ {
				delete(fields, names[k])
			}
		}
		switch typ {
		case protowire.VarintType:
			u, _ := protowire.ConsumeVarint(v)
			if n == 5 || n == 6 || n == 7 || n == 14 {
				fields[name] = u != 0
			} else if n == 4 || n == 10 {
				fields[name] = uint64(uint32(u))
			} else {
				fields[name] = u
			}
		case protowire.Fixed32Type:
			u, _ := protowire.ConsumeFixed32(v)
			fields[name] = math.Float32frombits(u)
		case protowire.Fixed64Type:
			u, _ := protowire.ConsumeFixed64(v)
			fields[name] = math.Float64frombits(u)
		case protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			if n == 1 || n == 15 {
				if !utf8.Valid(b) {
					return fmt.Errorf("Sparkplug invalid UTF-8 metric string")
				}
				fields[name] = string(b)
			} else if n == 17 {
				prior, _ := fields[name].([]byte)
				fields[name] = append(prior, b...)
			} else {
				fields[name] = append([]byte(nil), b...)
			}
		}
		return nil
	})
	if len(unknown) > 0 {
		fields["unknown_hex"] = hex.EncodeToString(unknown)
	}
	return fields, err
}

func sparkValue(m map[string]any, typ uint64, maxDepth, maxFields int) (any, string) {
	if m["metadata"] != nil || m["properties"] != nil {
		return nil, "metadata/properties need an independent structured profile"
	}
	switch typ {
	case 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17:
	default:
		return nil, "datatype needs an independent structured profile"
	}
	if m["is_null"] == true {
		for _, key := range []string{"int_value", "long_value", "float_value", "double_value", "boolean_value", "string_value", "bytes_value", "dataset_value", "template_value", "extension_value"} {
			if _, present := m[key]; present {
				return nil, "null metric carries a value"
			}
		}
		return nil, ""
	}
	if typ == 16 {
		w, ok := m["dataset_value"].([]byte)
		if !ok {
			return nil, "datatype/value mismatch"
		}
		return sparkDataSet(w, maxDepth, maxFields)
	}
	name := ""
	switch typ {
	case 1, 2, 3, 5, 6, 7:
		name = "int_value"
	case 4, 8, 13:
		name = "long_value"
	case 9:
		name = "float_value"
	case 10:
		name = "double_value"
	case 11:
		name = "boolean_value"
	case 12, 14, 15:
		name = "string_value"
	case 17:
		name = "bytes_value"
	default:
		return nil, "datatype needs an independent structured profile"
	}
	v, ok := m[name]
	if !ok {
		return nil, "datatype/value mismatch"
	}
	switch typ {
	case 1:
		return int64(int8(uint32(v.(uint64)))), ""
	case 2:
		return int64(int16(uint32(v.(uint64)))), ""
	case 3:
		return int64(int32(uint32(v.(uint64)))), ""
	case 4:
		return int64(v.(uint64)), ""
	case 5, 6:
		max := uint64(255)
		if typ == 6 {
			max = 65535
		}
		if v.(uint64) > max {
			return nil, "unsigned metric exceeds datatype width"
		}
	}
	return v, ""
}

func sparkTopic(topic string) (key, verb string, valid bool) {
	parts := strings.Split(topic, "/")
	if (len(parts) != 4 && len(parts) != 5) || parts[0] != "spBv1.0" {
		return "", "", false
	}
	for _, p := range parts[1:] {
		if p == "" || strings.ContainsAny(p, "+#\x00") || !utf8.ValidString(p) {
			return "", "", false
		}
	}
	device := parts[2] == "DBIRTH" || parts[2] == "DDATA" || parts[2] == "DDEATH" || parts[2] == "DCMD"
	if device != (len(parts) == 5) {
		return "", "", false
	}
	return parts[1] + "/" + parts[3], parts[2], true
}

func sparkObservation(topic string, payload []byte, maxDepth, maxFields int) (map[string]any, string, string, error) {
	key, verb, valid := sparkTopic(topic)
	view := map[string]any{"Profile": "sparkplug-b-node-scalars-3.0", "Topic": topic, "Context Status": "refused"}
	if !valid {
		view["Reason"] = "topic needs a supported node verb and four components"
		return view, key, verb, nil
	}
	view["Node Key"], view["Message Type"] = key, verb
	parts := strings.Split(topic, "/")
	if len(parts) == 5 {
		view["Device ID"] = parts[4]
		view["Profile"] = "sparkplug-b-device-scalars-3.0"
	}
	p, err := sparkDecode(payload, maxDepth, maxFields)
	if err != nil {
		var typed *ProtocolError
		if errors.As(err, &typed) && typed.Kind == ErrResourceExceeded {
			return nil, key, verb, err
		}
		view["Reason"] = err.Error()
		return view, key, verb, nil
	}
	view["Payload"] = p
	return view, key, verb, nil
}

func sparkBDSeq(p map[string]any) (uint64, bool) {
	metrics, _ := p["metrics"].([]map[string]any)
	found := false
	var bd uint64
	for _, m := range metrics {
		if m["name"] == "bdSeq" {
			u, ok := m["long_value"].(uint64)
			if found || !ok || m["datatype"] != uint64(4) || m["is_null"] == true || u > 255 {
				return 0, false
			}
			bd, found = u, true
		}
	}
	return bd, found
}

// Preview is transactional with MQTT pending IDs/topic aliases: the caller
// reserves the combined prospective state before committing either layer.
func (m *binMQTT) sparkPublish(dir int, topic string, payload []byte, flags byte, maxDepth, maxFields int) (map[string]any, *sparkNode, error) {
	v, key, verb, err := sparkObservation(topic, payload, maxDepth, maxFields)
	if err != nil {
		return nil, nil, err
	}
	next := m.spark
	refuse := func(reason, status string) (map[string]any, *sparkNode, error) {
		v["Reason"], v["Context Status"] = reason, status
		// Only the observed producer's current node can invalidate its own
		// definitions. Never erase a node's state for a forwarded/foreign topic.
		if dir == m.client && m.spark != nil && key == m.spark.key && (verb == "NBIRTH" || verb == "NDATA" || verb == "DBIRTH" || verb == "DDATA" || verb == "DDEATH") {
			next = &sparkNode{key: m.spark.key, bdSeq: m.spark.bdSeq}
		}
		return v, next, nil
	}
	p, ok := v["Payload"].(map[string]any)
	if !ok {
		return refuse(fmt.Sprint(v["Reason"]), "refused")
	}
	if verb != "NBIRTH" && verb != "NDATA" && verb != "NDEATH" && verb != "NCMD" && verb != "DBIRTH" && verb != "DDATA" && verb != "DDEATH" && verb != "DCMD" {
		return refuse("verb needs its own independent message/lifecycle profile", "unsupported")
	}
	if flags&1 != 0 || verb != "NDEATH" && (flags>>1)&3 != 0 || verb == "NDEATH" && (flags>>1)&3 != 1 {
		return refuse("invalid Sparkplug QoS/retain", "refused")
	}
	if m.spark == nil || key != m.spark.key || !m.connack || verb != "NCMD" && verb != "DCMD" && dir != m.client {
		return refuse("producer Will and successful CONNACK were not observed", "context-required")
	}
	if p["body"] != nil {
		return refuse("body needs an independent structured profile", "unsupported")
	}
	if verb == "DBIRTH" || verb == "DDATA" || verb == "DDEATH" {
		return m.sparkDevicePublish(v, verb, p, refuse, maxDepth, maxFields)
	}
	if verb == "DCMD" {
		if dir == m.client {
			return refuse("DCMD must target the observed producer", "refused")
		}
		return m.sparkDeviceCommand(v, p, refuse, maxDepth, maxFields)
	}
	if verb == "NCMD" {
		if _, present := p["seq"]; present {
			return refuse("NCMD excludes sequence number", "refused")
		}
		if dir == m.client {
			return refuse("NCMD must target the observed producer", "refused")
		}
		metrics, _ := p["metrics"].([]map[string]any)
		if len(metrics) != 1 || metrics[0]["name"] != "Node Control/Rebirth" {
			return m.sparkNodeCommand(v, p, refuse, maxDepth, maxFields)
		}
		if p["timestamp"] == nil {
			return refuse("selected NCMD requires timestamp and one Rebirth metric", "unsupported")
		}
		metric := metrics[0]
		_, aliased := metric["alias"]
		typ, typed := metric["datatype"].(uint64)
		value, reason := sparkValue(metric, 11, maxDepth, maxFields)
		if metric["name"] != "Node Control/Rebirth" || aliased || typed && typ != 11 || reason != "" || value != true {
			return refuse("NCMD Rebirth requires unaliased Boolean true", "refused")
		}
		v["Context Status"], v["bdSeq"] = "rebirth-requested", m.spark.bdSeq
		v["Metrics"] = []map[string]any{{"Name": "Node Control/Rebirth", "Datatype": uint64(11), "Value": true}}
		return v, &sparkNode{key: key, bdSeq: m.spark.bdSeq}, nil
	}
	if verb == "NDEATH" {
		bd, found := sparkBDSeq(p)
		if _, present := p["seq"]; present || !found || bd != m.spark.bdSeq {
			return refuse("NDEATH requires the registered bdSeq and no seq", "refused")
		}
		v["bdSeq"], v["Context Status"] = bd, "matched"
		return v, &sparkNode{key: key, bdSeq: bd}, nil
	}
	seq, hasSeq := p["seq"].(uint64)
	if !hasSeq || seq > 255 || p["timestamp"] == nil {
		return refuse("node message requires timestamp and seq in 0..255", "refused")
	}
	metrics, _ := p["metrics"].([]map[string]any)
	if verb == "NBIRTH" {
		bd, found := sparkBDSeq(p)
		if seq != 0 || !found || bd != m.spark.bdSeq {
			return refuse("NBIRTH requires seq zero and the registered bdSeq", "refused")
		}
		next = &sparkNode{key: key, bdSeq: bd, born: true, seq: seq, metrics: map[string]sparkMetricDef{}, aliases: map[uint64]string{}}
	} else {
		if !m.spark.born {
			return refuse("NBIRTH metric definitions were not observed", "context-required")
		}
		if seq != (m.spark.seq+1)%256 {
			return refuse("NDATA sequence gap requires a new NBIRTH", "refused")
		}
	}
	values := []map[string]any{}
	rebirth := false
	seen := map[string]bool{}
	var lastTimestamp uint64
	hasTimestamp := false
	for _, metric := range metrics {
		name, named := metric["name"].(string)
		typ, typed := metric["datatype"].(uint64)
		alias, aliased := metric["alias"].(uint64)
		if metric["timestamp"] == nil {
			return refuse("node metric requires timestamp", "refused")
		}
		if metric["is_historical"] != true {
			timestamp := metric["timestamp"].(uint64)
			if hasTimestamp && timestamp < lastTimestamp {
				return refuse("current node metrics require chronological order", "refused")
			}
			lastTimestamp, hasTimestamp = timestamp, true
		}
		if verb == "NBIRTH" {
			if !named || name == "" || !typed || seen[name] {
				return refuse("NBIRTH requires unique named typed metrics", "refused")
			}
			if aliased {
				if _, exists := next.aliases[alias]; exists {
					return refuse("NBIRTH metric aliases must be unique", "refused")
				}
				next.aliases[alias] = name
			}
			next.metrics[name] = sparkMetricDef{typ: typ}
		} else {
			if aliased {
				if named {
					return refuse("aliased NDATA excludes metric name", "refused")
				}
				name = m.spark.aliases[alias]
			}
			def, known := m.spark.metrics[name]
			if !known {
				return refuse("NDATA requires observed metric definitions", "refused")
			}
			if typed && typ != def.typ {
				return refuse("NDATA datatype differs from NBIRTH", "refused")
			}
			typ = def.typ
		}
		seen[name] = true
		value, reason := sparkValue(metric, typ, maxDepth, maxFields)
		if reason != "" {
			status := "refused"
			if strings.Contains(reason, "independent structured profile") {
				status = "unsupported"
			}
			return refuse(reason, status)
		}
		if verb == "NBIRTH" && name == "Node Control/Rebirth" {
			if typ != 11 || value != false || aliased || metric["is_null"] == true {
				return refuse("Node Control/Rebirth requires unaliased Boolean false", "refused")
			}
			rebirth = true
		}
		values = append(values, map[string]any{"Name": name, "Datatype": typ, "Value": value})
	}
	if verb == "NBIRTH" && !rebirth {
		return refuse("NBIRTH requires Node Control/Rebirth", "refused")
	}
	if verb == "NDATA" {
		copy := *m.spark
		copy.seq = seq
		next = &copy
	}
	v["Context Status"], v["bdSeq"], v["Metrics"] = "matched", next.bdSeq, values
	return v, next, nil
}

// Device scalar definitions belong to the observed producer's node, while
// names are scoped to one device and aliases are unique across the entire node.
type sparkDevice struct {
	born    bool
	metrics map[string]sparkMetricDef
	aliases map[uint64]string
}

func (s *sparkNode) withDevices() *sparkNode {
	n := *s
	n.devices = make(map[string]*sparkDevice, len(s.devices))
	for id, d := range s.devices {
		n.devices[id] = d
	}
	return &n
}

func (m *binMQTT) sparkDevicePublish(v map[string]any, verb string, p map[string]any, refuse func(string, string) (map[string]any, *sparkNode, error), maxDepth, maxFields int) (map[string]any, *sparkNode, error) {
	id := v["Device ID"].(string)
	if !m.spark.born {
		return refuse("NBIRTH metric definitions were not observed", "context-required")
	}
	seq, hasSeq := p["seq"].(uint64)
	if !hasSeq || seq > 255 || p["timestamp"] == nil {
		return refuse("device message requires timestamp and seq in 0..255", "refused")
	}
	if seq != (m.spark.seq+1)%256 {
		return refuse("device sequence gap requires a new NBIRTH", "refused")
	}
	old := m.spark.devices[id]
	if verb != "DBIRTH" && (old == nil || !old.born) {
		return refuse("DBIRTH metric definitions were not observed", "context-required")
	}
	ms, _ := p["metrics"].([]map[string]any)
	copy := *m.spark
	next := &copy
	if verb != "DDATA" {
		next = m.spark.withDevices()
	}
	next.seq = seq
	if verb == "DDEATH" {
		if len(ms) > 0 {
			return refuse("DDEATH metrics need an independent message profile", "unsupported")
		}
		// Death marks definitions stale, not unknown. Keep their aliases
		// reserved until this device rebirths or the entire node is reborn.
		stale := *old
		stale.born = false
		next.devices[id] = &stale
		v["Context Status"], v["bdSeq"] = "matched", next.bdSeq
		return v, next, nil
	}
	definition := old
	if verb == "DBIRTH" {
		definition = &sparkDevice{born: true, metrics: map[string]sparkMetricDef{}, aliases: map[uint64]string{}}
	}
	values := []map[string]any{}
	seen := map[string]bool{}
	var lastTimestamp uint64
	hasTimestamp := false
	for _, metric := range ms {
		name, named := metric["name"].(string)
		typ, typed := metric["datatype"].(uint64)
		alias, aliased := metric["alias"].(uint64)
		if metric["timestamp"] == nil {
			return refuse("device metric requires timestamp", "refused")
		}
		if metric["is_historical"] != true {
			timestamp := metric["timestamp"].(uint64)
			if hasTimestamp && timestamp < lastTimestamp {
				return refuse("current device metrics require chronological order", "refused")
			}
			lastTimestamp, hasTimestamp = timestamp, true
		}
		if verb == "DBIRTH" {
			if !named || name == "" || !typed || seen[name] {
				return refuse("DBIRTH requires unique named typed metrics", "refused")
			}
			if aliased {
				if _, exists := definition.aliases[alias]; exists {
					return refuse("DBIRTH metric aliases must be unique", "refused")
				}
				if _, exists := m.spark.aliases[alias]; exists {
					return refuse("device alias collides with another node or device metric", "refused")
				}
				for other, d := range m.spark.devices {
					if other != id {
						if _, exists := d.aliases[alias]; exists {
							return refuse("device alias collides with another node or device metric", "refused")
						}
					}
				}
				definition.aliases[alias] = name
			}
			definition.metrics[name] = sparkMetricDef{typ: typ}
		} else {
			if aliased {
				if named {
					return refuse("aliased DDATA excludes metric name", "refused")
				}
				name = definition.aliases[alias]
			}
			def, known := definition.metrics[name]
			if !known {
				return refuse("DDATA requires this device observed metric definitions", "refused")
			}
			if typed && typ != def.typ {
				return refuse("DDATA datatype differs from DBIRTH", "refused")
			}
			typ = def.typ
		}
		seen[name] = true
		value, reason := sparkValue(metric, typ, maxDepth, maxFields)
		if reason != "" {
			status := "refused"
			if strings.Contains(reason, "independent structured profile") {
				status = "unsupported"
			}
			return refuse(reason, status)
		}
		values = append(values, map[string]any{"Name": name, "Datatype": typ, "Value": value})
	}
	if verb == "DBIRTH" {
		next.devices[id] = definition
	}
	v["Context Status"], v["bdSeq"], v["Metrics"] = "matched", next.bdSeq, values
	return v, next, nil
}

// A command observation proves only compatible fields against a live observed
// DBIRTH. It never changes producer sequence, definitions, or measured values,
// and never claims that an output write succeeded at a device.
func (m *binMQTT) sparkDeviceCommand(v map[string]any, p map[string]any, refuse func(string, string) (map[string]any, *sparkNode, error), maxDepth, maxFields int) (map[string]any, *sparkNode, error) {
	if _, present := p["seq"]; present {
		return refuse("DCMD excludes sequence number", "refused")
	}
	if !m.spark.born {
		return refuse("NBIRTH metric definitions were not observed", "context-required")
	}
	d := m.spark.devices[v["Device ID"].(string)]
	if d == nil || !d.born {
		return refuse("DBIRTH metric definitions were not observed", "context-required")
	}
	ms, _ := p["metrics"].([]map[string]any)
	if p["timestamp"] == nil || len(ms) == 0 {
		return refuse("DCMD requires timestamp and at least one metric", "refused")
	}
	values := []map[string]any{}
	for _, metric := range ms {
		name, named := metric["name"].(string)
		alias, aliased := metric["alias"].(uint64)
		if aliased {
			if named {
				return refuse("aliased DCMD excludes metric name", "refused")
			}
			name = d.aliases[alias]
		}
		def, known := d.metrics[name]
		if !known {
			return refuse("DCMD requires this device observed metric definitions", "context-required")
		}
		if typ, typed := metric["datatype"].(uint64); typed && typ != def.typ {
			return refuse("DCMD datatype differs from DBIRTH", "refused")
		}
		if metric["is_null"] == true {
			return refuse("null command values need an independent write profile", "unsupported")
		}
		value, reason := sparkValue(metric, def.typ, maxDepth, maxFields)
		if reason != "" {
			status := "refused"
			if strings.Contains(reason, "independent structured profile") {
				status = "unsupported"
			}
			return refuse(reason, status)
		}
		values = append(values, map[string]any{"Name": name, "Datatype": def.typ, "Value": value})
	}
	v["Context Status"], v["Operation"], v["bdSeq"], v["Metrics"] = "matched", "command-observed", m.spark.bdSeq, values
	return v, m.spark, nil
}

func (m *binMQTT) sparkNodeCommand(v map[string]any, p map[string]any, refuse func(string, string) (map[string]any, *sparkNode, error), maxDepth, maxFields int) (map[string]any, *sparkNode, error) {
	if !m.spark.born {
		return refuse("NBIRTH metric definitions were not observed", "context-required")
	}
	ms, _ := p["metrics"].([]map[string]any)
	if p["timestamp"] == nil || len(ms) == 0 {
		return refuse("NCMD requires timestamp and at least one metric", "refused")
	}
	values := []map[string]any{}
	for _, metric := range ms {
		name, named := metric["name"].(string)
		alias, aliased := metric["alias"].(uint64)
		if aliased {
			if named {
				return refuse("aliased NCMD excludes metric name", "refused")
			}
			name = m.spark.aliases[alias]
		}
		if name == "Node Control/Rebirth" {
			return refuse("mixed Rebirth commands need an independent command profile", "unsupported")
		}
		def, known := m.spark.metrics[name]
		if !known {
			return refuse("NCMD requires this node observed metric definitions", "context-required")
		}
		if typ, typed := metric["datatype"].(uint64); typed && typ != def.typ {
			return refuse("NCMD datatype differs from NBIRTH", "refused")
		}
		if metric["is_null"] == true {
			return refuse("null command values need an independent write profile", "unsupported")
		}
		value, reason := sparkValue(metric, def.typ, maxDepth, maxFields)
		if reason != "" {
			status := "refused"
			if strings.Contains(reason, "independent structured profile") {
				status = "unsupported"
			}
			return refuse(reason, status)
		}
		values = append(values, map[string]any{"Name": name, "Datatype": def.typ, "Value": value})
	}
	v["Context Status"], v["Operation"], v["bdSeq"], v["Metrics"] = "matched", "command-observed", m.spark.bdSeq, values
	return v, m.spark, nil
}
