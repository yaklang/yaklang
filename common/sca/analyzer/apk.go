package analyzer

import (
	"bufio"
	"io/fs"

	"github.com/yaklang/yaklang/common/sca/types"
)

const (
	TypAPK TypAnalyzer = "apk-pkg"

	installFile = "lib/apk/db/installed"

	TypeInstallFile int = 1
)

func init() {
	RegisterAnalyzer(TypAPK, NewApkAnalyzer())
}

type apkAnalyzer struct{}

func NewApkAnalyzer() *apkAnalyzer {
	return &apkAnalyzer{}
}

func (a apkAnalyzer) Analyze(fi AnalyzeFileInfo) ([]types.Package, error) {
	var (
		pkgs    []types.Package
		pkg     types.Package
		version string
	)
	scanner := bufio.NewScanner(fi.f)

	for scanner.Scan() {
		line := scanner.Text()

		if len(line) < 2 {
			if pkg.Name != "" {
				pkgs = append(pkgs, pkg)
			}
			pkg = types.Package{}
			continue
		}
		// ref. https://wiki.alpinelinux.org/wiki/Apk_spec
		switch line[:2] {
		case "P:":
			pkg.Name = line[2:]
		case "V:":
			version = line[2:]
			pkg.Version = version
		}
	}
	if pkg.Name != "" {
		pkgs = append(pkgs, pkg)
	}

	return pkgs, nil
}

func (a apkAnalyzer) Match(path string, fi fs.FileInfo) int {
	if path == installFile {
		return TypeInstallFile
	}
	return 0
}
