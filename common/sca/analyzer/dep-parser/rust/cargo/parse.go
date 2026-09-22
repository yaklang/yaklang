package cargo

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/locktoml"
	"github.com/yaklang/yaklang/common/sca/internal/digest"
	"github.com/yaklang/yaklang/common/sca/model"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type cargoPkg struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Source       string   `json:"source,omitempty"`
	Checksum     string   `json:"checksum,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}
type Lockfile struct {
	Version  int        `json:"version"`
	Packages []cargoPkg `json:"package"`
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	raw, spans, err := locktoml.ReadRecords(ctx, r, "package")
	if err != nil {
		return nil, nil, fmt.Errorf("decode error: %w", err)
	}
	if err := textdecode.ReserveRecordConversion(ctx, raw); err != nil {
		return nil, nil, err
	}
	lockfile, err := decodeLock(ctx, raw)
	if err != nil {
		return nil, nil, fmt.Errorf("decode error: %w", err)
	}
	if lockfile.Version != 0 && lockfile.Version != 1 && lockfile.Version != 2 && lockfile.Version != 3 && lockfile.Version != 4 {
		return nil, nil, fmt.Errorf("unsupported_syntax: cargo lock version %d", lockfile.Version)
	}

	// v4 source URLs remain encoded exactly as written. Decode neither identity
	// nor references: %2B and + must not collapse into the same source.
	pkgs := cargoIndex{name: map[string][]cargoPkg{}, version: map[[2]string][]cargoPkg{}, full: map[[3]string][]cargoPkg{}}
	for _, pkg := range lockfile.Packages {
		pkgs.name[pkg.Name] = append(pkgs.name[pkg.Name], pkg)
		k := [2]string{pkg.Name, pkg.Version}
		pkgs.version[k] = append(pkgs.version[k], pkg)
		f := [3]string{pkg.Name, pkg.Version, pkg.Source}
		pkgs.full[f] = append(pkgs.full[f], pkg)
	}

	var libs []types.Library
	var deps []types.Dependency
	for index, pkg := range lockfile.Packages {
		if strings.TrimSpace(pkg.Name) == "" {
			return nil, nil, fmt.Errorf("malformed_input: cargo package identity")
		}
		pkgID := nativeID(pkg)
		lib := types.Library{
			ID:      pkgID,
			Name:    pkg.Name,
			Source:  pkg.Source,
			Version: pkg.Version,
		}
		if pkg.Checksum != "" {
			declared := digest.ParseDeclared("sha256:" + pkg.Checksum)
			lib.Verification = declared.Canonical
			lib.DeclaredIntegrity = pkg.Checksum
			for _, issue := range declared.Issues {
				lib.Diagnostics = append(lib.Diagnostics, model.Diagnostic{Code: "malformed_input", Stage: "cargo", Reason: issue, Incomplete: true})
			}
		}
		if index < len(spans) {
			lib.Locations = []types.Location{{StartLine: spans[index].StartLine, EndLine: spans[index].EndLine}}
		}

		libs = append(libs, lib)
		dep := parseDependencies(pkgID, pkg, pkgs)
		if dep != nil {
			deps = append(deps, *dep)
		}
	}
	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))
	return libs, deps, nil
}

func nativeID(p cargoPkg) string {
	b, _ := json.Marshal([]string{p.Name, p.Version, p.Source})
	return string(b)
}
func parseDependencies(id string, pkg cargoPkg, index cargoIndex) *types.Dependency {
	dep := &types.Dependency{ID: id}
	for _, raw := range pkg.Dependencies {
		f := strings.Fields(raw)
		var matches []cargoPkg
		name, constraint := "", ""
		switch len(f) {
		case 1:
			name = f[0]
			matches = index.name[f[0]]
		case 2:
			name, constraint = f[0], f[1]
			matches = index.version[[2]string{f[0], f[1]}]
		case 3:
			name, constraint = f[0], f[1]
			src := strings.TrimSuffix(strings.TrimPrefix(f[2], "("), ")")
			matches = index.full[[3]string{f[0], f[1], src}]
		}
		if name == "" {
			continue
		}
		// Constraint is only the version token present in the lock line.
		// Name-only "foo" stays unconstrained; the locked version belongs on
		// the uniquely Resolved package, not on the declaration.
		req := types.Requirement{Target: name, Constraint: constraint, Condition: raw}
		if len(matches) == 1 {
			nid := nativeID(matches[0])
			dep.DependsOn = append(dep.DependsOn, nid)
			req.Resolved = nid
		}
		dep.Requirements = append(dep.Requirements, req)
	}
	if len(dep.DependsOn) == 0 && len(dep.Requirements) == 0 {
		return nil
	}
	sort.Strings(dep.DependsOn)
	return dep
}

type cargoIndex struct {
	name    map[string][]cargoPkg
	version map[[2]string][]cargoPkg
	full    map[[3]string][]cargoPkg
}

func decodeLock(ctx context.Context, m map[string]any) (Lockfile, error) {
	var out Lockfile
	version, err := locktoml.Field[int64](m, "version")
	if err != nil {
		return out, err
	}
	if int64(int(version)) != version {
		return out, fmt.Errorf("unsupported_syntax: cargo lock version overflow")
	}
	out.Version = int(version)
	records, err := locktoml.Field[[]any](m, "package")
	if err != nil {
		return out, err
	}
	if records == nil {
		return out, nil
	}
	need, err := budget.SizeMul(len(records), 4*budget.SizeString+budget.SizeSlice)
	if err != nil {
		return out, err
	}
	if err = budget.From(ctx).Working(need + budget.SizeSlice); err != nil {
		return out, err
	}
	out.Packages = make([]cargoPkg, len(records))
	for i, record := range records {
		if err := ctx.Err(); err != nil {
			return Lockfile{}, err
		}
		r, err := locktoml.Table(record)
		if err != nil {
			return Lockfile{}, err
		}
		p := &out.Packages[i]
		for _, f := range []struct {
			k   string
			out *string
		}{{"name", &p.Name}, {"version", &p.Version}, {"source", &p.Source}, {"checksum", &p.Checksum}} {
			*f.out, err = locktoml.Field[string](r, f.k)
			if err != nil {
				return Lockfile{}, err
			}
		}
		deps, err := locktoml.Field[[]any](r, "dependencies")
		if err != nil {
			return Lockfile{}, err
		}
		if deps != nil {
			need, err := budget.SizeMul(len(deps), budget.SizeString)
			if err != nil {
				return Lockfile{}, err
			}
			if err = budget.From(ctx).Working(need + budget.SizeSlice); err != nil {
				return Lockfile{}, err
			}
			p.Dependencies = make([]string, len(deps))
			for j, v := range deps {
				s, ok := v.(string)
				if !ok {
					return Lockfile{}, fmt.Errorf("malformed_input: cargo dependency must be a string")
				}
				p.Dependencies[j] = s
			}
		}
	}
	return out, nil
}
