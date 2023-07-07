package analyzer

import (
	"bufio"

	"github.com/yaklang/yaklang/common/sca/types"
)

const (
	TypAPK TypAnalyzer = "apk-pkg"

	// installed file
	installFile           = "lib/apk/db/installed"
	statusInstallFile int = 1
)

func init() {
	RegisterAnalyzer(TypAPK, NewApkAnalyzer())
}

type apkAnalyzer struct{}

func NewApkAnalyzer() *apkAnalyzer {
	return &apkAnalyzer{}
}

func (a apkAnalyzer) Analyze(afi AnalyzeFileInfo) ([]types.Package, error) {
	fi := afi.self
	switch fi.matchStatus {
	case statusInstallFile:
		var (
			pkgs    []types.Package
			pkg     types.Package
			version string
		)
		scanner := bufio.NewScanner(fi.f)

		for scanner.Scan() {
			line := scanner.Text()

			if len(line) < 2 {
				if pkg.Name != "" && pkg.Version != "" {
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
		if pkg.Name != "" && pkg.Version != "" {
			pkgs = append(pkgs, pkg)
		}

		return pkgs, nil

	}
	return nil, nil
}

func (a apkAnalyzer) Match(info MatchInfo) int {
	if info.path == installFile {
		return statusInstallFile
	}
	return 0
}
