package pnpm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/lockyaml"
	"github.com/yaklang/yaklang/common/sca/internal/digest"
	"github.com/yaklang/yaklang/common/sca/model"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type PackageResolution struct {
	Tarball   string `json:"tarball,omitempty"`
	Integrity string `json:"integrity,omitempty"`
}

type PackageInfo struct {
	Resolution           PackageResolution `json:"resolution"`
	Dependencies         map[string]string `json:"dependencies,omitempty"`
	DevDependencies      map[string]string `json:"devDependencies,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	IsDev                bool              `json:"dev,omitempty"`
	Name                 string            `json:"name,omitempty"`
	Version              string            `json:"version,omitempty"`
}

type pnpmImporter struct {
	Specifiers           map[string]any `json:"specifiers,omitempty"`
	Dependencies         map[string]any `json:"dependencies,omitempty"`
	DevDependencies      map[string]any `json:"devDependencies,omitempty"`
	OptionalDependencies map[string]any `json:"optionalDependencies,omitempty"`
}

type LockFile struct {
	LockfileVersion any                     `json:"lockfileVersion"`
	Specifiers      map[string]any          `json:"specifiers,omitempty"`
	Dependencies    map[string]any          `json:"dependencies,omitempty"`
	DevDependencies map[string]any          `json:"devDependencies,omitempty"`
	Importers       map[string]pnpmImporter `json:"importers,omitempty"`
	Packages        map[string]PackageInfo  `json:"packages,omitempty"`
	Snapshots       map[string]PackageInfo  `json:"snapshots,omitempty"`
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) ID(name, version string) string {
	return fmt.Sprintf("%s@%s", name, version)
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	var lockFile LockFile
	data, err := textdecode.ReadRaw(ctx, r, 16<<20)
	if err != nil {
		return nil, nil, err
	}
	tree, err := lockyaml.Parse(ctx, data)
	if err != nil {
		return nil, nil, err
	}
	if err := textdecode.ReserveRecordConversion(ctx, tree); err != nil {
		return nil, nil, err
	}
	data, err = json.Marshal(tree)
	if err != nil {
		return nil, nil, err
	}
	if err := budget.From(ctx).Working(int64(len(data))); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(data, &lockFile); err != nil {
		return nil, nil, fmt.Errorf("decode error: %w", err)
	}

	lockVer, err := parseLockfileVersion(lockFile)
	if err != nil {
		return nil, nil, err
	}

	if lockVer == 9 {
		if err := validateV9Tree(tree); err != nil {
			return nil, nil, err
		}
		lockFile, err = joinSnapshots(ctx, lockFile)
		if err != nil {
			return nil, nil, err
		}
	}

	libs, deps, err := p.parse(lockVer, lockFile)
	if err != nil {
		return nil, nil, err
	}

	return libs, deps, nil
}

func (p *Parser) parse(lockVer int, lockFile LockFile) ([]types.Library, []types.Dependency, error) {
	var libs []types.Library
	var deps []types.Dependency

	specOf := map[string]string{}
	for name, spec := range lockFile.Specifiers {
		if s, ok := spec.(string); ok {
			specOf[name] = s
		}
	}
	for name, v := range lockFile.Dependencies {
		if s := nestedSpecifier(v); s != "" && specOf[name] == "" {
			specOf[name] = s
		}
	}

	byName := map[string][]string{}
	type rec struct {
		path, name, version string
		info                PackageInfo
	}
	var recs []rec
	for depPath, info := range lockFile.Packages {
		if info.IsDev {
			continue
		}
		name := info.Name
		version := info.Version
		if name == "" {
			name, version = parsePackage(depPath, lockVer)
		}
		if strings.TrimSpace(name) == "" {
			return nil, nil, fmt.Errorf("malformed_input: pnpm package identity %q", depPath)
		}
		byName[name] = append(byName[name], depPath)
		recs = append(recs, rec{path: depPath, name: name, version: version, info: info})
	}

	directSnapshots := map[string]bool{}
	if lockVer == 9 {
		for _, importer := range lockFile.Importers {
			for _, entries := range []map[string]any{importer.Dependencies, importer.OptionalDependencies} {
				for name, raw := range entries {
					_, version := importerEntry(raw)
					if id := resolvePnpmRef(lockFile, lockVer, name, version, byName); id != "" {
						directSnapshots[id] = true
					}
				}
			}
		}
	}
	for _, item := range recs {
		declared := digest.ParseDeclared(item.info.Resolution.Integrity)
		direct := isImporterDirect(item.name, lockFile)
		if lockVer == 9 {
			direct = directSnapshots[item.path]
		}
		lib := types.Library{
			ID:                item.path,
			Variant:           item.path,
			Source:            item.info.Resolution.Tarball,
			Name:              item.name,
			Version:           item.version,
			Indirect:          !direct && isIndirectLib(item.name, lockFile.Dependencies),
			Verification:      declared.Canonical,
			DeclaredIntegrity: declared.Original,
		}
		if len(lockFile.Importers) == 0 && !lib.Indirect {
			if constraint, ok := specOf[item.name]; ok {
				lib.DeclaredVersion = constraint
			}
		}
		for _, issue := range declared.Issues {
			lib.Diagnostics = append(lib.Diagnostics, model.Diagnostic{Code: "malformed_input", Stage: "pnpm", Reason: issue, Incomplete: true})
		}
		libs = append(libs, lib)

		reqs := append(pnpmReqs(item.info.Dependencies, "", lockFile, lockVer, byName), pnpmReqs(item.info.OptionalDependencies, "optional", lockFile, lockVer, byName)...)
		sort.Slice(reqs, func(i, j int) bool {
			if reqs[i].Target != reqs[j].Target {
				return reqs[i].Target < reqs[j].Target
			}
			return reqs[i].Constraint < reqs[j].Constraint
		})
		var dependsOn []string
		for _, q := range reqs {
			if q.Scope != "optional" && q.Resolved != "" {
				dependsOn = append(dependsOn, q.Resolved)
			}
		}
		sort.Strings(dependsOn)
		if len(dependsOn) > 0 || len(reqs) > 0 {
			deps = append(deps, types.Dependency{ID: item.path, DependsOn: dependsOn, Requirements: reqs})
		}
	}

	impLibs, impDeps := importerDeclarations(lockFile, lockVer, byName)
	libs = append(libs, impLibs...)
	deps = append(deps, impDeps...)

	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))
	return libs, deps, nil
}

func isImporterDirect(name string, lockFile LockFile) bool {
	imp, ok := lockFile.Importers["."]
	if !ok {
		return false
	}
	if _, ok := imp.Dependencies[name]; ok {
		return true
	}
	if _, ok := imp.OptionalDependencies[name]; ok {
		return true
	}
	return false
}

func importerDeclarations(lockFile LockFile, lockVer int, byName map[string][]string) ([]types.Library, []types.Dependency) {
	if len(lockFile.Importers) == 0 {
		return nil, nil
	}
	paths := make([]string, 0, len(lockFile.Importers))
	for p := range lockFile.Importers {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var libs []types.Library
	var deps []types.Dependency
	for _, path := range paths {
		imp := lockFile.Importers[path]
		id := "importer:" + path
		libs = append(libs, types.Library{
			ID:       id,
			Name:     path,
			Evidence: "declared",
			Variant:  path,
		})
		specOf := map[string]string{}
		for name, spec := range imp.Specifiers {
			if s, ok := spec.(string); ok {
				specOf[name] = s
			}
		}
		reqs := append(importerReqs(imp.Dependencies, "", specOf, lockFile, lockVer, byName), importerReqs(imp.OptionalDependencies, "optional", specOf, lockFile, lockVer, byName)...)
		reqs = append(reqs, importerReqs(imp.DevDependencies, "dev", specOf, lockFile, lockVer, byName)...)
		sort.Slice(reqs, func(i, j int) bool {
			if reqs[i].Target != reqs[j].Target {
				return reqs[i].Target < reqs[j].Target
			}
			if reqs[i].Scope != reqs[j].Scope {
				return reqs[i].Scope < reqs[j].Scope
			}
			return reqs[i].Constraint < reqs[j].Constraint
		})
		if len(reqs) == 0 {
			continue
		}
		var dependsOn []string
		for _, q := range reqs {
			if q.Scope != "optional" && q.Scope != "dev" && q.Resolved != "" {
				dependsOn = append(dependsOn, q.Resolved)
			}
		}
		sort.Strings(dependsOn)
		deps = append(deps, types.Dependency{ID: id, DependsOn: dependsOn, Requirements: reqs})
	}
	return libs, deps
}

func importerReqs(entries map[string]any, scope string, specOf map[string]string, lockFile LockFile, lockVer int, byName map[string][]string) []types.Requirement {
	if len(entries) == 0 {
		return nil
	}
	var out []types.Requirement
	for name, raw := range entries {
		spec, ver := importerEntry(raw)
		if spec == "" {
			spec = specOf[name]
		}
		cond := name
		if spec != "" {
			cond = name + "@" + spec
		}
		q := types.Requirement{Target: name, Constraint: spec, Scope: scope, Condition: cond}
		if id := resolvePnpmRef(lockFile, lockVer, name, ver, byName); id != "" {
			q.Resolved = id
		}
		out = append(out, q)
	}
	return out
}

func importerEntry(v any) (spec, ver string) {
	switch t := v.(type) {
	case string:
		return "", t
	case map[string]any:
		spec, _ = t["specifier"].(string)
		ver, _ = t["version"].(string)
		return spec, ver
	default:
		return "", ""
	}
}

func nestedSpecifier(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m["specifier"].(string)
	return s
}

func pnpmReqs(deps map[string]string, scope string, lockFile LockFile, lockVer int, byName map[string][]string) []types.Requirement {
	if len(deps) == 0 {
		return nil
	}
	var out []types.Requirement
	for name, ver := range deps {
		raw := name
		if ver != "" {
			raw = name + "@" + ver
		}
		q := types.Requirement{Target: name, Constraint: ver, Scope: scope, Condition: raw}
		if id := resolvePnpmRef(lockFile, lockVer, name, ver, byName); id != "" {
			q.Resolved = id
		}
		out = append(out, q)
	}
	return out
}

func resolvePnpmRef(lockFile LockFile, lockVer int, name, ver string, byName map[string][]string) string {
	keys := []string{ver}
	if lockVer < 6 {
		keys = append(keys, "/"+name+"/"+ver)
	} else {
		keys = append(keys, "/"+name+"@"+ver)
	}
	keys = append(keys, name+"@"+ver)
	for _, k := range keys {
		if k != "" {
			if _, ok := lockFile.Packages[k]; ok {
				return k
			}
		}
	}
	// v9 references name an exact peer context. A missing key must not bind
	// to a different context merely because it is the only matching version.
	if lockVer == 9 {
		return ""
	}
	var hits []string
	for _, path := range byName[name] {
		info := lockFile.Packages[path]
		n, v := info.Name, info.Version
		if n == "" {
			n, v = parsePackage(path, lockVer)
		}
		if n == name && v == ver {
			hits = append(hits, path)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return ""
}

func parseLockfileVersion(lockFile LockFile) (int, error) {
	var text string
	switch v := lockFile.LockfileVersion.(type) {
	case nil:
		return 0, fmt.Errorf("unsupported_syntax: missing pnpm lock version")
	case string:
		text = strings.TrimSpace(v)
	case float64:
		if v != v || v > 1e6 || v < 0 {
			return 0, fmt.Errorf("unsupported_syntax: pnpm lock version %v", lockFile.LockfileVersion)
		}
		text = strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		text = strconv.Itoa(v)
	case int64:
		text = strconv.FormatInt(v, 10)
	case json.Number:
		text = v.String()
	default:
		return 0, fmt.Errorf("unsupported_syntax: pnpm lock version type %T", v)
	}
	if text == "" || strings.EqualFold(text, "nan") || strings.EqualFold(text, "inf") || strings.EqualFold(text, "+inf") || strings.EqualFold(text, "-inf") {
		return 0, fmt.Errorf("unsupported_syntax: pnpm lock version %v", lockFile.LockfileVersion)
	}
	allowed := map[string]int{
		"5": 5, "5.0": 5, "5.1": 5, "5.2": 5, "5.3": 5, "5.4": 5,
		"6": 6, "6.0": 6,
		"9": 9, "9.0": 9,
	}
	major, ok := allowed[text]
	if !ok {
		return 0, fmt.Errorf("unsupported_syntax: pnpm lock version %v", lockFile.LockfileVersion)
	}
	return major, nil
}

func isIndirectLib(name string, directDeps map[string]interface{}) bool {
	_, ok := directDeps[name]
	return !ok
}

// cf. https://github.com/pnpm/pnpm/blob/ce61f8d3c29eee46cee38d56ced45aea8a439a53/packages/dependency-path/src/index.ts#L112-L163
func parsePackage(depPath string, lockFileVersion int) (string, string) {
	if lockFileVersion == 9 {
		// v9 keys no longer have the leading registry separator.
		return parseDepPath("/"+depPath, "@")
	}
	// The version separator is different between v5 and v6+.
	versionSep := "@"
	if lockFileVersion < 6 {
		versionSep = "/"
	}
	return parseDepPath(depPath, versionSep)
}

var lockedVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func parseDepPath(depPath, versionSep string) (string, string) {
	// Skip registry
	// e.g.
	//    - "registry.npmjs.org/lodash/4.17.10" => "lodash/4.17.10"
	//    - "registry.npmjs.org/@babel/generator/7.21.9" => "@babel/generator/7.21.9"
	//    - "/lodash/4.17.10" => "lodash/4.17.10"
	_, depPath, _ = strings.Cut(depPath, "/")

	// Parse scope
	// e.g.
	//    - v5:  "@babel/generator/7.21.9" => {"babel", "generator/7.21.9"}
	//    - v6+: "@babel/helper-annotate-as-pure@7.18.6" => "{"babel", "helper-annotate-as-pure@7.18.6"}
	var scope string
	if strings.HasPrefix(depPath, "@") {
		scope, depPath, _ = strings.Cut(depPath, "/")
	}

	// Parse package name
	// e.g.
	//    - v5:  "generator/7.21.9" => {"generator", "7.21.9"}
	//    - v6+: "helper-annotate-as-pure@7.18.6" => {"helper-annotate-as-pure", "7.18.6"}
	var name, version string
	name, version, _ = strings.Cut(depPath, versionSep)
	if scope != "" {
		name = fmt.Sprintf("%s/%s", scope, name)
	}
	// Trim peer deps
	// e.g.
	//    - v5:  "7.21.5_@babel+core@7.21.8" => "7.21.5"
	//    - v6+: "7.21.5(@babel/core@7.20.7)" => "7.21.5"
	if idx := strings.IndexAny(version, "_("); idx != -1 {
		version = version[:idx]
	}
	if !lockedVersion.MatchString(version) {
		return "", ""
	}
	return name, version
}

// v9 stores shared metadata in packages and installed peer contexts in snapshots.
// Join before the common projection; references keep the complete snapshot key.
func joinSnapshots(ctx context.Context, lock LockFile) (LockFile, error) {
	if len(lock.Packages) > 0 && len(lock.Snapshots) == 0 {
		return lock, fmt.Errorf("evidence_insufficient: pnpm v9 packages without snapshots")
	}
	need, err := budget.SizeMul(len(lock.Snapshots), 384)
	if err != nil {
		return lock, err
	}
	if err = budget.From(ctx).Working(need + budget.SizeMap); err != nil {
		return lock, err
	}
	joined := make(map[string]PackageInfo, len(lock.Snapshots))
	for key, snapshot := range lock.Snapshots {
		if err := ctx.Err(); err != nil {
			return lock, err
		}
		base, _, _ := strings.Cut(key, "(")
		metadata, ok := lock.Packages[base]
		if !ok {
			return lock, fmt.Errorf("evidence_insufficient: pnpm snapshot %q has no package metadata", key)
		}
		// Shared metadata may be expanded into many peer contexts; reserve each
		// projected integrity/source copy before the common parser allocates it.
		copies, e := budget.SizeMul(len(metadata.Name)+len(metadata.Version)+len(metadata.Resolution.Integrity)+len(metadata.Resolution.Tarball), 24)
		if e != nil {
			return lock, e
		}
		if e = budget.From(ctx).Working(copies); e != nil {
			return lock, e
		}
		if metadata.Name == "" {
			name, version := parsePackage(base, 9)
			if name == "" || version == "" {
				return lock, fmt.Errorf("unsupported_syntax: pnpm v9 identity %q", base)
			}
		} else if metadata.Version == "" {
			return lock, fmt.Errorf("malformed_input: pnpm v9 missing version for %q", base)
		}
		// Validate balanced peer/patch suffixes without recursively interpreting them.
		depth := 0
		for _, c := range key[len(base):] {
			if depth == 0 && c != '(' {
				return lock, fmt.Errorf("malformed_input: pnpm snapshot suffix %q", key)
			}
			switch c {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth < 0 {
				return lock, fmt.Errorf("malformed_input: pnpm snapshot suffix %q", key)
			}
		}
		if depth != 0 {
			return lock, fmt.Errorf("malformed_input: pnpm snapshot suffix %q", key)
		}
		metadata.Dependencies = snapshot.Dependencies
		metadata.OptionalDependencies = snapshot.OptionalDependencies
		metadata.DevDependencies = snapshot.DevDependencies
		joined[key] = metadata
	}
	lock.Packages = joined
	return lock, nil
}

func validateV9Tree(tree map[string]any) error {
	for _, key := range []string{"packages", "snapshots", "importers"} {
		value, present := tree[key]
		if !present {
			continue
		}
		entries, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("malformed_input: pnpm v9 %s must be a mapping", key)
		}
		for name, value := range entries {
			entry, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("malformed_input: pnpm v9 %s entry %q must be a mapping", key, name)
			}
			if key != "importers" {
				continue
			}
			for _, kind := range []string{"dependencies", "devDependencies", "optionalDependencies"} {
				value, present := entry[kind]
				if !present {
					continue
				}
				deps, ok := value.(map[string]any)
				if !ok {
					return fmt.Errorf("malformed_input: pnpm v9 importer dependencies must be a mapping")
				}
				for dep, value := range deps {
					ref, ok := value.(map[string]any)
					if !ok {
						return fmt.Errorf("malformed_input: pnpm v9 importer dependency %q must contain specifier/version", dep)
					}
					for _, field := range []string{"specifier", "version"} {
						text, ok := ref[field].(string)
						if !ok || text == "" {
							return fmt.Errorf("malformed_input: pnpm v9 importer dependency %q missing %s", dep, field)
						}
					}
				}
			}
		}
	}
	return nil
}
