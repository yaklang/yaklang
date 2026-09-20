package pnpm

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"encoding/json"
	"github.com/yaklang/yaklang/common/sca/core/lockyaml"
	"io"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"log"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type PackageResolution struct {
	Tarball string `json:"tarball,omitempty"`
}

type PackageInfo struct {
	Resolution      PackageResolution `json:"resolution"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
	IsDev           bool              `json:"dev,omitempty"`
	Name            string            `json:"name,omitempty"`
	Version         string            `json:"version,omitempty"`
}

type LockFile struct {
	LockfileVersion any                    `json:"lockfileVersion"`
	Dependencies    map[string]any         `json:"dependencies,omitempty"`
	DevDependencies map[string]any         `json:"devDependencies,omitempty"`
	Packages        map[string]PackageInfo `json:"packages,omitempty"`
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) ID(name, version string) string {
	return fmt.Sprintf("%s@%s", name, version)
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var lockFile LockFile
	data, err := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if err != nil {
		return nil, nil, err
	}
	tree, err := lockyaml.Parse(types.ContextOf(r), data)
	if err != nil {
		return nil, nil, err
	}
	data, err = json.Marshal(tree)
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(data, &lockFile); err != nil {
		return nil, nil, fmt.Errorf("decode error: %w", err)
	}

	lockVer := parseLockfileVersion(lockFile)
	if lockVer < 5 || lockVer >= 7 {
		return nil, nil, fmt.Errorf("unsupported_syntax: pnpm lock version %v", lockFile.LockfileVersion)
	}

	libs, deps := p.parse(lockVer, lockFile)

	return libs, deps, nil
}

func (p *Parser) parse(lockVer float64, lockFile LockFile) ([]types.Library, []types.Dependency) {
	var libs []types.Library
	var deps []types.Dependency

	// Dependency path is a path to a dependency with a specific set of resolved subdependencies.
	// cf. https://github.com/pnpm/spec/blob/ad27a225f81d9215becadfa540ef05fa4ad6dd60/dependency-path.md
	for depPath, info := range lockFile.Packages {
		if info.IsDev {
			continue
		}

		// Dependency name may be present in dependencyPath or Name field. Same for Version.
		// e.g. packages installed from local directory or tarball
		// cf. https://github.com/pnpm/spec/blob/274ff02de23376ad59773a9f25ecfedd03a41f64/lockfile/6.0.md#packagesdependencypathname
		name := info.Name
		version := info.Version

		if name == "" {
			name, version = parsePackage(depPath, lockVer)
		}
		pkgID := depPath

		dependencies := make([]string, 0, len(info.Dependencies))
		for depName, depVer := range info.Dependencies {
			ref := depVer
			if _, ok := lockFile.Packages[ref]; !ok {
				if lockVer < 6 {
					ref = "/" + depName + "/" + depVer
				} else {
					ref = "/" + depName + "@" + depVer
				}
			}
			if _, ok := lockFile.Packages[ref]; !ok {
				ref = p.ID(depName, depVer)
			}
			dependencies = append(dependencies, ref)
		}

		libs = append(libs, types.Library{
			ID:      pkgID,
			Variant: depPath, Source: info.Resolution.Tarball,
			Name:     name,
			Version:  version,
			Indirect: isIndirectLib(name, lockFile.Dependencies),
		})

		if len(dependencies) > 0 {
			deps = append(deps, types.Dependency{
				ID:        pkgID,
				DependsOn: dependencies,
			})
		}
	}

	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))
	return libs, deps
}

func parseLockfileVersion(lockFile LockFile) float64 {
	switch v := lockFile.LockfileVersion.(type) {
	// v5
	case float64:
		return v
	// v6+
	case string:
		if lockVer, err := strconv.ParseFloat(v, 64); err != nil {
			log.Printf("Unable to convert the lock file version to float: %s", err)
			return -1
		} else {
			return lockVer
		}
	default:
		log.Printf("Unknown type for the lock file version: %s", lockFile.LockfileVersion)
		return -1
	}
}

func isIndirectLib(name string, directDeps map[string]interface{}) bool {
	_, ok := directDeps[name]
	return !ok
}

// cf. https://github.com/pnpm/pnpm/blob/ce61f8d3c29eee46cee38d56ced45aea8a439a53/packages/dependency-path/src/index.ts#L112-L163
func parsePackage(depPath string, lockFileVersion float64) (string, string) {
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
