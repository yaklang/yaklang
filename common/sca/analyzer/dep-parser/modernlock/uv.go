// Package modernlock reads frozen uv and Bun lock snapshots without acquisition
// or host-dependent environment resolution.
package modernlock

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/locktoml"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"github.com/yaklang/yaklang/common/sca/internal/digest"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type UV struct{}

func bad(s string) error                             { return scanerr.New(scanerr.MalformedInput, "%s", s) }
func encoded(v any) string                           { b, _ := json.Marshal(v); return string(b) }
func str(m map[string]any, k string) (string, error) { return locktoml.Field[string](m, k) }
func stringsOf(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil, bad("expected string array")
	}
	out := make([]string, 0, len(a))
	for _, x := range a {
		s, ok := x.(string)
		if !ok {
			return nil, bad("expected string array member")
		}
		out = append(out, s)
	}
	return out, nil
}
func uvSource(v any) (string, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return "", bad("uv source must be a tagged table")
	}
	kind := ""
	for k, v := range m {
		s, ok := v.(string)
		if !ok || s == "" {
			return "", bad("invalid uv source value")
		}
		switch k {
		case "registry", "git", "url", "path", "directory", "editable", "virtual":
			if kind != "" {
				return "", bad("ambiguous uv source")
			}
			kind = k
		case "subdirectory":
		default:
			return "", scanerr.New(scanerr.UnsupportedSyntax, "uv source field %q", k)
		}
	}
	if kind == "" || len(m) > 1 && (kind != "url" || len(m) != 2) {
		return "", bad("invalid uv source fields")
	}
	return encoded(m), nil
}
func uvName(s string) string {
	if s == "" {
		return ""
	}
	alnum := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' }
	if !alnum(s[0]) || !alnum(s[len(s)-1]) {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if !alnum(s[i]) && s[i] != '-' && s[i] != '_' && s[i] != '.' {
			return ""
		}
	}
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == '-' || r == '_' || r == '.' }), "-")
}
func uvArtifacts(p map[string]any) (string, error) {
	var hashes []string
	check := func(v any) error {
		m, ok := v.(map[string]any)
		if !ok {
			return bad("uv artifact must be a table")
		}
		h, e := str(m, "hash")
		if e != nil {
			return e
		}
		if h != "" {
			hashes = append(hashes, h)
		}
		for _, k := range []string{"url", "path"} {
			if _, e := str(m, k); e != nil {
				return e
			}
		}
		return nil
	}
	if v, ok := p["sdist"]; ok {
		if e := check(v); e != nil {
			return "", e
		}
	}
	if v, ok := p["wheels"]; ok {
		a, ok := v.([]any)
		if !ok {
			return "", bad("uv wheels must be an array")
		}
		for _, v := range a {
			if e := check(v); e != nil {
				return "", e
			}
		}
	}
	return strings.Join(hashes, " "), nil
}

type uvRecord struct {
	raw map[string]any
	lib types.Library
}

