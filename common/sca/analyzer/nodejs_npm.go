package analyzer

import (
	"encoding/json"
	"fmt"

	dio "github.com/aquasecurity/go-dep-parser/pkg/io"
	"github.com/aquasecurity/go-dep-parser/pkg/nodejs/npm"
	godeptypes "github.com/aquasecurity/go-dep-parser/pkg/types"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypNpm TypAnalyzer = "npm-lang"

	packageJson     = "package.json"
	packageLockJson = "package-lock.json"

	statusNpmJson     = 1
	statusNpmLockJson = 2
)

func init() {
	RegisterAnalyzer(TypNpm, NewNpmAnalyzer())
}

type npmAnalyzer struct{}

func NewNpmAnalyzer() *npmAnalyzer {
	return &npmAnalyzer{}
}

func (a npmAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusNpmJson:
		p := newNpmParse()
		pkgs, err := ParseLanguageConfiguration(fi, p)
		if err != nil {
			return nil, err
		}
		return pkgs, nil

	case statusNpmLockJson:
		p := npm.NewParser()
		pkgs, err := ParseLanguageConfiguration(fi, p)
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a npmAnalyzer) Match(info MatchInfo) int {
	if info.fi.Name() == packageJson {
		return statusNpmJson
	}
	if info.fi.Name() == packageLockJson {
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
	Workspaces           []string          `json:"workspaces"`
}

func parseLicense(val interface{}) string {
	// the license isn't always a string, check for legacy struct if not string
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		if license, ok := v["type"]; ok {
			return license.(string)
		}
	}
	return ""
}

type parser struct{}

func newNpmParse() *parser {
	return &parser{}
}

func (*parser) Parse(r dio.ReadSeekerAt) ([]godeptypes.Library, []godeptypes.Dependency, error) {
	var pkgJSON packageJSON
	//todo: use json field select
	if err := json.NewDecoder(r).Decode(&pkgJSON); err != nil {
		return nil, nil, nil
	}

	id := fmt.Sprintf("%s@%s", pkgJSON.Name, pkgJSON.Version)
	lib := godeptypes.Library{
		ID:      id,
		Name:    pkgJSON.Name,
		Version: pkgJSON.Version,
		License: parseLicense(pkgJSON.License),
	}

	dep := godeptypes.Dependency{
		ID: id,
		// depend id list
		DependsOn: lo.MapToSlice(pkgJSON.Dependencies, func(name, version string) string {
			return fmt.Sprintf("%s@%s", name, version)
		}),
	}

	return []godeptypes.Library{lib}, []godeptypes.Dependency{dep}, nil
}
