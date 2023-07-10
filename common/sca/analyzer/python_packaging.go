package analyzer

import (
	"archive/zip"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"io"
	"os"
	"strings"

	"github.com/aquasecurity/go-dep-parser/pkg/python/packaging"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	TypPythonPackaging TypAnalyzer = "python-packaging-lang"

	statusPythonPackaging int = 1
	statusEgg             int = 2
)

var (
	// .egg format
	// https://setuptools.readthedocs.io/en/latest/deprecated/python_eggs.html#eggs-and-their-formats
	eggFile = ".egg" // zip format

	pythonPackagingrequiredFiles = []string{

		"EGG-INFO/PKG-INFO",

		// .egg-info format: .egg-info can be Analyzer file or directory
		// https://setuptools.readthedocs.io/en/latest/deprecated/python_eggs.html#eggs-and-their-formats
		".egg-info",
		".egg-info/PKG-INFO",

		// wheel
		".dist-info/METADATA",
	}
)

func init() {
	RegisterAnalyzer(TypPythonPackaging, NewPythonPackagingAnalyzer())
}

type pythonPackagingAnalyzer struct{}

func NewPythonPackagingAnalyzer() *pythonPackagingAnalyzer {
	return &pythonPackagingAnalyzer{}
}

func (a pythonPackagingAnalyzer) Match(info MatchInfo) int {
	for _, r := range pythonPackagingrequiredFiles {
		if strings.HasSuffix(info.path, r) {
			return statusPythonPackaging
		}
	}
	if strings.HasSuffix(info.path, eggFile) {
		return statusEgg
	}
	return 0
}

func (a pythonPackagingAnalyzer) Analyze(afi AnalyzeFileInfo) ([]dxtypes.Package, error) {
	fi := afi.Self

	switch fi.MatchStatus {
	case statusEgg:
		realFileInfo, err := fi.File.Stat()
		if err != nil {
			return nil, utils.Errorf("failed to get file info: %s", err)
		}
		zr, err := zip.NewReader(fi.File, realFileInfo.Size())
		for _, vf := range zr.File {
			matched := a.Match(MatchInfo{
				path: vf.Name,
			})
			// no matched, skip
			if matched == 0 {
				continue
			}

			// open zip file, write to tmp file
			r, err := vf.Open()
			if err != nil {
				return nil, err
			}
			defer r.Close()

			f, err := os.CreateTemp("", "python-egg-file-*")
			if err != nil {
				return nil, err
			}
			defer func() {
				name := f.Name()
				f.Close()
				os.Remove(name)
			}()

			if _, err = io.Copy(f, r); err != nil {
				return nil, err
			}
			// reset file offset to read
			f.Seek(0, 0)

			return ParseLanguageConfiguration(FileInfo{
				File: f,
			}, packaging.NewParser())
		}
	case statusPythonPackaging:
		return ParseLanguageConfiguration(fi, packaging.NewParser())
	}

	return nil, nil
}
