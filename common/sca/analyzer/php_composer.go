package analyzer

import (
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/php/composer"
	"golang.org/x/exp/slices"
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
