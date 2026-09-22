package model

import "strconv"

// The frequent ASCII identity path writes the same JSON field order and
// omission rules as encoding/json into caller-owned scratch. HTML characters
// use its exact escapes; quotes, controls and non-ASCII use the standard encoder.
func plainJSON(values ...string) bool {
	for _, s := range values {
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c < 32 || c > 126 || c == '"' || c == '\\' {
				return false
			}
		}
	}
	return true
}

func jsonField(dst []byte, key, value string) []byte {
	if len(dst) > 1 {
		dst = append(dst, ',')
	}
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	return jsonQuoted(dst, value)
}

func jsonQuoted(dst []byte, value string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '<' || c == '>' || c == '&' {
			dst = append(dst, value[start:i]...)
			dst = append(dst, '\\', 'u', '0', '0', "0123456789abcdef"[c>>4], "0123456789abcdef"[c&15])
			start = i + 1
		}
	}
	dst = append(dst, value[start:]...)
	return append(dst, '"')
}

func componentJSON(dst []byte, k ComponentKey) ([]byte, bool) {
	if !plainJSON(k.Ecosystem, k.Name, k.Version, k.Source, k.Architecture, k.Variant, k.Verification) {
		return nil, false
	}
	dst = append(dst, '{')
	dst = jsonField(dst, "ecosystem", k.Ecosystem)
	dst = jsonField(dst, "name", k.Name)
	for _, field := range [...][2]string{{"version", k.Version}, {"source", k.Source}, {"architecture", k.Architecture}, {"variant", k.Variant}, {"verification", k.Verification}} {
		if field[1] != "" {
			dst = jsonField(dst, field[0], field[1])
		}
	}
	return append(dst, '}'), true
}

func observationJSON(dst []byte, o Observation) ([]byte, bool) {
	if len(o.Provides) != 0 || !plainJSON(o.Condition, o.Scope, o.Component, o.Snapshot, o.Project, o.File, o.NativeID, o.Kind, o.DeclaredIntegrity) {
		return nil, false
	}
	dst = append(dst, '{')
	if o.Condition != "" {
		dst = jsonField(dst, "condition", o.Condition)
	}
	if o.Scope != "" {
		dst = jsonField(dst, "scope", o.Scope)
	}
	for _, f := range [...][2]string{{"component", o.Component}, {"snapshot", o.Snapshot}, {"project", o.Project}, {"file", o.File}} {
		dst = jsonField(dst, f[0], f[1])
	}
	for _, f := range [...]struct {
		key   string
		value int
	}{{"startLine", o.StartLine}, {"endLine", o.EndLine}} {
		if f.value != 0 {
			dst = append(dst, ',', '"')
			dst = append(dst, f.key...)
			dst = append(dst, '"', ':')
			dst = strconv.AppendInt(dst, int64(f.value), 10)
		}
	}
	if o.NativeID != "" {
		dst = jsonField(dst, "nativeId", o.NativeID)
	}
	dst = jsonField(dst, "kind", o.Kind)
	if o.DeclaredIntegrity != "" {
		dst = jsonField(dst, "declaredIntegrity", o.DeclaredIntegrity)
	}
	return append(dst, '}'), true
}

func jsonStrings(dst []byte, key string, values []string) []byte {
	if len(values) == 0 {
		return dst
	}
	if len(dst) > 1 {
		dst = append(dst, ',')
	}
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':', '[')
	for i, s := range values {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = jsonQuoted(dst, s)
	}
	return append(dst, ']')
}

func requirementJSON(dst []byte, q Requirement) ([]byte, bool) {
	if !plainJSON(q.From, q.Target, q.Constraint, q.Scope, q.Condition, q.Group, q.Operator) || !plainJSON(q.Candidates...) || !plainJSON(q.Resolved...) {
		return nil, false
	}
	dst = append(dst, '{')
	dst = jsonStrings(dst, "candidates", q.Candidates)
	dst = jsonField(dst, "from", q.From)
	dst = jsonField(dst, "target", q.Target)
	if q.Constraint != "" {
		dst = jsonField(dst, "constraint", q.Constraint)
	}
	dst = jsonStrings(dst, "resolved", q.Resolved)
	for _, f := range [...][2]string{{"scope", q.Scope}, {"condition", q.Condition}, {"group", q.Group}, {"operator", q.Operator}} {
		if f[1] != "" {
			dst = jsonField(dst, f[0], f[1])
		}
	}
	return append(dst, '}'), true
}
