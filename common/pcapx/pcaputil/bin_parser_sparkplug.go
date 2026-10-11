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
}

func (s *sparkNode) retainedBytes() int64 {
	if s == nil {
		return 0
	}
	n := int64(128 + len(s.key))
	for name := range s.metrics {
		n += int64(96 + len(name))
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
	count := 0
	// Validate nested Metric envelopes in the allocation-free first pass.
	err := sparkWalk(w, 1, maxDepth, maxFields, &count, func(n protowire.Number, typ protowire.Type, v []byte) error {
		_, _, tag := protowire.ConsumeTag(v)
		v = v[tag:]
		if n == 2 && typ == protowire.BytesType {
			b, _ := protowire.ConsumeBytes(v)
			return sparkWalk(b, 2, maxDepth, maxFields, &count, nil)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	metrics := []map[string]any{}
	unknown := []byte{}
	count = 0
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
		// Proto2 optional fields and the value oneof use the last correctly
		// typed occurrence. A known field with the wrong wire type is unknown.
		if n >= 10 {
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

func sparkValue(m map[string]any, typ uint64) (any, string) {
	if m["metadata"] != nil || m["properties"] != nil {
		return nil, "metadata/properties need an independent structured profile"
	}
	switch typ {
	case 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17:
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
	if len(parts) != 4 || parts[0] != "spBv1.0" {
		return "", "", false
	}
	for _, p := range parts[1:] {
		if p == "" || strings.ContainsAny(p, "+#\x00") || !utf8.ValidString(p) {
			return "", "", false
		}
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
		if dir == m.client && m.spark != nil && key == m.spark.key && (verb == "NBIRTH" || verb == "NDATA") {
			next = &sparkNode{key: m.spark.key, bdSeq: m.spark.bdSeq}
		}
		return v, next, nil
	}
	p, ok := v["Payload"].(map[string]any)
	if !ok {
		return refuse(fmt.Sprint(v["Reason"]), "refused")
	}
	if verb != "NBIRTH" && verb != "NDATA" && verb != "NDEATH" && verb != "NCMD" {
		return refuse("verb needs its own independent message/lifecycle profile", "unsupported")
	}
	if flags&1 != 0 || verb != "NDEATH" && (flags>>1)&3 != 0 || verb == "NDEATH" && (flags>>1)&3 != 1 {
		return refuse("invalid Sparkplug QoS/retain", "refused")
	}
	if m.spark == nil || key != m.spark.key || !m.connack || verb != "NCMD" && dir != m.client {
		return refuse("producer Will and successful CONNACK were not observed", "context-required")
	}
	if p["body"] != nil {
		return refuse("body needs an independent structured profile", "unsupported")
	}
	if verb == "NCMD" {
		if dir == m.client {
			return refuse("NCMD must target the observed producer", "refused")
		}
		metrics, _ := p["metrics"].([]map[string]any)
		if len(metrics) != 1 || p["timestamp"] == nil {
			return refuse("selected NCMD requires timestamp and one Rebirth metric", "unsupported")
		}
		metric := metrics[0]
		_, aliased := metric["alias"]
		typ, typed := metric["datatype"].(uint64)
		value, reason := sparkValue(metric, 11)
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
		value, reason := sparkValue(metric, typ)
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
