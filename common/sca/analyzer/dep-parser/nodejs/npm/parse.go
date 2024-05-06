package npm

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/samber/lo"

	"github.com/liamg/jfather"
	"github.com/yaklang/yaklang/common/log"
	outils "github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/utils"
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

func (p *Parser) Parse(r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var lockFile LockFile
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, outils.Errorf("read error: %w", err)
	}
	if err := jfather.Unmarshal(input, &lockFile); err != nil {
		return nil, nil, outils.Errorf("decode error: %w", err)
	}

	var libs []types.Library
	var deps []types.Dependency
	if lockFile.LockfileVersion == 1 {
		libs, deps = p.parseV1(lockFile.Dependencies, map[string]string{})
	} else {
		libs, deps = p.parseV2(lockFile.Packages)
	}

	return utils.UniqueLibraries(libs), uniqueDeps(deps), nil
}

func (p *Parser) parseV2(packages map[string]Package) ([]types.Library, []types.Dependency) {
	libs := make(map[string]types.Library, len(packages)-1)
	var deps []types.Dependency

	// Resolve links first
	// https://docs.npmjs.com/cli/v9/configuring-npm/package-lock-json#packages
	resolveLinks(packages)

	directDeps := map[string]struct{}{}
	for name, version := range lo.Assign(packages[""].Dependencies, packages[""].OptionalDependencies, packages[""].DevDependencies) {
		pkgPath := joinPaths(nodeModulesDir, name)
		if _, ok := packages[pkgPath]; !ok {
			log.Debugf("Unable to find the direct dependency: '%s@%s'", name, version)
			continue
		}
		// Store the package paths of direct dependencies
		// e.g. node_modules/body-parser
		directDeps[pkgPath] = struct{}{}
	}

	for pkgPath, pkg := range packages {
		if !strings.HasPrefix(pkgPath, "node_modules") {
			continue
		}

		// pkg.Name exists when package name != folder name
		pkgName := pkg.Name
		if pkgName == "" {
			pkgName = pkgNameFromPath(pkgPath)
		}

		pkgID := utils.PackageID(pkgName, pkg.Version)
		location := types.Location{
			StartLine: pkg.StartLine,
			EndLine:   pkg.EndLine,
		}

		// There are cases when similar libraries use same dependencies
		// we need to add location for each these dependencies
		if savedLib, ok := libs[pkgID]; ok {
			savedLib.Locations = append(savedLib.Locations, location)
			sort.Sort(savedLib.Locations)
			libs[pkgID] = savedLib
			continue
		}

		lib := types.Library{
			ID:       pkgID,
			Name:     pkgName,
			Version:  pkg.Version,
			Indirect: isIndirectLib(pkgPath, directDeps),
			Dev:      pkg.Dev,
			ExternalReferences: []types.ExternalRef{
				{
					Type: types.RefOther,
					URL:  pkg.Resolved,
				},
			},
			Locations: []types.Location{location},
		}
		libs[pkgID] = lib

		// npm builds graph using optional deps. e.g.:
		// └─┬ watchpack@1.7.5
		// ├─┬ chokidar@3.5.3 - optional dependency
		// │ └── glob-parent@5.1.
		dependencies := lo.Assign(pkg.Dependencies, pkg.OptionalDependencies)
		dependsOn := make([]string, 0, len(dependencies))
		for depName, depVersion := range dependencies {
			depID, err := findDependsOn(pkgPath, depName, packages)
			if err != nil {
				log.Warnf("Cannot resolve the version: '%s@%s'", depName, depVersion)
				continue
			}
			dependsOn = append(dependsOn, depID)
		}

		if len(dependsOn) > 0 {
			deps = append(deps, types.Dependency{
				ID:        lib.ID,
				DependsOn: dependsOn,
			})
		}

	}

	return maps.Values(libs), deps
}

// for local package npm uses links. e.g.:
// function/func1 -> target of package
// node_modules/func1 -> link to target
// see `package-lock_v3_with_workspace.json` to better understanding
func resolveLinks(packages map[string]Package) {
	links := lo.PickBy(packages, func(_ string, pkg Package) bool {
		return pkg.Link
	})
	// Early return
	if len(links) == 0 {
		return
	}

	rootPkg := packages[""]
	if rootPkg.Dependencies == nil {
		rootPkg.Dependencies = make(map[string]string)
	}

	workspaces := rootPkg.Workspaces
	for pkgPath, pkg := range packages {
		for linkPath, link := range links {
			if !strings.HasPrefix(pkgPath, link.Resolved) {
				continue
			}
			// The target doesn't have the "resolved" field, so we need to copy it from the link.
			if pkg.Resolved == "" {
				pkg.Resolved = link.Resolved
			}

			// Resolve the link package so all packages are located under "node_modules".
			resolvedPath := strings.ReplaceAll(pkgPath, link.Resolved, linkPath)
			packages[resolvedPath] = pkg

			// Delete the target package
			delete(packages, pkgPath)

			if isWorkspace(pkgPath, workspaces) {
				rootPkg.Dependencies[pkgNameFromPath(linkPath)] = pkg.Version
			}
			break
		}
	}
	packages[""] = rootPkg
}

func isWorkspace(pkgPath string, workspaces []string) bool {
	for _, workspace := range workspaces {
		if match, err := path.Match(workspace, pkgPath); err != nil {
			log.Debugf("unable to parse workspace %q for %s", workspace, pkgPath)
		} else if match {
			return true
		}
	}
	return false
}

