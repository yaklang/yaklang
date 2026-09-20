package analyzer

import (
	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/php/composer"
	"slices"
)

const (
	TypPHPComposer TypAnalyzer = "composer-lang"

	phpLockFile = "composer.lock"
	phpJsonFile = "composer.json"

	statusComposerLock int = 1
	statusComposerJson int = 2
)

func init() {
	RegisterAnalyzer(TypPHPComposer, NewPHPComposerAnalyzer())
}

type composerAnalyzer struct{}

func NewPHPComposerAnalyzer() *composerAnalyzer {
	return &composerAnalyzer{}
}

func (a composerAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusComposerJson:
		var manifest struct {
			Name, Version string
			Require       map[string]string `json:"require"`
			License       []string          `json:"license"`
		}
		raw, err := io.ReadAll(io.LimitReader(fi.LazyFile, (16<<20)+1))
		if err != nil {
			return nil, err
		}
		if _, err = jsonrecord.Decode(fi.LazyFile.Context(), raw, &manifest); err != nil {
			return nil, err
		}
		var packages []*dxtypes.Package
		if manifest.Name != "" {
			packages = append(packages, &dxtypes.Package{Name: manifest.Name, Version: manifest.Version, PackageDetails: &dxtypes.PackageDetails{Evidence: "declared", RawLicenses: manifest.License}})
		}
		for name, constraint := range manifest.Require {
			packages = append(packages, &dxtypes.Package{Name: name, Version: constraint, IsVersionRange: !exactNPMVersion.MatchString(constraint), PackageDetails: &dxtypes.PackageDetails{Evidence: "declared", DeclaredName: name, DeclaredVersion: constraint, Instance: "declaration:" + name}})
		}
		return packages, nil
	case statusComposerLock:
		// parse composer lock file
		lockParser := composer.NewParser()
		pkgs, err := ParseLanguageConfiguration(fi, lockParser)
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a composerAnalyzer) Match(info MatchInfo) int {
	_, filename := info.fileSystem.PathSplit(info.Path)
	// Skip `composer.lock` inside `vendor` folder
	if slices.Contains(strings.Split(info.Path, "/"), "vendor") {
		return 0
	}
	if filename == phpJsonFile {
		return statusComposerJson
	}
	if filename == phpLockFile {
		return statusComposerLock
	}
	return 0
}
