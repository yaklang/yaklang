package poetry

import (
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/utils"
	outils "github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Lockfile struct {
	Packages []struct {
		Category       string                 `toml:"category"`
		Description    string                 `toml:"description"`
		Marker         string                 `toml:"marker,omitempty"`
		Name           string                 `toml:"name"`
		Optional       bool                   `toml:"optional"`
		PythonVersions string                 `toml:"python-versions"`
		Version        string                 `toml:"version"`
		Dependencies   map[string]interface{} `toml:"dependencies"`
		Metadata       interface{}
	} `toml:"package"`
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var lockfile Lockfile
	if _, err := toml.NewDecoder(r).Decode(&lockfile); err != nil {
		return nil, nil, outils.Errorf("failed to decode poetry.lock: %w", err)
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
		libs = append(libs, types.Library{
			ID:      pkgID,
			Name:    pkg.Name,
			Version: pkg.Version,
		})

		dependsOn := parseDependencies(pkg.Dependencies, libVersions)
		if len(dependsOn) != 0 {
			deps = append(deps, types.Dependency{
				ID:        pkgID,
				DependsOn: dependsOn,
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
			log.Debugf("failed to parse poetry dependency: %s", err)
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
		return "", outils.Errorf("no version found for %q", name)
	}

	for _, ver := range vers {
		return utils.PackageID(name, ver), nil
	}
	return "", outils.Errorf("no matched version found for %q", name)
}

func normalizePkgName(name string) string {
	// The package names don't use `_`, `.` or upper case, but dependency names can contain them.
	// We need to normalize those names.
	name = strings.ToLower(name)              // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L819
	name = strings.ReplaceAll(name, "_", "-") // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L50
	name = strings.ReplaceAll(name, ".", "-") // e.g. https://github.com/python-poetry/poetry/blob/c8945eb110aeda611cc6721565d7ad0c657d453a/poetry.lock#L816
	return name
}