func (UV) Parse(_ fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	root, spans, e := locktoml.ReadRecords(ctx, r, "package")
	if e != nil {
		return nil, nil, e
	}
	if e = textdecode.ReserveRecordConversion(ctx, root); e != nil {
		return nil, nil, e
	}
	version, e := locktoml.Field[int64](root, "version")
	if e != nil {
		return nil, nil, e
	}
	revision, e := locktoml.Field[int64](root, "revision")
	if e != nil {
		return nil, nil, e
	}
	if version != 1 || revision < 0 || revision > 3 {
		return nil, nil, scanerr.New(scanerr.UnsupportedSyntax, "uv version %d revision %d", version, revision)
	}
	py, e := str(root, "requires-python")
	if e != nil {
		return nil, nil, e
	}
	markers, e := stringsOf(root["resolution-markers"])
	if e != nil {
		return nil, nil, e
	}
	for _, key := range []string{"supported-markers", "required-markers"} {
		if _, e := stringsOf(root[key]); e != nil {
			return nil, nil, e
		}
	}
	if v, exists := root["conflicts"]; exists {
		groups, ok := v.([]any)
		if !ok {
			return nil, nil, bad("uv conflicts must be an array")
		}
		for _, group := range groups {
			items, ok := group.([]any)
			if !ok {
				return nil, nil, bad("uv conflict group must be an array")
			}
			for _, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, nil, bad("uv conflict item must be a table")
				}
				for _, key := range []string{"package", "extra", "group"} {
					if _, e := str(m, key); e != nil {
						return nil, nil, e
					}
				}
			}
		}
	}
	global := encoded(map[string]any{"requires-python": py, "resolution-markers": markers, "supported-markers": root["supported-markers"], "required-markers": root["required-markers"], "conflicts": root["conflicts"], "manifest": root["manifest"]})
	a, ok := root["package"].([]any)
	if !ok {
		return nil, nil, bad("uv package array missing")
	}
	if e = budget.From(ctx).Working(int64(len(a)) * (2048 + 24*int64(len(global)))); e != nil {
		return nil, nil, e
	}
	records := make([]uvRecord, 0, len(a))
	index := map[string][]int{}
	ids := map[string]bool{}
	for i, v := range a {
		if e = ctx.Err(); e != nil {
			return nil, nil, e
		}
		p, ok := v.(map[string]any)
		if !ok {
			return nil, nil, bad("uv package must be a table")
		}
		name, e := str(p, "name")
		if e != nil {
			return nil, nil, e
		}
		name = uvName(name)
		if name == "" {
			return nil, nil, bad("empty uv name")
		}
		version, e := str(p, "version")
		if e != nil {
			return nil, nil, e
		}
		source, e := uvSource(p["source"])
		if e != nil {
			return nil, nil, e
		}
		id := encoded([]string{name, version, source})
		if ids[id] {
			return nil, nil, bad("duplicate uv package identity")
		}
		ids[id] = true
		pm, e := stringsOf(p["resolution-markers"])
		if e != nil {
			return nil, nil, e
		}
		hashes, e := uvArtifacts(p)
		if e != nil {
			return nil, nil, e
		}
		d := digest.ParseDeclared(hashes)
		if len(d.Issues) > 0 {
			return nil, nil, bad("invalid uv artifact digest")
		}
		// Keep artifact-to-URL association and package declaration metadata. The
		// hashes are lock declarations, never evidence of locally verified bytes.
		variant := encoded(map[string]any{"source": p["source"], "resolution-markers": pm, "sdist": p["sdist"], "wheels": p["wheels"], "metadata": p["metadata"]})
		lib := types.Library{ID: id, Name: name, Version: version, Source: source, Variant: variant, Condition: encoded(map[string]any{"lock": global, "resolution-markers": pm}), Scope: "directness:unknown", Evidence: "locked", DeclaredIntegrity: hashes, Verification: d.Canonical}
		if i < len(spans) {
			lib.Locations = types.Locations{{StartLine: spans[i].StartLine, EndLine: spans[i].EndLine}}
		}
		index[name] = append(index[name], len(records))
		records = append(records, uvRecord{p, lib})
	}
	var libs []types.Library
	var deps []types.Dependency
	steps := 0
	for _, p := range records {
		libs = append(libs, p.lib)
		d := types.Dependency{ID: p.lib.ID}
		add := func(raw any, scope string) error {
			if raw == nil {
				return nil
			}
			a, ok := raw.([]any)
			if !ok {
				return bad("uv dependencies must be arrays")
			}
			if e := budget.From(ctx).Working(int64(len(a)) * (1024 + 24*int64(len(p.lib.Condition)))); e != nil {
				return e
			}
			for _, v := range a {
				if e := ctx.Err(); e != nil {
					return e
				}
				m, ok := v.(map[string]any)
				if !ok {
					return bad("uv dependency must be a table")
				}
				name, e := str(m, "name")
				if e != nil {
					return e
				}
				name = uvName(name)
				if name == "" {
					return bad("empty uv dependency")
				}
				version, e := str(m, "version")
				if e != nil {
					return e
				}
				source := ""
				if v, ok := m["source"]; ok {
					source, e = uvSource(v)
					if e != nil {
						return e
					}
				}
				marker, e := str(m, "marker")
				if e != nil {
					return e
				}
				extras, e := stringsOf(m["extra"])
				if e != nil {
					return e
				}
				q := types.Requirement{Target: name, Constraint: version, Scope: scope, Condition: encoded(map[string]any{"package": p.lib.Condition, "marker": marker, "extra": extras, "source": source})}
				match := ""
				n := 0
				for _, i := range index[name] {
					steps++
					if steps > budget.From(ctx).Limits.MaxResolveSteps {
						return scanerr.New(scanerr.ResourceLimit, "uv resolution steps")
					}
					if e := ctx.Err(); e != nil {
						return e
					}
					candidate := records[i].lib
					if (version == "" || candidate.Version == version) && (source == "" || candidate.Source == source) {
						match = candidate.ID
						n++
					}
				}
				if n == 1 {
					q.Resolved = match
				}
				d.Requirements = append(d.Requirements, q)
				// Conditional edges stay in Requirements; exporting an unconditional edge
				// here would erase markers/extras and imply an installation selection.
			}
			return nil
		}
		if e = add(p.raw["dependencies"], "runtime"); e != nil {
			return nil, nil, e
		}
		for _, k := range []string{"optional-dependencies", "dev-dependencies", "dependency-groups"} {
			if v, exists := p.raw[k]; exists {
				groups, ok := v.(map[string]any)
				if !ok {
					return nil, nil, bad("uv dependency groups must be a table")
				}
				for group, v := range groups {
					if e = add(v, k+":"+group); e != nil {
						return nil, nil, e
					}
				}
			}
		}
		sort.Slice(d.Requirements, func(i, j int) bool {
			a, b := d.Requirements[i], d.Requirements[j]
			if a.Target != b.Target {
				return a.Target < b.Target
			}
			if a.Constraint != b.Constraint {
				return a.Constraint < b.Constraint
			}
			if a.Scope != b.Scope {
				return a.Scope < b.Scope
			}
			return a.Condition < b.Condition
		})
		if len(d.Requirements) > 0 {
			deps = append(deps, d)
		}
	}
	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))
	return libs, deps, nil
}