func findDependsOn(pkgPath, depName string, packages map[string]Package) (string, error) {
	depPath := joinPaths(pkgPath, nodeModulesDir)
	paths := strings.Split(depPath, "/")
	// Try to resolve the version with the nearest directory
	// e.g. for pkgPath == `node_modules/body-parser/node_modules/debug`, depName == `ms`:
	//    - "node_modules/body-parser/node_modules/debug/node_modules/ms"
	//    - "node_modules/body-parser/node_modules/ms"
	//    - "node_modules/ms"
	for i := len(paths) - 1; i >= 0; i-- {
		if paths[i] != nodeModulesDir {
			continue
		}
		path := joinPaths(paths[:i+1]...)
		path = joinPaths(path, depName)

		if dep, ok := packages[path]; ok {
			return utils.PackageID(depName, dep.Version), nil
		}
	}

	// It should not reach here.
	return "", outils.Errorf("can't find dependsOn for %s", depName)
}

func (p *Parser) parseV1(dependencies map[string]Dependency, versions map[string]string) ([]types.Library, []types.Dependency) {
	// Update package name and version mapping.
	for pkgName, dep := range dependencies {
		// Overwrite the existing package version so that the nested version can take precedence.
		versions[pkgName] = dep.Version
	}

	var libs []types.Library
	var deps []types.Dependency
	for pkgName, dependency := range dependencies {
		lib := types.Library{
			ID:       utils.PackageID(pkgName, dependency.Version),
			Name:     pkgName,
			Version:  dependency.Version,
			Dev:      dependency.Dev,
			Indirect: true, // lockfile v1 schema doesn't have information about Direct dependencies
			ExternalReferences: []types.ExternalRef{
				{
					Type: types.RefOther,
					URL:  dependency.Resolved,
				},
			},
			Locations: []types.Location{
				{
					StartLine: dependency.StartLine,
					EndLine:   dependency.EndLine,
				},
			},
		}
		libs = append(libs, lib)

		dependsOn := make([]string, 0, len(dependency.Requires))
		for libName, requiredVer := range dependency.Requires {
			// Try to resolve the version with nested dependencies first
			if resolvedDep, ok := dependency.Dependencies[libName]; ok {
				libID := utils.PackageID(libName, resolvedDep.Version)
				dependsOn = append(dependsOn, libID)
				continue
			}

			// Try to resolve the version with the higher level dependencies
			if ver, ok := versions[libName]; ok {
				dependsOn = append(dependsOn, utils.PackageID(libName, ver))
				continue
			}

			// It should not reach here.
			log.Warnf("Cannot resolve the version: %s@%s", libName, requiredVer)
		}

		if len(dependsOn) > 0 {
			deps = append(deps, types.Dependency{
				ID:        utils.PackageID(lib.Name, lib.Version),
				DependsOn: dependsOn,
			})
		}

		if dependency.Dependencies != nil {
			// Recursion
			childLibs, childDeps := p.parseV1(dependency.Dependencies, maps.Clone(versions))
			libs = append(libs, childLibs...)
			deps = append(deps, childDeps...)
		}
	}

	return libs, deps
}

func uniqueDeps(deps []types.Dependency) []types.Dependency {
	var uniqDeps []types.Dependency
	unique := make(map[string]struct{})

	for _, dep := range deps {
		sort.Strings(dep.DependsOn)
		depKey := fmt.Sprintf("%s:%s", dep.ID, strings.Join(dep.DependsOn, ","))
		if _, ok := unique[depKey]; !ok {
			unique[depKey] = struct{}{}
			uniqDeps = append(uniqDeps, dep)
		}
	}

	sort.Sort(types.Dependencies(uniqDeps))
	return uniqDeps
}

func isIndirectLib(pkgPath string, directDeps map[string]struct{}) bool {
	// A project can contain 2 different versions of the same dependency.
	// e.g. `node_modules/string-width/node_modules/strip-ansi` and `node_modules/string-ansi`
	// direct dependencies always have root path (`node_modules/<lib_name>`)
	if _, ok := directDeps[pkgPath]; ok {
		return false
	}
	return true
}

func pkgNameFromPath(path string) string {
	// lock file contains path to dependency in `node_modules`. e.g.:
	// node_modules/string-width
	// node_modules/string-width/node_modules/strip-ansi
	// we renamed to `node_modules` directory prefixes `workspace` when resolving Links
	// node_modules/function1
	// node_modules/nested_func/node_modules/debug
	if index := strings.LastIndex(path, nodeModulesDir); index != -1 {
		return path[index+len(nodeModulesDir)+1:]
	}
	log.Warnf("npm %q package path doesn't have `node_modules` prefix", path)
	return path
}

func joinPaths(paths ...string) string {
	return strings.Join(paths, "/")
}

// UnmarshalJSONWithMetadata needed to detect start and end lines of deps for v1
func (t *Dependency) UnmarshalJSONWithMetadata(node jfather.Node) error {
	if err := node.Decode(&t); err != nil {
		return err
	}
	// Decode func will overwrite line numbers if we save them first
	t.StartLine = node.Range().Start.Line
	t.EndLine = node.Range().End.Line
	return nil
}

// UnmarshalJSONWithMetadata needed to detect start and end lines of deps for v2 or newer
func (t *Package) UnmarshalJSONWithMetadata(node jfather.Node) error {
	if err := node.Decode(&t); err != nil {
		return err
	}
	// Decode func will overwrite line numbers if we save them first
	t.StartLine = node.Range().Start.Line
	t.EndLine = node.Range().End.Line
	return nil
}
