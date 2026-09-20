package analyzer

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/lazyfile"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/python/packaging"
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
		if strings.HasSuffix(info.Path, r) {
			return statusPythonPackaging
		}
	}
	if strings.HasSuffix(info.Path, eggFile) {
		return statusEgg
	}
	return 0
}

func (a pythonPackagingAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self

	switch fi.MatchStatus {
	case statusEgg:
		realFileInfo, err := fi.LazyFile.Stat()
		if err != nil {
			return nil, fmt.Errorf("failed to get file info: %s", err)
		}
		zr, err := zip.NewReader(fi.LazyFile, realFileInfo.Size())
		if err != nil {
			return nil, err
		}
		policy := budget.From(fi.LazyFile.Context())
		if err = policy.Archive(len(zr.File), 0); err != nil {
			return nil, err
		}
		for _, vf := range zr.File {
			if err = fi.LazyFile.Context().Err(); err != nil {
				return nil, err
			}
			if !fs.ValidPath(strings.TrimSuffix(vf.Name, "/")) || strings.ContainsAny(vf.Name, `\:`) || vf.Mode()&fs.ModeSymlink != 0 {
				return nil, fmt.Errorf("invalid_path: egg entry")
			}
			if vf.UncompressedSize64 > uint64(policy.Limits.MaxFileBytes) {
				return nil, fmt.Errorf("resource_limit: egg entry")
			}
			if err = policy.Archive(0, int64(vf.UncompressedSize64)); err != nil {
				return nil, err
			}
			matched := a.Match(MatchInfo{
				Path: vf.Name,
			})
			// no matched, skip
			if matched == 0 {
				continue
			}

			r, err := vf.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(io.LimitReader(r, policy.Limits.MaxFileBytes+1))
			r.Close()
			if err != nil {
				return nil, err
			}
			if int64(len(data)) > policy.Limits.MaxFileBytes {
				return nil, fmt.Errorf("resource_limit: egg expansion")
			}
			nested := lazyfile.NewMemory(fi.Path+"!/"+vf.Name, data)
			nested.SetContext(fi.LazyFile.Context())
			return ParseLanguageConfiguration(&FileInfo{Path: fi.Path + "!/" + vf.Name, LazyFile: nested, filesystem: fi.filesystem}, packaging.NewParser())
		}
	case statusPythonPackaging:
		return ParseLanguageConfiguration(fi, packaging.NewParser())
	}

	return nil, nil
}
