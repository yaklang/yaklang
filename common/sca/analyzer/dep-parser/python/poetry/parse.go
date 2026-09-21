package poetry

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/locktoml"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/internal/digest"
	"github.com/yaklang/yaklang/common/sca/model"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type poetrySource struct {
	Type        string `json:"type"`
	URL         string `json:"url"`
	Reference   string `json:"reference"`
	ResolvedRef string `json:"resolved_reference"`
	Directory   string `json:"directory"`
}

type poetryPackage struct {
	Category       string                 `json:"category"`
	Description    string                 `json:"description"`
	Marker         string                 `json:"marker,omitempty"`
	Name           string                 `json:"name"`
	Optional       bool                   `json:"optional"`
	PythonVersions string                 `json:"python-versions"`
	Version        string                 `json:"version"`
	Dependencies   map[string]interface{} `json:"dependencies"`
	Source         *poetrySource          `json:"source"`
	Files          []struct {
		File string `json:"file"`
		Hash string `json:"hash"`
	} `json:"files"`
	Metadata interface{}
}
type Lockfile struct {
	Packages []poetryPackage `json:"package"`
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	raw, spans, err := locktoml.ReadRecords(ctx, r, "package")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode poetry.lock: %w", err)
	}
	if err := textdecode.ReserveRecordConversion(ctx, raw); err != nil {
		return nil, nil, err
	}
	lockfile, err := decodeLock(ctx, raw)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode poetry.lock: %w", err)
	}

	libIndex := parseIndex(lockfile)

	var libs []types.Library
	var deps []types.Dependency
	for index, pkg := range lockfile.Packages {
		if pkg.Category == "dev" {
			continue
		}

		origin := poetryOrigin(pkg.Source)
		pkgID := poetryNativeID(pkg.Name, pkg.Version, origin)
		scope := pkg.Category
		if pkg.Optional {
			scope = "optional"
		}
		var hashes []string
		for _, f := range pkg.Files {
			if f.Hash != "" {
				hashes = append(hashes, f.Hash)
			}
		}
		declared := digest.ParseDeclared(strings.Join(hashes, " "))
		lib := types.Library{
			ID:                pkgID,
			Name:              pkg.Name,
			Condition:         pkg.Marker,
			Scope:             scope,
			Version:           pkg.Version,
			Source:            origin,
			Verification:      declared.Canonical,
			DeclaredIntegrity: declared.Original,
		}
		if index < len(spans) {
			lib.Locations = []types.Location{{StartLine: spans[index].StartLine, EndLine: spans[index].EndLine}}
		}
		for _, issue := range declared.Issues {
			lib.Diagnostics = append(lib.Diagnostics, model.Diagnostic{Code: "malformed_input", Stage: "poetry", Reason: issue, Incomplete: true})
		}
		libs = append(libs, lib)

		reqs := poetryRequirements(pkg.Dependencies)
		for i := range reqs {
			if id, err := parseDependency(reqs[i].Target, nil, libIndex); err == nil {
				reqs[i].Resolved = id
			}
		}
		dependsOn := parseDependencies(pkg.Dependencies, libIndex)
		if len(dependsOn) != 0 || len(reqs) != 0 {
			deps = append(deps, types.Dependency{
				ID:           pkgID,
				Requirements: reqs,
				DependsOn:    dependsOn,
			})
		}
	}
	return libs, deps, nil
}

func poetryOrigin(src *poetrySource) string {
	if src == nil {
		return ""
	}
	origin := src.URL
	if origin == "" {
		origin = src.Directory
	}
	if origin == "" {
		origin = src.Type
	}
	ref := src.ResolvedRef
	if ref == "" {
		ref = src.Reference
	}
	if ref != "" && origin != "" {
		origin += "#" + ref
	}
	return origin
}

func poetryRequirements(deps map[string]any) []types.Requirement {
	var reqs []types.Requirement
	for name, raw := range deps {
		q := types.Requirement{Target: normalizePkgName(name)}
		switch v := raw.(type) {
		case string:
			q.Constraint = v
		case map[string]any:
			if text, ok := v["version"].(string); ok {
				q.Constraint = text
			}
			var cond []string
			if text, ok := v["markers"].(string); ok && text != "" {
				cond = append(cond, text)
			}
			if extras := extrasText(v["extras"]); extras != "" {
				cond = append(cond, "extras=["+extras+"]")
			}
			q.Condition = strings.Join(cond, "; ")
			if opt, ok := v["optional"].(bool); ok && opt {
				q.Scope = "optional"
			}
		default:
			if v != nil {
				q.Constraint = fmt.Sprint(v)
			}
		}
		reqs = append(reqs, q)
	}
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].Target < reqs[j].Target })
	return reqs
}

func extrasText(v any) string {
	if v == nil {
		return ""
	}
	var names []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			s := strings.TrimSpace(fmt.Sprint(x))
			if s != "" {
				names = append(names, s)
			}
		}
	case []string:
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				names = append(names, s)
			}
		}
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		if s == "" {
			return ""
		}
		return s
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

