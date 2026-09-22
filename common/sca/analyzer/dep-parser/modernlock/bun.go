package modernlock

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"github.com/yaklang/yaklang/common/sca/internal/digest"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Bun struct{}
type bunInfo struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalPeers        []string          `json:"optionalPeers"`
	OS                   any               `json:"os"`
	CPU                  any               `json:"cpu"`
}
type bunLock struct {
	Overrides           map[string]any               `json:"overrides"`
	PatchedDependencies map[string]string            `json:"patchedDependencies"`
	Catalog             map[string]string            `json:"catalog"`
	Catalogs            map[string]map[string]string `json:"catalogs"`
	Version             *int                         `json:"lockfileVersion"`
	Config              *int                         `json:"configVersion"`
	Workspaces          map[string]json.RawMessage   `json:"workspaces"`
	Packages            map[string][]json.RawMessage `json:"packages"`
}

func bunPath(s string) bool {
	if s == "" || strings.ContainsAny(s, "\\\x00") {
		return false
	}
	parts := strings.Split(s, "/")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "" || parts[i] == "." || parts[i] == ".." {
			return false
		}
		if strings.HasPrefix(parts[i], "@") {
			i++
			if i >= len(parts) || parts[i] == "" || parts[i] == "." || parts[i] == ".." || strings.HasPrefix(parts[i], "@") {
				return false
			}
		}
	}
	return true
}
func bunParent(s string) string {
	i := strings.LastIndex(s, "/")
	if i < 0 {
		return ""
	}
	p := s[:i]
	j := strings.LastIndex(p, "/")
	if strings.HasPrefix(p[j+1:], "@") {
		if j < 0 {
			return ""
		}
		return p[:j]
	}
	return p
}
func bunTarget(s string) bool           { return bunPath(s) && bunParent(s) == "" }
func bunObject(n *jsonrecord.Node) bool { return n != nil && n.Object != nil }
func bunString(raw json.RawMessage) (string, error) {
	var s string
	if len(raw) == 0 || raw[0] != '"' {
		return "", bad("Bun tuple field must be a string")
	}
	if e := json.Unmarshal(raw, &s); e != nil {
		return "", e
	}
	return s, nil
}
func bunInfoDecode(raw []byte, n *jsonrecord.Node) (bunInfo, error) {
	var info bunInfo
	if !bunObject(n) {
		return info, bad("Bun metadata must be an object")
	}
	if e := json.Unmarshal(raw, &info); e != nil {
		return info, e
	}
	for _, k := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
		if v := n.Get(k); v != nil {
			if !bunObject(v) {
				return info, bad("Bun dependencies must be an object")
			}
			for name, value := range v.Object {
				if !bunTarget(name) || len(value.Raw) == 0 || value.Raw[0] != '"' {
					return info, bad("invalid Bun dependency declaration")
				}
			}
		}
	}
	for _, k := range []string{"bundledDependencies", "bundleDependencies", "libc"} {
		if n.Get(k) != nil {
			return info, scanerr.New(scanerr.UnsupportedSyntax, "Bun metadata %q", k)
		}
	}
	for _, v := range []any{info.OS, info.CPU} {
		switch x := v.(type) {
		case nil, string:
		case []any:
			for _, y := range x {
				if _, ok := y.(string); !ok {
					return info, bad("invalid Bun platform selector")
				}
			}
		default:
			return info, bad("invalid Bun platform selector")
		}
	}
	return info, nil
}

type bunRecord struct {
	lib       types.Library
	info      bunInfo
	path      string
	workspace bool
}

