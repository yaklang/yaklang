package analyzer

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	licenses "github.com/yaklang/yaklang/common/sca/license"

	rpmdb "github.com/knqyf263/go-rpmdb/pkg"
	"github.com/mattn/go-sqlite3"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	TypRPM TypAnalyzer = "rpm-pkg"

	statusRPM int = 1
)

func init() {
	RegisterAnalyzer(TypRPM, NewRPMAnalyzer())

	sql.Register("sqlite", &sqlite3.SQLiteDriver{})
}

var (
	rpmRequiredFiles = []string{
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
)

type rpmAnalyzer struct {
}

func NewRPMAnalyzer() *rpmAnalyzer {
	return &rpmAnalyzer{}
}

func (a rpmAnalyzer) createPackage(pkgInfo *rpmdb.PackageInfo, provides map[string]*dxtypes.Package) *dxtypes.Package {
	pkg := &dxtypes.Package{
		Name:         pkgInfo.Name,
		Version:      pkgInfo.Version,
		Verification: fmt.Sprintf("md5:%s", pkgInfo.SigMD5),
		License:      []string{licenses.Normalize(pkgInfo.License)},
	}
	for _, provide := range pkgInfo.Provides {
		// handler libc.so.6(GLIBC_2.2.5)(64bit) => libc.so.6
		if strings.Contains(provide, "(") {
			provide = provide[:strings.Index(provide, "(")]
		}
		// handler /usr/bin/pkg-config => pkg-config
		if strings.Contains(provide, "/") {
			provide = provide[strings.LastIndex(provide, "/")+1:]
		}
		provides[provide] = pkg
	}
	pkg.DependsOn.And = make(map[string]string)
	for _, dep := range pkgInfo.Requires {
		// pass rpm package manage
		// all package depend on rpm because these installed by rpm
		if strings.HasPrefix(dep, "rpmlib") {
			continue
		}

		// handler libc.so.6(GLIBC_2.2.5)(64bit) => libc.so.6
		if strings.Contains(dep, "(") {
			dep = dep[:strings.Index(dep, "(")]
		}
		// handler /usr/bin/pkg-config => pkg-config
		if strings.Contains(dep, "/") {
			dep = dep[strings.LastIndex(dep, "/")+1:]
		}

		// remove depende that provide by self
		if p, ok := provides[dep]; ok {
			if p == pkg {
				continue
			}
			pkg.DependsOn.And[dep] = p.Version
		} else {
			pkg.DependsOn.And[dep] = "*"
		}
	}
	return pkg
}

func (a rpmAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusRPM:
		provides := make(map[string]*dxtypes.Package)

		db, err := rpmdb.Open(fi.LazyFile.Name())
		if err != nil {
			return nil, utils.Errorf("failed to open RPM DB: %v", err)
		}
		pkgList, err := db.ListPackages()
		if err != nil {
			return nil, utils.Errorf("failed to list packages: %v", err)
		}
		pkgs := make([]*dxtypes.Package, len(pkgList))
		for i, pkgInfo := range pkgList {
			pkgs[i] = a.createPackage(pkgInfo, provides)
		}
		handleDependsOn(pkgs, provides)
		// lo.ForEach(pkgs, func(pkg *dxtypes.Package, _ int) {
		// 	fmt.Printf(`
		// 	name: %s
		// 	depends: %v
		// 	`, pkg.Name, pkg.DependsOn)
		// })
		return makePotentialPkgs(pkgs), nil
	}
	return nil, nil
}

func (a rpmAnalyzer) Match(info MatchInfo) int {
	if utils.StringSliceContainsAll(rpmRequiredFiles, info.path) {
		return statusRPM
	}
	return 0
}
