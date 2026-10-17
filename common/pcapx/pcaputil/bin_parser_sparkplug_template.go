package pcaputil

import (
	"encoding/hex"
	"fmt"
	"google.golang.org/protobuf/encoding/protowire"
	"math"
	"unicode/utf8"
)

// A definition retains only literal names/types and optional version. Mutable
// public values/defaults are never used as an instance's observation.
type sparkTemplateDef struct {
	version    string
	hasVersion bool
	members    map[string]uint64
	parameters map[string]uint64
}

func (d *sparkTemplateDef) retainedBytes() int64 {
	if d == nil {
		return 0
	}
	n := int64(128 + len(d.version))
	for k := range d.members {
		n += int64(64 + len(k))
	}
	for k := range d.parameters {
		n += int64(64 + len(k))
	}
	return n
}
func sparkTemplateDecode(w []byte, maxDepth, maxFields int) (map[string]any, error) {
	p := map[string]any{}
	ms := []map[string]any{}
	ps := []map[string]any{}
	unknown := []byte{}
	count := 0
	err := sparkWalk(w, 3, maxDepth, maxFields, &count, func(n protowire.Number, t protowire.Type, raw []byte) error {
		_, _, k := protowire.ConsumeTag(raw)
		v := raw[k:]
		switch {
		case (n == 1 || n == 4) && t == protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			if !utf8.Valid(b) {
				return fmt.Errorf("Template invalid UTF-8")
			}
			name := "version"
			if n == 4 {
				name = "template_ref"
			}
			p[name] = string(b)
		case n == 5 && t == protowire.VarintType:
			u, _ := protowire.ConsumeVarint(v)
			p["is_definition"] = u != 0
		case n == 2 && t == protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			m, e := sparkDecodeMetric(b, maxDepth, maxFields)
			if e != nil {
				return e
			}
			ms = append(ms, m)
		case n == 3 && t == protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			m, e := sparkTemplateParameter(b, maxDepth, maxFields)
			if e != nil {
				return e
			}
			ps = append(ps, m)
		default:
			unknown = append(unknown, raw...)
		}
		return nil
	})
	p["metrics"], p["parameters"] = ms, ps
	if len(unknown) > 0 {
		p["unknown_hex"] = hex.EncodeToString(unknown)
	}
	return p, err
}
func sparkTemplateParameter(w []byte, maxDepth, maxFields int) (map[string]any, error) {
	p := map[string]any{}
	count := 0
	unknown := []byte{}
	names := map[protowire.Number]string{1: "name", 2: "type", 3: "int_value", 4: "long_value", 5: "float_value", 6: "double_value", 7: "boolean_value", 8: "string_value", 9: "extension_value"}
	err := sparkWalk(w, 4, maxDepth, maxFields, &count, func(n protowire.Number, t protowire.Type, raw []byte) error {
		_, _, k := protowire.ConsumeTag(raw)
		v := raw[k:]
		name, ok := names[n]
		want := protowire.VarintType
		if n == 1 || n == 8 || n == 9 {
			want = protowire.BytesType
		} else if n == 5 {
			want = protowire.Fixed32Type
		} else if n == 6 {
			want = protowire.Fixed64Type
		}
		if !ok || t != want {
			unknown = append(unknown, raw...)
			return nil
		}
		if n >= 3 {
			for i := protowire.Number(3); i <= 9; i++ {
				delete(p, names[i])
			}
		}
		switch t {
		case protowire.VarintType:
			u, _ := protowire.ConsumeVarint(v)
			if n == 7 {
				p[name] = u != 0
			} else if n == 2 || n == 3 {
				p[name] = uint64(uint32(u))
			} else {
				p[name] = u
			}
		case protowire.Fixed32Type:
			u, _ := protowire.ConsumeFixed32(v)
			p[name] = math.Float32frombits(u)
		case protowire.Fixed64Type:
			u, _ := protowire.ConsumeFixed64(v)
			p[name] = math.Float64frombits(u)
		case protowire.BytesType:
			b, _ := protowire.ConsumeBytes(v)
			if n == 9 {
				p[name] = append([]byte{}, b...)
			} else {
				if !utf8.Valid(b) {
					return fmt.Errorf("Template parameter invalid UTF-8")
				}
				p[name] = string(b)
			}
		}
		return nil
	})
	if len(unknown) > 0 {
		p["unknown_hex"] = hex.EncodeToString(unknown)
	}
	return p, err
}

