package analyzer

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/nodejs/npm"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	godeptypes "github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	lo "github.com/yaklang/yaklang/common/sca/internal/collection"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const (
	TypNodeNpm TypAnalyzer = "npm-lang"

	packageJson     = "package.json"
	packageLockJson = "package-lock.json"

	statusNpmJson     = 1
	statusNpmLockJson = 2
)

func init() {
	RegisterAnalyzer(TypNodeNpm, NewNodeNpmAnalyzer())
}

type npmAnalyzer struct{}

func NewNodeNpmAnalyzer() *npmAnalyzer {
	return &npmAnalyzer{}
}

func (a npmAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	var p godeptypes.Parser
	switch fi.MatchStatus {
	case statusNpmJson:
		p = newNpmParse()

	case statusNpmLockJson:
		p = npm.NewParser()
	default:
		return nil, nil
	}
	pkgs, err := ParseLanguageConfiguration(fi, p)
	if err != nil {
		return nil, err
	}
	lo.ForEach(pkgs, func(pkg *dxtypes.Package, _ int) {
		pkg.Version = handlerSemverVersionRange(strings.TrimSpace(pkg.Version))
	})
	return pkgs, nil
}

func (a npmAnalyzer) Match(info MatchInfo) int {
	if info.FileInfo.Name() == packageJson {
		return statusNpmJson
	}
	if info.FileInfo.Name() == packageLockJson {
		return statusNpmLockJson
	}
	return 0
}

type packageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	License              interface{}       `json:"license"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	Workspaces           []string          `json:"workspaces"`
}

func parseLicense(val interface{}) string {
	// the license isn't always a string, check for legacy struct if not string
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		if license, ok := v["type"]; ok {
			text, _ := license.(string)
			return text
		}
	}
	return ""
}

type parser struct{}

func newNpmParse() *parser {
	return &parser{}
}

func (*parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]godeptypes.Library, []godeptypes.Dependency, error) {
	ctx := types.ContextOf(r)
	var pkgJSON packageJSON
	// todo: use json field select
	data, err := textdecode.ReadRaw(ctx, r, 16<<20)
	if err != nil {
		return nil, nil, err
	}
	if _, err := jsonrecord.Decode(ctx, data, &pkgJSON); err != nil {
		return nil, nil, err
	}

	id := fmt.Sprintf("%s@%s", pkgJSON.Name, pkgJSON.Version)
	lib := godeptypes.Library{
		ID:      id,
		Name:    pkgJSON.Name,
		Version: pkgJSON.Version,
		License: parseLicense(pkgJSON.License),
	}

	var libs []godeptypes.Library
	if lib.Name != "" {
		libs = append(libs, lib)
	}
	dep := godeptypes.Dependency{ID: id}
	for _, scope := range []struct {
		name    string
		values  map[string]string
		emitLib bool
		dev     bool
	}{{"runtime", pkgJSON.Dependencies, true, false}, {"optional", pkgJSON.OptionalDependencies, true, false}, {"dev", pkgJSON.DevDependencies, false, true}} {
		names := make([]string, 0, len(scope.values))
		for n := range scope.values {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			version := scope.values[name]
			if scope.emitLib {
				ref := "declaration:" + name
				libs = append(libs, godeptypes.Library{ID: ref, Name: name, Version: version, IsVersionRange: !exactNPMVersion.MatchString(version), Evidence: "declared", DeclaredName: name, DeclaredVersion: version, Dev: scope.dev, Scope: scope.name})
				dep.DependsOn = append(dep.DependsOn, ref)
			}
			dep.Requirements = append(dep.Requirements, godeptypes.Requirement{Target: name, Constraint: version, Scope: scope.name})
		}
	}
	return libs, []godeptypes.Dependency{dep}, nil
}

var exactNPMVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
