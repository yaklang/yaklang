package pnpm

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	Resolution      PackageResolution `json:"resolution"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
	IsDev           bool              `json:"dev,omitempty"`
	Name            string            `json:"name,omitempty"`
	Version         string            `json:"version,omitempty"`
}

type LockFile struct {
	LockfileVersion any                    `json:"lockfileVersion"`
	Specifiers      map[string]any         `json:"specifiers,omitempty"`
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
	if err := budget.From(types.ContextOf(r)).Working(int64(len(data))); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(data, &lockFile); err != nil {
		return nil, nil, fmt.Errorf("decode error: %w", err)
	}

	lockVer, err := parseLockfileVersion(lockFile)
	if err != nil {
		return nil, nil, err
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
		if strings.TrimSpace(name) == "" {
			return nil, nil, fmt.Errorf("malformed_input: pnpm package identity %q", depPath)
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

		declared := digest.ParseDeclared(info.Resolution.Integrity)
		lib := types.Library{
			ID:      pkgID,
			Variant: depPath, Source: info.Resolution.Tarball,
			Name:              name,
			Version:           version,
			Indirect:          isIndirectLib(name, lockFile.Dependencies),
			Verification:      declared.Canonical,
			DeclaredIntegrity: declared.Original,
		}
		for _, issue := range declared.Issues {
			lib.Diagnostics = append(lib.Diagnostics, model.Diagnostic{Code: "malformed_input", Stage: "pnpm", Reason: issue, Incomplete: true})
		}
		libs = append(libs, lib)

		if len(dependencies) > 0 {
			deps = append(deps, types.Dependency{
				ID:        pkgID,
				DependsOn: dependencies,
			})
		}
	}

	specOf := map[string]string{}
	for name, spec := range lockFile.Specifiers {
		constraint := strings.TrimSpace(fmt.Sprint(spec))
		if s, ok := spec.(string); ok {
			constraint = s
		}
		specOf[name] = constraint
	}
	for i, lib := range libs {
		if constraint, ok := specOf[lib.Name]; ok {
			libs[i].DeclaredVersion = constraint
		}
	}

	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))
	return libs, deps, nil
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
		log.Printf("Unknown type for the lock file version: %s", lockFile.LockfileVersion)
		return 0, fmt.Errorf("unsupported_syntax: pnpm lock version type %T", v)
	}
	if text == "" || strings.EqualFold(text, "nan") || strings.EqualFold(text, "inf") || strings.EqualFold(text, "+inf") || strings.EqualFold(text, "-inf") {
		return 0, fmt.Errorf("unsupported_syntax: pnpm lock version %v", lockFile.LockfileVersion)
	}
	allowed := map[string]int{
		"5": 5, "5.0": 5, "5.1": 5, "5.2": 5, "5.3": 5, "5.4": 5,
		"6": 6, "6.0": 6,
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
