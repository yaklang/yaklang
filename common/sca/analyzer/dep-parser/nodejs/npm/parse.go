package npm

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	lo "github.com/yaklang/yaklang/common/sca/internal/collection"
	"github.com/yaklang/yaklang/common/sca/internal/digest"
	"github.com/yaklang/yaklang/common/sca/model"
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
	Integrity    string                `json:"integrity"`
	StartLine    int
	EndLine      int
}

type Package struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	Resolved             string            `json:"resolved"`
	Integrity            string            `json:"integrity"`
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
		paths = append(paths, key)
	}
	sort.Strings(paths)
	direct := map[string]bool{}
	root := packages[""]
	for name := range lo.Assign(root.Dependencies, root.OptionalDependencies, root.DevDependencies, root.PeerDependencies) {
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
		id := key
		if id == "" {
			id = "."
		}
		name := record.Name
		if name == "" && key != "" {
			name = pkgNameFromPath(key)
		}
		hasDecl := len(record.Dependencies)+len(record.OptionalDependencies)+len(record.DevDependencies)+len(record.PeerDependencies) > 0
		if key == "" && name == "" && !hasDecl {
			continue
		}
		declared := digest.ParseDeclared(record.Integrity)
		lib := types.Library{
			ID: id, Name: name, Version: record.Version, Source: record.Resolved,
			Dev: record.Dev, Indirect: key != "" && !direct[key],
			Verification:      declared.Canonical,
			DeclaredIntegrity: declared.Original,
			Locations:         []types.Location{{StartLine: record.StartLine, EndLine: record.EndLine}},
		}
		if key == "" {
			lib.Evidence = "declared"
			lib.Indirect = false
		}
		for _, issue := range declared.Issues {
			lib.Diagnostics = append(lib.Diagnostics, model.Diagnostic{Code: "malformed_input", Stage: "npm", Reason: issue, Incomplete: true})
		}
		libs = append(libs, lib)
		d := types.Dependency{ID: id}
		for _, scope := range []struct {
			name   string
			values map[string]string
		}{{"runtime", record.Dependencies}, {"optional", record.OptionalDependencies}, {"dev", record.DevDependencies}, {"peer", record.PeerDependencies}} {
			names := make([]string, 0, len(scope.values))
			for n := range scope.values {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				target := findInstalled(key, n, packages)
				d.Requirements = append(d.Requirements, types.Requirement{Target: n, Constraint: scope.values[n], Scope: scope.name, Resolved: target})
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
		all[key] = Package{Name: name, Version: d.Version, Dependencies: d.Requires, Dev: d.Dev, Resolved: d.Resolved, Integrity: d.Integrity, StartLine: d.StartLine, EndLine: d.EndLine}
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
