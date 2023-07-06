package analyzer

import (
	"io/fs"
	"strings"

	"github.com/aquasecurity/go-dep-parser/pkg/c/conan"
	"github.com/yaklang/yaklang/common/sca/types"
)

const (
	TypConan TypAnalyzer = "conan-lang"

	ConanLock = "conan.lock"

	statusConan int = 1
)

func init() {
	RegisterAnalyzer(TypConan, NewConanAnalyzer())
}

type conanAnalyzer struct{}

func NewConanAnalyzer() *conanAnalyzer {
	return &conanAnalyzer{}
}

func (a conanAnalyzer) Analyze(fi AnalyzeFileInfo) ([]types.Package, error) {
	p := conan.NewParser()
	res, err := ParseLanguageConfiguration(fi, p)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (a conanAnalyzer) Match(path string, fi fs.FileInfo) int {
	if strings.HasSuffix(path, ConanLock) {
		return statusConan
	}
	return 0
}