// JSONC accepts comments and trailing commas only; it does not broaden the
// strict bounded JSON grammar used by the other lock analyzers. Blanking keeps
// byte offsets and source line ranges unchanged.
func jsonc(ctx context.Context, raw []byte) ([]byte, error) {
	if !utf8.Valid(raw) {
		return nil, bad("JSONC requires UTF-8")
	}
	if e := budget.From(ctx).Working(int64(len(raw)) * 2); e != nil {
		return nil, e
	}
	out := append([]byte(nil), raw...)
	in, escape := false, false
	for i := 0; i < len(out); i++ {
		if i%4096 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		c := out[i]
		if in {
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				in = false
			}
			continue
		}
		if c == '"' {
			in = true
			continue
		}
		if c == '/' && i+1 < len(out) {
			switch out[i+1] {
			case '/':
				out[i] = ' '
				i++
				for i < len(out) && out[i] != '\n' {
					out[i] = ' '
					i++
				}
			case '*':
				out[i] = ' '
				out[i+1] = ' '
				i += 2
				closed := false
				for i < len(out) {
					if i+1 < len(out) && out[i] == '*' && out[i+1] == '/' {
						out[i] = ' '
						out[i+1] = ' '
						i++
						closed = true
						break
					}
					if out[i] != '\n' && out[i] != '\r' {
						out[i] = ' '
					}
					i++
				}
				if !closed {
					return nil, bad("unterminated JSONC comment")
				}
			}
		}
	}
	in = false
	escape = false
	prev := byte(0)
	for i, c := range out {
		if i%4096 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		if in {
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				in = false
				prev = '"'
			}
			continue
		}
		if c == '"' {
			in = true
			continue
		}
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(out) && (out[j] == ' ' || out[j] == '\n' || out[j] == '\r' || out[j] == '\t') {
				j++
			}
			if j < len(out) && (out[j] == ']' || out[j] == '}') && prev != 0 && prev != '[' && prev != '{' && prev != ',' && prev != ':' {
				out[i] = ' '
				continue
			}
		}
		prev = c
	}
	return out, nil
}
