package analyzer

import (
	"fmt"
	"slices"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	licenses "github.com/yaklang/yaklang/common/sca/license"

	rpmdb "github.com/yaklang/yaklang/common/sca/core/rpm"
)

const (
	TypRPM TypAnalyzer = "rpm-pkg"

	statusRPM int = 1
)

func init() {
	RegisterAnalyzer(TypRPM, NewRPMAnalyzer())

}

var rpmRequiredFiles = []string{
	// Berkeley DB
	"usr/lib/sysimage/rpm/Packages",
	"var/lib/rpm/Packages",

	// NDB
	"usr/lib/sysimage/rpm/Packages.db",
	"var/lib/rpm/Packages.db",

	// SQLite3
	"usr/lib/sysimage/rpm/rpmdb.sqlite",
	"var/lib/rpm/rpmdb.sqlite",
}

type rpmAnalyzer struct{}

func NewRPMAnalyzer() *rpmAnalyzer {
	return &rpmAnalyzer{}
}

func (a rpmAnalyzer) createPackage(pkgInfo *rpmdb.PackageInfo, provides map[string]*dxtypes.Package) *dxtypes.Package {
	pkg := &dxtypes.Package{
		Name: pkgInfo.Name,

		Version:      pkgInfo.Version,
		Verification: fmt.Sprintf("md5:%s", pkgInfo.SigMD5),
		License:      []string{licenses.Normalize(pkgInfo.License)}, PackageDetails: &dxtypes.PackageDetails{RawLicenses: []string{pkgInfo.License}},
	}
	pkg.Architecture = pkgInfo.Arch
	pkg.Variant = fmt.Sprintf("epoch=%d;release=%s", pkgInfo.Epoch, pkgInfo.Release)
	pkg.Evidence = "installed"
	pkg.Provides = append([]string(nil), pkgInfo.Provides...)
	pkg.DependsOn.And = map[string]string{}
	for _, dep := range pkgInfo.Requires {
		pkg.DependsOn.And[dep] = ""
	}
	return pkg
}

func (a rpmAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusRPM:
		provides := make(map[string]*dxtypes.Package)

		stat, err := fi.LazyFile.Stat()
		if err != nil {
			return nil, err
		}
		pkgList, err := rpmdb.Parse(afi.Self.LazyFile.Context(), fi.LazyFile, stat.Size(), rpmdb.Limits{})
		if err != nil {
			return nil, fmt.Errorf("failed to list packages: %v", err)
		}
		pkgs := make([]*dxtypes.Package, len(pkgList))
		for i, pkgInfo := range pkgList {
			pkgs[i] = a.createPackage(pkgInfo, provides)
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a rpmAnalyzer) Match(info MatchInfo) int {
	if slices.Contains(rpmRequiredFiles, info.Path) {
		return statusRPM
	}
	return 0
}
