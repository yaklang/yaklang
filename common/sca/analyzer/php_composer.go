package analyzer

import (
	"path/filepath"
	"strings"

	"github.com/aquasecurity/go-dep-parser/pkg/c/conan"
	"github.com/yaklang/yaklang/common/sca/types"
	"golang.org/x/exp/slices"
)

const (
	TypComposer TypAnalyzer = "composer-lang"

	statusComposer int = 1
)

var phprequiredFiles = []string{
	// types.ComposerLock,
	// types.ComposerJson,
}

func init() {
	RegisterAnalyzer(TypConan, NewConanAnalyzer())
}

type composerAnalyzer struct{}

func NewComposerAnalyzer() *composerAnalyzer {
	return &composerAnalyzer{}
}

func (a composerAnalyzer) Analyze(afi AnalyzeFileInfo) ([]types.Package, error) {
	fi := afi.self
	switch fi.matchStatus {
	case statusConan:
		p := conan.NewParser()
		res, err := ParseLanguageConfiguration(fi, p)
		if err != nil {
			return nil, err
		}
		return res, nil
	}
	return nil, nil
}

func (a composerAnalyzer) Match(info MatchInfo) int {
	fileName := filepath.Base(info.path)
	if !slices.Contains(requiredFiles, fileName) {
		return 0
	}
	// Skip `composer.lock` inside `vendor` folder
	if slices.Contains(strings.Split(info.path, "/"), "vendor") {
		return 0
	}
	return statusComposer
}
