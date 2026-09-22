// Package lockjson implements static NuGet and Swift lock records, without
// invoking either package manager or resolving anything outside the snapshot.
package lockjson

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Nuget struct{}
type nugetLock struct {
	Version      int                                `json:"version"`
	Dependencies map[string]map[string]nugetPackage `json:"dependencies"`
}
type nugetPackage struct {
	Type         string             `json:"type"`
	Requested    string             `json:"requested"`
	Resolved     string             `json:"resolved"`
	ContentHash  string             `json:"contentHash"`
	Dependencies map[string]*string `json:"dependencies"`
}

// Native IDs are JSON tuples, not delimiter joins: input strings may contain
// delimiters or NUL. Framework/RID groups never share a resolution index.
func native(parts ...string) string  { b, _ := json.Marshal(parts); return string(b) }
func malformed(reason string) error  { return scanerr.New(scanerr.MalformedInput, "%s", reason) }
func object(n *jsonrecord.Node) bool { return n != nil && n.Object != nil }
func array(n *jsonrecord.Node) bool  { return n != nil && len(n.Raw) > 0 && n.Raw[0] == '[' }
func location(n *jsonrecord.Node) types.Locations {
	a, b := n.Lines()
	return types.Locations{{StartLine: a, EndLine: b}}
}

func (Nuget) Parse(_ fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	raw, err := textdecode.ReadRaw(ctx, r, 16<<20)
	if err != nil {
		return nil, nil, err
	}
	var lock nugetLock
	nodes, err := jsonrecord.Decode(ctx, raw, &lock)
	if err != nil {
		return nil, nil, err
	}
	if nodes.Get("version") == nil || lock.Version < 1 {
		return nil, nil, malformed("missing NuGet lock version")
	}
	if lock.Version != 1 && lock.Version != 2 {
		return nil, nil, scanerr.New(scanerr.UnsupportedSyntax, "NuGet lock version %d", lock.Version)
	}
	if !object(nodes.Get("dependencies")) {
		return nil, nil, malformed("NuGet dependencies must be a framework map")
	}
	var libs []types.Library
	var deps []types.Dependency
	for framework, packages := range lock.Dependencies {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		group := nodes.Get("dependencies", framework)
		if strings.TrimSpace(framework) == "" || !object(group) {
			return nil, nil, malformed("invalid NuGet framework group")
		}
		// Reserve the per-framework index, IDs, output records and sorting scratch
		// before allocating. JSON DTO conversion already reserves field storage.
		if err := budget.From(ctx).Working(int64(len(packages)) * (2048 + 24*int64(len(framework)))); err != nil {
			return nil, nil, err
		}
		index := make(map[string]string, len(packages))
		for name, p := range packages {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			n := group.Get(name)
			if !object(n) || strings.TrimSpace(name) == "" {
				return nil, nil, malformed("invalid NuGet package record")
			}
			lower := strings.ToLower(name)
			if _, exists := index[lower]; exists {
				return nil, nil, malformed("case-insensitive NuGet package identity collision")
			}
			kind := strings.ToLower(p.Type)
			switch kind {
			case "direct", "transitive", "centraltransitive", "project":
			default:
				return nil, nil, scanerr.New(scanerr.UnsupportedSyntax, "NuGet dependency type %q", p.Type)
			}
			if kind != "project" && p.Resolved == "" {
				return nil, nil, malformed("NuGet package has no resolved version")
			}
			if n.Get("dependencies") != nil && !object(n.Get("dependencies")) {
				return nil, nil, malformed("NuGet package dependencies must be an object")
			}
			id := native(framework, lower)
			index[lower] = id
			lib := types.Library{ID: id, Name: lower, DeclaredName: name, Version: p.Resolved, DeclaredVersion: p.Requested, Scope: p.Type, Condition: framework, Variant: framework, Indirect: kind == "transitive" || kind == "centraltransitive", Evidence: "locked", Locations: location(n), DeclaredIntegrity: p.ContentHash}
			if kind == "project" {
				lib.Evidence = "declared"
				lib.Variant = native(framework, "project")
			}
			if p.ContentHash != "" {
				hash, e := base64.StdEncoding.DecodeString(p.ContentHash)
				if e != nil || len(hash) != 64 {
					return nil, nil, malformed("NuGet contentHash must be a base64 SHA-512 digest")
				}
				lib.Verification = "sha512:" + hex.EncodeToString(hash)
			}
			libs = append(libs, lib)
		}
		for name, p := range packages {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if err := budget.From(ctx).Working(int64(len(p.Dependencies)) * (512 + 24*int64(len(framework)))); err != nil {
				return nil, nil, err
			}
			d := types.Dependency{ID: index[strings.ToLower(name)]}
			for target, constraint := range p.Dependencies {
				if strings.TrimSpace(target) == "" {
					return nil, nil, malformed("empty NuGet dependency name")
				}
				q := types.Requirement{Target: target, Condition: framework, Resolved: index[strings.ToLower(target)]}
				if constraint != nil {
					q.Constraint = *constraint
				}
				// The lock group is the authoritative result of restore, not a new
				// semver solver. Missing targets never fall back to another framework.
				d.Requirements = append(d.Requirements, q)
				if q.Resolved != "" {
					d.DependsOn = append(d.DependsOn, q.Resolved)
				}
			}
			sort.Slice(d.Requirements, func(i, j int) bool { return d.Requirements[i].Target < d.Requirements[j].Target })
			sort.Strings(d.DependsOn)
			if len(d.Requirements) > 0 {
				deps = append(deps, d)
			}
		}
	}
	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))
	return libs, deps, nil
}
