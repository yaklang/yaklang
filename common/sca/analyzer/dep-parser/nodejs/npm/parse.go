package npm

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	lo "github.com/yaklang/yaklang/common/sca/internal/collection"

	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	"io/fs"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const nodeModulesDir = "node_modules"

type LockFile struct {
	Dependencies    map[string]Dependency `json:"dependencies"`
	Packages        map[string]Package    `json:"packages"`
	LockfileVersion int                   `json:"lockfileVersion"`
}
type Dependency struct {
	Version      string                `json:"version"`
	Dev          bool                  `json:"dev"`
	Dependencies map[string]Dependency `json:"dependencies"`
	Requires     map[string]string     `json:"requires"`
	Resolved     string                `json:"resolved"`
	StartLine    int
	EndLine      int
}

type Package struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	Resolved             string            `json:"resolved"`
	Dev                  bool              `json:"dev"`
	Link                 bool              `json:"link"`
	Workspaces           []string          `json:"workspaces"`
	StartLine            int
	EndLine              int
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var lockFile LockFile
	input, err := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read error: %w", err)
	}
	nodes, err := jsonrecord.Decode(types.ContextOf(r), input, &lockFile)
	if err != nil {
		return nil, nil, fmt.Errorf("decode error: %w", err)
	}
	for k, v := range lockFile.Packages {
		v.StartLine, v.EndLine = nodes.Get("packages", k).Lines()
		lockFile.Packages[k] = v
	}
	setDependencyLocations(lockFile.Dependencies, nodes.Get("dependencies"))

	if lockFile.LockfileVersion < 1 || lockFile.LockfileVersion > 3 {
		return nil, nil, fmt.Errorf("unsupported_syntax: npm lock version %d", lockFile.LockfileVersion)
	}
	if lockFile.LockfileVersion == 1 {
		lockFile.Packages = map[string]Package{}
		flattenV1(lockFile.Packages, "", lockFile.Dependencies)
	}
	for key, record := range lockFile.Packages {
		if !validPath(key) || record.Link && !validPath(record.Resolved) {
			return nil, nil, fmt.Errorf("invalid_path: npm record %q", key)
		}
		if record.Link && followLink(key, lockFile.Packages) == "" {
			return nil, nil, fmt.Errorf("evidence_insufficient: cyclic or missing npm link %q", key)
		}
	}
	libs, deps := p.parseV2(lockFile.Packages)
	return libs, deps, nil
}

// parseV2 indexes physical installation paths. Equal component versions at
// different paths remain distinct observations with their own outgoing edges.
func (p *Parser) parseV2(packages map[string]Package) ([]types.Library, []types.Dependency) {
	paths := make([]string, 0, len(packages))
	for key := range packages {
		if key != "" {
			paths = append(paths, key)
		}
	}
	sort.Strings(paths)
	direct := map[string]bool{}
	for name := range lo.Assign(packages[""].Dependencies, packages[""].OptionalDependencies, packages[""].DevDependencies) {
		if target := findInstalled("", name, packages); target != "" {
			direct[target] = true
		}
	}
	var libs []types.Library
	var deps []types.Dependency
	for _, key := range paths {
		record := packages[key]
		if record.Link {
			continue
		} // the target record is the workspace component
		name := record.Name
		if name == "" {
			name = pkgNameFromPath(key)
		}
		libs = append(libs, types.Library{ID: key, Name: name, Version: record.Version, Source: record.Resolved, Dev: record.Dev, Indirect: !direct[key], Locations: []types.Location{{StartLine: record.StartLine, EndLine: record.EndLine}}})
		d := types.Dependency{ID: key}
		for _, scope := range []struct {
			name   string
			values map[string]string
		}{{"runtime", record.Dependencies}, {"optional", record.OptionalDependencies}} {
			names := make([]string, 0, len(scope.values))
			for name := range scope.values {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				target := findInstalled(key, name, packages)
				d.Requirements = append(d.Requirements, types.Requirement{Target: name, Constraint: scope.values[name], Scope: scope.name, Resolved: target})
			}
		}
		deps = append(deps, d)
	}
	return libs, deps
}
func followLink(key string, packages map[string]Package) string {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		p, ok := packages[key]
		if !ok || seen[key] {
			return ""
		}
		seen[key] = true
		if !p.Link {
			return key
		}
		key = p.Resolved
		if !validPath(key) {
			return ""
		}
	}
	return ""
}
func findInstalled(from, name string, packages map[string]Package) string {
	if name == "" || strings.ContainsAny(name, `\:`) || strings.Contains(name, "..") {
		return ""
	}
	// Only complete node_modules boundaries are candidates; no prefix matching.
	dir := from
	for depth := 0; depth < 128; depth++ {
		candidate := path.Join(dir, "node_modules", name)
		if _, ok := packages[candidate]; ok {
			return followLink(candidate, packages)
		}
		if dir == "" || dir == "." {
			break
		}
		dir = path.Dir(dir)
		if path.Base(dir) == "node_modules" {
			dir = path.Dir(dir)
		}
	}
	return ""
}
func validPath(name string) bool {
	return name == "" || fs.ValidPath(name) && !strings.ContainsAny(name, `\:`)
}
func pkgNameFromPath(name string) string {
	const marker = "node_modules/"
	if i := strings.LastIndex(name, marker); i >= 0 {
		return name[i+len(marker):]
	}
	return path.Base(name)
}
func flattenV1(all map[string]Package, base string, dependencies map[string]Dependency) {
	for name, d := range dependencies {
		key := path.Join(base, "node_modules", name)
		all[key] = Package{Name: name, Version: d.Version, Dependencies: d.Requires, Dev: d.Dev, Resolved: d.Resolved, StartLine: d.StartLine, EndLine: d.EndLine}
		flattenV1(all, key, d.Dependencies)
	}
}

func setDependencyLocations(deps map[string]Dependency, node *jsonrecord.Node) {
	for k, v := range deps {
		child := node.Get(k)
		v.StartLine, v.EndLine = child.Lines()
		setDependencyLocations(v.Dependencies, child.Get("dependencies"))
		deps[k] = v
	}
}