type poetryIdent struct {
	Version, Source, ID string
}

func poetryNativeID(name, version, source string) string {
	b, _ := json.Marshal([]string{name, version, source})
	return string(b)
}

func parseIndex(lockfile Lockfile) map[string][]poetryIdent {
	idx := map[string][]poetryIdent{}
	for _, pkg := range lockfile.Packages {
		if pkg.Category == "dev" {
			continue
		}
		origin := poetryOrigin(pkg.Source)
		name := normalizePkgName(pkg.Name)
		idx[name] = append(idx[name], poetryIdent{
			Version: pkg.Version,
			Source:  origin,
			ID:      poetryNativeID(pkg.Name, pkg.Version, origin),
		})
	}
	return idx
}

func parseDependencies(deps map[string]any, libIndex map[string][]poetryIdent) []string {
	var dependsOn []string
	for name, versRange := range deps {
		// Unresolved declarations remain in parseRequirements; this helper
		// only contributes uniquely bound lock identities.
		if dep, err := parseDependency(name, versRange, libIndex); err == nil && dep != "" {
			dependsOn = append(dependsOn, dep)
		}
	}
	sort.Slice(dependsOn, func(i, j int) bool {
		return dependsOn[i] < dependsOn[j]
	})
	return dependsOn
}

func parseDependency(name string, versRange any, libIndex map[string][]poetryIdent) (string, error) {
	name = normalizePkgName(name)
	hits := libIndex[name]
	if len(hits) == 0 {
		return "", fmt.Errorf("no version found for %q", name)
	}
	if len(hits) > 1 {
		return "", fmt.Errorf("ambiguous locked versions for %q", name)
	}
	return hits[0].ID, nil
}

func normalizePkgName(name string) string {
	// The package names don't use `_`, `.` or upper case, but dependency names can contain them.
	// We need to normalize those names.
	name = strings.ToLower(name)              // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L819
	name = strings.ReplaceAll(name, "_", "-") // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L50
	name = strings.ReplaceAll(name, ".", "-") // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L816
	return name
}

func decodeLock(ctx context.Context, m map[string]any) (Lockfile, error) {
	var out Lockfile
	records, err := locktoml.Field[[]any](m, "package")
	if err != nil {
		return out, err
	}
	if records == nil {
		return out, nil
	}
	need, err := budget.SizeMul(len(records), 192)
	if err != nil {
		return out, err
	}
	if err = budget.From(ctx).Working(need + budget.SizeSlice); err != nil {
		return out, err
	}
	out.Packages = make([]poetryPackage, len(records))
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
		}{{"category", &p.Category}, {"description", &p.Description}, {"marker", &p.Marker}, {"name", &p.Name}, {"python-versions", &p.PythonVersions}, {"version", &p.Version}} {
			*f.out, err = locktoml.Field[string](r, f.k)
			if err != nil {
				return Lockfile{}, err
			}
		}
		p.Optional, err = locktoml.Field[bool](r, "optional")
		if err != nil {
			return Lockfile{}, err
		}
		p.Dependencies, err = locktoml.Field[map[string]any](r, "dependencies")
		if err != nil {
			return Lockfile{}, err
		}
		p.Metadata = r["metadata"]
		source, err := locktoml.Field[map[string]any](r, "source")
		if err != nil {
			return Lockfile{}, err
		}
		if source != nil {
			if err = budget.From(ctx).Working(5*budget.SizeString + budget.SizeObject); err != nil {
				return Lockfile{}, err
			}
			p.Source = &poetrySource{}
			for _, f := range []struct {
				k   string
				out *string
			}{{"type", &p.Source.Type}, {"url", &p.Source.URL}, {"reference", &p.Source.Reference}, {"resolved_reference", &p.Source.ResolvedRef}, {"directory", &p.Source.Directory}} {
				*f.out, err = locktoml.Field[string](source, f.k)
				if err != nil {
					return Lockfile{}, err
				}
			}
		}
		files, err := locktoml.Field[[]any](r, "files")
		if err != nil {
			return Lockfile{}, err
		}
		if files != nil {
			need, err := budget.SizeMul(len(files), 2*budget.SizeString)
			if err != nil {
				return Lockfile{}, err
			}
			if err = budget.From(ctx).Working(need + budget.SizeSlice); err != nil {
				return Lockfile{}, err
			}
			p.Files = make([]struct {
				File string `json:"file"`
				Hash string `json:"hash"`
			}, len(files))
			for j, v := range files {
				f, err := locktoml.Table(v)
				if err != nil {
					return Lockfile{}, err
				}
				p.Files[j].File, err = locktoml.Field[string](f, "file")
				if err != nil {
					return Lockfile{}, err
				}
				p.Files[j].Hash, err = locktoml.Field[string](f, "hash")
				if err != nil {
					return Lockfile{}, err
				}
			}
		}
	}
	return out, nil
}