func sparkTemplateProject(p map[string]any, d *sparkTemplateDef, birth bool, maxDepth, maxFields int) (map[string]any, *sparkTemplateDef, string, string) {
	fail := func(why, status string) (map[string]any, *sparkTemplateDef, string, string) {
		return nil, nil, why, status
	}
	definition, present := p["is_definition"].(bool)
	if !present {
		return fail("Template requires is_definition", "refused")
	}
	if p["unknown_hex"] != nil {
		return fail("Template extensions need an independent structured profile", "unsupported")
	}
	ref, hasRef := p["template_ref"].(string)
	if definition && hasRef {
		return fail("Template definition excludes template_ref", "refused")
	}
	if !definition && (!hasRef || ref == "") {
		return fail("Template instance requires template_ref", "refused")
	}
	if !definition && d == nil {
		return fail("Template reference requires an observed NBIRTH definition", "context-required")
	}
	version, hasVersion := p["version"].(string)
	if !definition && hasVersion && d.hasVersion && version != d.version {
		return fail("Template version differs from observed definition", "refused")
	}
	ms := p["metrics"].([]map[string]any)
	ps := p["parameters"].([]map[string]any)
	schema := &sparkTemplateDef{version: version, hasVersion: hasVersion, members: map[string]uint64{}, parameters: map[string]uint64{}}
	metrics := []map[string]any{}
	params := []map[string]any{}
	for _, m := range ms {
		name, named := m["name"].(string)
		typ, typed := m["datatype"].(uint64)
		if !named || name == "" || schema.members[name] != 0 {
			return fail("Template member names must be unique and explicit", "refused")
		}
		if _, aliased := m["alias"]; aliased || m["is_historical"] == true || m["is_transient"] == true || m["unknown_hex"] != nil {
			return fail("Template nested metric context needs an independent structured profile", "unsupported")
		}
		// Keep the pre-existing unselected structured-type boundary before
		// instance comparison; partial Basic observations do not upgrade nesting.
		if typed && (typ < 1 || typ > 14) {
			return fail("Template members require Basic datatypes", "unsupported")
		}
		if !definition {
			want, known := d.members[name]
			if !known {
				return fail("Template member datatype differs from definition", "refused")
			}
			// DATA SHOULD omit datatype. Resolve it only by the explicit name
			// in this instance's actually observed definition, never position.
			if !typed && !birth {
				typ, typed = want, true
			}
			if typed && typ != want {
				return fail("Template member datatype differs from definition", "refused")
			}
		}
		if !typed || typ < 1 || typ > 14 {
			return fail("Template members require Basic datatypes", "unsupported")
		}
		value, why := sparkValue(m, typ, maxDepth, maxFields)
		if why != "" {
			status := "refused"
			if why == "datatype/value mismatch" && definition {
				why = "Template member declarations without defaults need an independent structured profile"
				status = "unsupported"
			}
			return fail(why, status)
		}
		schema.members[name] = typ
		metrics = append(metrics, map[string]any{"name": name, "datatype": typ, "value": value})
	}
	if !definition && birth && len(schema.members) != len(d.members) {
		return fail("Template instance members differ from definition", "refused")
	}
	if !definition && !birth && len(metrics) == 0 {
		return fail("Template empty Data needs an independent structured profile", "unsupported")
	}
	for _, p := range ps {
		name, named := p["name"].(string)
		typ, typed := p["type"].(uint64)
		if !named || name == "" || schema.parameters[name] != 0 {
			return fail("Template parameter names must be unique and explicit", "refused")
		}
		if !typed || typ < 1 || typ > 14 {
			return fail("Template parameters require Basic datatypes", "unsupported")
		}
		if p["unknown_hex"] != nil {
			return fail("Template parameter extensions need an independent structured profile", "unsupported")
		}
		if !definition {
			want, ok := d.parameters[name]
			if !ok || typ != want {
				return fail("Template parameter datatype differs from definition", "refused")
			}
		}
		value, why := sparkTemplateParameterValue(p, typ, maxDepth, maxFields)
		if why != "" {
			if why == "Template parameter defaults need an independent structured profile" {
				return fail(why, "unsupported")
			}
			return fail(why, "refused")
		}
		schema.parameters[name] = typ
		params = append(params, map[string]any{"name": name, "type": typ, "value": value})
	}
	if !definition && len(schema.parameters) != len(d.parameters) {
		return fail("Template explicit parameters differ from definition", "unsupported")
	}
	out := map[string]any{"is_definition": definition, "metrics": metrics, "parameters": params}
	if hasVersion {
		out["version"] = version
	}
	if hasRef {
		out["template_ref"] = ref
	}
	return out, schema, "", ""
}
func sparkTemplateMetric(metric map[string]any, maxDepth, maxFields int) (map[string]any, string, string) {
	if metric["metadata"] != nil || metric["properties"] != nil {
		return nil, "metadata/properties need an independent structured profile", "unsupported"
	}
	if metric["is_null"] == true {
		return nil, "Template whole-null needs an independent structured profile", "unsupported"
	}
	raw, ok := metric["template_value"].([]byte)
	if !ok {
		return nil, "datatype/value mismatch", "refused"
	}
	p, err := sparkTemplateDecode(raw, maxDepth, maxFields)
	if err != nil {
		return nil, err.Error(), "refused"
	}
	return p, "", ""
}

