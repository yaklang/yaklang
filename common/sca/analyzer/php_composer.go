package analyzer

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/aquasecurity/go-dep-parser/pkg/php/composer"
	"golang.org/x/exp/slices"
)

const (
	TypComposer TypAnalyzer = "composer-lang"

	phpLockFile = "composer.lock"
	phpJsonFile = "composer.json"

	statusComposerLock int = 1
	statusComposerJson int = 2
)

func init() {
	RegisterAnalyzer(TypComposer, NewPHPComposerAnalyzer())
}

type composerAnalyzer struct{}

func NewPHPComposerAnalyzer() *composerAnalyzer {
	return &composerAnalyzer{}
}

func (a composerAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
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
	fileName := filepath.Base(info.path)
	// Skip `composer.lock` inside `vendor` folder
	if slices.Contains(strings.Split(info.path, "/"), "vendor") {
		return 0
	}
	if fileName == phpJsonFile {
		return statusComposerJson
	}
	if fileName == phpLockFile {
		return statusComposerLock
	}
	return 0
}
