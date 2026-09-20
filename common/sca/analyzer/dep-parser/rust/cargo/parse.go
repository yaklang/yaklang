package cargo

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/locktoml"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type cargoPkg struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Source       string   `json:"source,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}
type Lockfile struct {
	Packages []cargoPkg `json:"package"`
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var lockfile Lockfile
	spans, err := locktoml.DecodeRecords(types.ContextOf(r), r, &lockfile, "package")
	if err != nil {
		return nil, nil, fmt.Errorf("decode error: %w", err)
	}

	// We need to get version for unique dependencies for lockfile v3 from lockfile.Packages
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
		pkgID := nativeID(pkg)
		lib := types.Library{
			ID:      pkgID,
			Name:    pkg.Name,
			Source:  pkg.Source,
			Version: pkg.Version,
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
		switch len(f) {
		case 1:
			matches = index.name[f[0]]
		case 2:
			matches = index.version[[2]string{f[0], f[1]}]
		case 3:
			matches = index.full[[3]string{f[0], f[1], strings.TrimSuffix(strings.TrimPrefix(f[2], "("), ")")}]
		}

		if len(matches) == 1 {
			dep.DependsOn = append(dep.DependsOn, nativeID(matches[0]))
		} else {
			dep.DependsOn = append(dep.DependsOn, "unresolved-cargo:"+raw)
		}
	}
	if len(dep.DependsOn) == 0 {
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