// Definitions within one valid Birth are order independent. The new registry
// is private and uncommitted until the entire producer observation succeeds.
func sparkTemplateBirthDefinitions(ms []map[string]any, maxDepth, maxFields int) (map[string]*sparkTemplateDef, string, string) {
	var defs map[string]*sparkTemplateDef
	for _, m := range ms {
		if m["datatype"] != uint64(19) {
			continue
		}
		p, why, status := sparkTemplateMetric(m, maxDepth, maxFields)
		if why != "" {
			return nil, why, status
		}
		if p["is_definition"] != true {
			continue
		}
		name, ok := m["name"].(string)
		if !ok || name == "" || defs[name] != nil {
			return nil, "NBIRTH requires unique named typed metrics", "refused"
		}
		_, d, why, status := sparkTemplateProject(p, nil, true, maxDepth, maxFields)
		if why != "" {
			return nil, why, status
		}
		if defs == nil {
			defs = map[string]*sparkTemplateDef{}
		}
		defs[name] = d
	}
	return defs, "", ""
}
func sparkTemplateNodeValue(metric map[string]any, name, verb string, node *sparkNode, maxDepth, maxFields int) (any, string, string) {
	p, why, status := sparkTemplateMetric(metric, maxDepth, maxFields)
	if why != "" {
		return nil, why, status
	}
	definition := p["is_definition"] == true
	if verb != "NBIRTH" && definition {
		return nil, "Template definitions are only valid in NBIRTH", "refused"
	}
	ref, _ := p["template_ref"].(string)
	if verb != "NBIRTH" {
		old := node.metrics[name]
		if old.templateDefinition {
			return nil, "Template definition metric cannot carry NDATA", "refused"
		}
		if old.templateRef != ref {
			return nil, "Template reference differs from instance NBIRTH", "refused"
		}
	}
	value, _, why, status := sparkTemplateProject(p, node.templates[ref], verb == "NBIRTH", maxDepth, maxFields)
	if why != "" {
		return nil, why, status
	}
	if verb == "NBIRTH" {
		d := node.metrics[name]
		d.templateDefinition = definition
		d.templateRef = ref
		node.metrics[name] = d
	}
	return value, "", ""
}

// Parameter UInt32 is canonically uint32/int_value. Tahu's fixed Java
// encoder also emits uint64/long_value and its decoder accepts either. Observe
// that explicit compatibility form without inheriting its unchecked overflow.
func sparkTemplateParameterValue(p map[string]any, typ uint64, maxDepth, maxFields int) (any, string) {
	if typ == 7 {
		if v, present := p["long_value"].(uint64); present {
			if v > math.MaxUint32 {
				return nil, "unsigned metric exceeds datatype width"
			}
			return v, ""
		}
	}
	value, why := sparkValue(p, typ, maxDepth, maxFields)
	if why != "datatype/value mismatch" {
		return value, why
	}
	for _, key := range []string{"int_value", "long_value", "float_value", "double_value", "boolean_value", "string_value", "extension_value"} {
		if _, present := p[key]; present {
			return nil, why
		}
	}
	return nil, "Template parameter defaults need an independent structured profile"
}