func (Bun) Parse(_ fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	raw, e := textdecode.ReadRaw(ctx, r, 16<<20)
	if e != nil {
		return nil, nil, e
	}
	raw, e = jsonc(ctx, raw)
	if e != nil {
		return nil, nil, e
	}
	var lock bunLock
	n, e := jsonrecord.Decode(ctx, raw, &lock)
	if e != nil {
		return nil, nil, e
	}
	if lock.Version == nil {
		return nil, nil, bad("missing Bun lockfileVersion")
	}
	v := *lock.Version
	if v < 0 || v > 3 {
		return nil, nil, scanerr.New(scanerr.UnsupportedSyntax, "Bun text lock version %d", v)
	}
	if lock.Config != nil && *lock.Config != 1 {
		return nil, nil, scanerr.New(scanerr.UnsupportedSyntax, "Bun config version %d", *lock.Config)
	}
	if !bunObject(n.Get("workspaces")) || !bunObject(n.Get("packages")) {
		return nil, nil, bad("Bun requires workspace and package maps")
	}
	for _, key := range []string{"overrides", "patchedDependencies", "catalog", "catalogs"} {
		if node := n.Get(key); node != nil && !bunObject(node) {
			return nil, nil, bad("Bun root declarations must be objects")
		}
	}
	for _, key := range []string{"patchedDependencies", "catalog"} {
		if node := n.Get(key); node != nil {
			for _, value := range node.Object {
				if len(value.Raw) == 0 || value.Raw[0] != '"' {
					return nil, nil, bad("Bun declaration value must be a string")
				}
			}
		}
	}
	if catalogs := n.Get("catalogs"); catalogs != nil {
		for _, catalog := range catalogs.Object {
			if !bunObject(catalog) {
				return nil, nil, bad("Bun catalog must be an object")
			}
			for _, value := range catalog.Object {
				if len(value.Raw) == 0 || value.Raw[0] != '"' {
					return nil, nil, bad("Bun catalog version must be a string")
				}
			}
		}
	}
	var validateOverrides func(map[string]any) error
	validateOverrides = func(m map[string]any) error {
		for _, v := range m {
			switch x := v.(type) {
			case string:
			case map[string]any:
				if e := validateOverrides(x); e != nil {
					return e
				}
			default:
				return bad("Bun override must be a string or scoped object")
			}
		}
		return nil
	}
	if e = validateOverrides(lock.Overrides); e != nil {
		return nil, nil, e
	}
	declarations := encoded(map[string]any{"overrides": lock.Overrides, "patchedDependencies": lock.PatchedDependencies, "catalog": lock.Catalog, "catalogs": lock.Catalogs})
	if e = budget.From(ctx).Working(int64(len(lock.Packages)+len(lock.Workspaces)) * (4096 + 24*int64(len(declarations)))); e != nil {
		return nil, nil, e
	}
	workspaces := map[string]bunInfo{}
	records := []bunRecord{}
	index := map[string]string{}
	for path, raw := range lock.Workspaces {
		if e = ctx.Err(); e != nil {
			return nil, nil, e
		}
		node := n.Get("workspaces", path)
		info, e := bunInfoDecode(raw, node)
		if e != nil {
			return nil, nil, e
		}
		if info.Name == "" {
			return nil, nil, bad("Bun workspace requires name")
		}
		workspaces[path] = info
		a, b := node.Lines()
		lib := types.Library{ID: encoded([]string{"workspace", path}), Name: info.Name, Version: info.Version, Source: "workspace:" + path, Variant: encoded(map[string]any{"workspace": path, "declarations": declarations}), Evidence: "declared", Scope: "workspace", Locations: types.Locations{{StartLine: a, EndLine: b}}}
		records = append(records, bunRecord{lib, info, "", true})
	}
	for path, tuple := range lock.Packages {
		if e = ctx.Err(); e != nil {
			return nil, nil, e
		}
		if !bunPath(path) || len(tuple) == 0 {
			return nil, nil, bad("invalid Bun package path or tuple")
		}
		node := n.Get("packages", path)
		first, e := bunString(tuple[0])
		if e != nil {
			return nil, nil, e
		}
		at := -1
		if len(first) > 1 {
			at = strings.Index(first[1:], "@")
			if at >= 0 {
				at++
			}
		}
		if at < 1 || at == len(first)-1 {
			return nil, nil, bad("invalid Bun resolution")
		}
		name, resolution := first[:at], first[at+1:]
		if !bunTarget(name) {
			return nil, nil, bad("invalid Bun resolved name")
		}
		lib := types.Library{ID: encoded([]string{"package", path}), Name: name, Evidence: "locked", Scope: "directness:unknown"}
		var info bunInfo
		source, tag, hash := "", "", ""
		kind := ""
		metadata := -1
		switch {
		case strings.HasPrefix(resolution, "workspace:"):
			kind = "workspace"
			source = resolution
			expected := 1
			if v == 0 {
				expected = 2
				metadata = 1
			}
			if len(tuple) != expected {
				return nil, nil, bad("invalid Bun workspace tuple")
			}
			var ok bool
			info, ok = workspaces[strings.TrimPrefix(resolution, "workspace:")]
			if !ok {
				return nil, nil, bad("Bun workspace reference is absent")
			}
			lib.Version = info.Version
			lib.Evidence = "declared"
		case strings.HasPrefix(resolution, "file:"), strings.HasPrefix(resolution, "link:"):
			kind = "local"
			source = resolution
			metadata = 1
			if len(tuple) != 2 {
				return nil, nil, bad("invalid Bun local tuple")
			}
			lib.Evidence = "declared"
		case strings.HasPrefix(resolution, "git+"), strings.HasPrefix(resolution, "git:"), strings.HasPrefix(resolution, "github:"):
			kind = "git"
			source = resolution
			metadata = 1
			if len(tuple) != 3 {
				return nil, nil, bad("invalid Bun git tuple")
			}
			tag, e = bunString(tuple[2])
			if e != nil {
				return nil, nil, e
			}
			if tag == "" || strings.ContainsAny(tag, "/\\\x00") || tag == "." || tag == ".." {
				return nil, nil, bad("invalid Bun git tag")
			}
		case strings.HasPrefix(resolution, "https://"), strings.HasPrefix(resolution, "http://"):
			kind = "tarball"
			source = resolution
			metadata = 1
			if len(tuple) != 2 {
				return nil, nil, bad("invalid Bun tarball tuple")
			}
		case len(tuple) == 4 && resolution[0] >= '0' && resolution[0] <= '9':
			kind = "registry"
			lib.Version = resolution
			metadata = 2
			source, e = bunString(tuple[1])
			if e != nil {
				return nil, nil, e
			}
			hash, e = bunString(tuple[3])
			if e != nil {
				return nil, nil, e
			}
			if v >= 2 && source != "" && hash == "" {
				return nil, nil, bad("Bun v2 explicit registry URL requires integrity")
			}
		default:
			return nil, nil, scanerr.New(scanerr.UnsupportedSyntax, "Bun resolution %q", resolution)
		}
		if metadata >= 0 {
			decoded, e := bunInfoDecode(tuple[metadata], node.Elements()[metadata])
			if e != nil {
				return nil, nil, e
			}
			if kind != "workspace" {
				info = decoded
			}
		}
		// A workspace can be installed at several locations. Keep each observation's
		// dependency lookup path instead of flattening them into a global name index.
		if kind == "workspace" {
			if e = budget.From(ctx).Working(int64(len(lock.Workspaces[strings.TrimPrefix(resolution, "workspace:")]))*24 + 4096); e != nil {
				return nil, nil, e
			}
		}
		d := digest.ParseDeclared(hash)
		if len(d.Issues) > 0 {
			return nil, nil, bad("invalid Bun integrity")
		}
		lib.Source = source
		lib.Verification = d.Canonical
		lib.DeclaredIntegrity = hash
		lib.Variant = encoded(map[string]any{"path": path, "kind": kind, "resolution": resolution, "tag": tag, "declarations": declarations})
		lib.Condition = encoded(map[string]any{"os": info.OS, "cpu": info.CPU})
		a, b := node.Lines()
		lib.Locations = types.Locations{{StartLine: a, EndLine: b}}
		index[path] = lib.ID
		records = append(records, bunRecord{lib, info, path, false})
	}
	steps := 0
	resolve := func(path, target string) (string, error) {
		for {
			steps++
			if steps > budget.From(ctx).Limits.MaxResolveSteps {
				return "", scanerr.New(scanerr.ResourceLimit, "Bun resolution steps")
			}
			if e := ctx.Err(); e != nil {
				return "", e
			}
			if e := budget.From(ctx).Working(int64(len(path) + len(target) + 1)); e != nil {
				return "", e
			}
			key := target
			if path != "" {
				key = path + "/" + target
			}
			if id := index[key]; id != "" {
				return id, nil
			}
			if path == "" {
				return "", nil
			}
			path = bunParent(path)
		}
	}
	var libs []types.Library
	var deps []types.Dependency
	for _, rec := range records {
		if e = ctx.Err(); e != nil {
			return nil, nil, e
		}
		libs = append(libs, rec.lib)
		d := types.Dependency{ID: rec.lib.ID}
		// Uninstalled non-root workspace declarations are retained but cannot borrow
		// the root's resolution: their installation location is not in this snapshot.
		rootWorkspace := rec.lib.Source == "workspace:"
		for _, group := range []struct {
			scope string
			m     map[string]string
		}{{"runtime", rec.info.Dependencies}, {"development", rec.info.DevDependencies}, {"optional", rec.info.OptionalDependencies}, {"peer", rec.info.PeerDependencies}} {
			if e = budget.From(ctx).Working(int64(len(group.m)) * (1024 + 24*int64(len(rec.lib.Condition)+len(rec.path)))); e != nil {
				return nil, nil, e
			}
			for target, constraint := range group.m {
				q := types.Requirement{Target: target, Constraint: constraint, Scope: group.scope, Condition: rec.lib.Condition}
				if !rec.workspace || rootWorkspace {
					q.Resolved, e = resolve(rec.path, target)
					if e != nil {
						return nil, nil, e
					}
				}
				for _, p := range rec.info.OptionalPeers {
					if p == target && group.scope == "peer" {
						q.Scope = "optional-peer"
					}
				}
				d.Requirements = append(d.Requirements, q)
			}
		}
		sort.Slice(d.Requirements, func(i, j int) bool {
			a, b := d.Requirements[i], d.Requirements[j]
			if a.Target != b.Target {
				return a.Target < b.Target
			}
			return a.Scope < b.Scope
		})
		if len(d.Requirements) > 0 {
			deps = append(deps, d)
		}
	}
	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))
	return libs, deps, nil
}
