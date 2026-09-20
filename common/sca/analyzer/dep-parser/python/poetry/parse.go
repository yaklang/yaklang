package poetry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/locktoml"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/utils"
	"github.com/yaklang/yaklang/common/sca/internal/digest"
	"github.com/yaklang/yaklang/common/sca/model"
	"log"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Lockfile struct {
	Packages []struct {
		Category       string                 `json:"category"`
		Description    string                 `json:"description"`
		Marker         string                 `json:"marker,omitempty"`
		Name           string                 `json:"name"`
		Optional       bool                   `json:"optional"`
		PythonVersions string                 `json:"python-versions"`
		Version        string                 `json:"version"`
		Dependencies   map[string]interface{} `json:"dependencies"`
		Files          []struct {
			File string `json:"file"`
			Hash string `json:"hash"`
		} `json:"files"`
		Metadata interface{}
	} `json:"package"`
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var lockfile Lockfile
	if err := locktoml.Decode(types.ContextOf(r), r, &lockfile); err != nil {
		return nil, nil, fmt.Errorf("failed to decode poetry.lock: %w", err)
	}

	// Keep all installed versions
	libVersions := parseVersions(lockfile)

	var libs []types.Library
	var deps []types.Dependency
	for _, pkg := range lockfile.Packages {
		if pkg.Category == "dev" {
			continue
		}

		pkgID := utils.PackageID(pkg.Name, pkg.Version)
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
			Verification:      declared.Canonical,
			DeclaredIntegrity: declared.Original,
		}
		for _, issue := range declared.Issues {
			lib.Diagnostics = append(lib.Diagnostics, model.Diagnostic{Code: "malformed_input", Stage: "poetry", Reason: issue, Incomplete: true})
		}
		libs = append(libs, lib)

		dependsOn := parseDependencies(pkg.Dependencies, libVersions)
		reqs := []types.Requirement{}
		for name, raw := range pkg.Dependencies {
			q := types.Requirement{Target: normalizePkgName(name)}
			switch v := raw.(type) {
			case string:
				q.Constraint = v
			case map[string]any:
				if text, ok := v["version"].(string); ok {
					q.Constraint = text
				}
				if text, ok := v["markers"].(string); ok {
					q.Condition = text
				}
			default:
				q.Constraint = fmt.Sprint(v)
			}
			reqs = append(reqs, q)
		}
		sort.Slice(reqs, func(i, j int) bool { return reqs[i].Target < reqs[j].Target })
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

// parseVersions stores all installed versions of libraries for use in dependsOn
// as the dependencies of libraries use version range.
func parseVersions(lockfile Lockfile) map[string][]string {
	libVersions := map[string][]string{}
	for _, pkg := range lockfile.Packages {
		if pkg.Category == "dev" {
			continue
		}
		if vers, ok := libVersions[pkg.Name]; ok {
			libVersions[pkg.Name] = append(vers, pkg.Version)
		} else {
			libVersions[pkg.Name] = []string{pkg.Version}
		}
	}
	return libVersions
}

func parseDependencies(deps map[string]any, libVersions map[string][]string) []string {
	var dependsOn []string
	for name, versRange := range deps {
		if dep, err := parseDependency(name, versRange, libVersions); err != nil {
			log.Printf("failed to parse poetry dependency: %s", err)
		} else if dep != "" {
			dependsOn = append(dependsOn, dep)
		}
	}
	sort.Slice(dependsOn, func(i, j int) bool {
		return dependsOn[i] < dependsOn[j]
	})
	return dependsOn
}

func parseDependency(name string, versRange any, libVersions map[string][]string) (string, error) {
	name = normalizePkgName(name)
	vers, ok := libVersions[name]
	if !ok {
		return "", fmt.Errorf("no version found for %q", name)
	}

	if len(vers) > 1 {
		return "", fmt.Errorf("ambiguous locked versions for %q", name)
	}
	for _, ver := range vers {
		return utils.PackageID(name, ver), nil
	}
	return "", fmt.Errorf("no matched version found for %q", name)
}

func normalizePkgName(name string) string {
	// The package names don't use `_`, `.` or upper case, but dependency names can contain them.
	// We need to normalize those names.
	name = strings.ToLower(name)              // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L819
	name = strings.ReplaceAll(name, "_", "-") // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L50
	name = strings.ReplaceAll(name, ".", "-") // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L816
	return name
}
