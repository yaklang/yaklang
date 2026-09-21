package composer

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	maps "github.com/yaklang/yaklang/common/sca/internal/collection"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/utils"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type lockFile struct {
	Packages []packageInfo `json:"packages"`
}
type packageInfo struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Require map[string]string `json:"require"`
	License []string          `json:"license"`
	Source  struct {
		URL       string `json:"url"`
		Reference string `json:"reference"`
	} `json:"source"`
	StartLine int
	EndLine   int
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	var lockFile lockFile
	input, err := textdecode.ReadRaw(ctx, r, 16<<20)
	if err != nil {
		return nil, nil, fmt.Errorf("read error: %w", err)
	}
	nodes, err := jsonrecord.Decode(ctx, input, &lockFile)
	if err != nil {
		return nil, nil, fmt.Errorf("decode error: %w", err)
	}
	for i, n := range nodes.Get("packages").Elements() {
		lockFile.Packages[i].StartLine, lockFile.Packages[i].EndLine = n.Lines()
	}

	libs := map[string]types.Library{}
	foundDeps := map[string][]string{}
	requirements := map[string][]types.Requirement{}
	for _, pkg := range lockFile.Packages {
		lib := types.Library{
			ID:       utils.PackageID(pkg.Name, pkg.Version),
			Name:     pkg.Name,
			Version:  pkg.Version,
			Indirect: false, // composer.lock file doesn't have info about Direct/Indirect deps. Will think that all dependencies are Direct
			License:  strings.Join(pkg.License, ", "),
			Locations: []types.Location{
				{
					StartLine: pkg.StartLine,
					EndLine:   pkg.EndLine,
				},
			},
		}
		if pkg.Source.URL != "" {
			lib.Source = pkg.Source.URL + "#" + pkg.Source.Reference
		}
		if _, exists := libs[lib.Name]; exists {
			return nil, nil, fmt.Errorf("malformed_input: duplicate Composer package %s", lib.Name)
		}
		libs[lib.Name] = lib

		var dependsOn []string
		for depName, constraint := range pkg.Require {
			requirements[lib.ID] = append(requirements[lib.ID], types.Requirement{Target: depName, Constraint: constraint})
			// Require field includes required php version, skip this
			// Also skip PHP extensions
			if depName == "php" || strings.HasPrefix(depName, "ext-") {
				continue
			}
			dependsOn = append(dependsOn, depName) // field uses range of versions, so later we will fill in the versions from the libraries
		}
		if len(requirements[lib.ID]) > 0 {
			foundDeps[lib.ID] = dependsOn
		}
	}

	// fill deps versions
	var deps []types.Dependency
	for libID, depsOn := range foundDeps {
		var dependsOn []string
		for _, depName := range depsOn {
			if lib, ok := libs[depName]; ok {
				dependsOn = append(dependsOn, lib.ID)
				continue
			}
		}
		sort.Strings(dependsOn)
		qs := requirements[libID]
		for i := range qs {
			if target, ok := libs[qs[i].Target]; ok {
				qs[i].Resolved = target.ID
			}
		}
		sort.Slice(qs, func(i, j int) bool { return qs[i].Target < qs[j].Target })
		deps = append(deps, types.Dependency{
			ID:           libID,
			DependsOn:    dependsOn,
			Requirements: qs,
		})
	}

	libSlice := maps.Values(libs)
	sort.Sort(types.Libraries(libSlice))
	sort.Sort(types.Dependencies(deps))

	return libSlice, deps, nil
}
