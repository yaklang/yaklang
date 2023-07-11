package analyzer

import (
	"database/sql"
	"fmt"

	"github.com/samber/lo"
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

func (a rpmAnalyzer) Analyze(afi AnalyzeFileInfo) ([]dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusRPM:
		provides := make(map[string]*dxtypes.Package)

		db, err := rpmdb.Open(fi.File.Name())
		if err != nil {
			return nil, utils.Errorf("failed to open RPM DB: %v", err)
		}
		pkgList, err := db.ListPackages()
		if err != nil {
			return nil, utils.Errorf("failed to list packages: %v", err)
		}
		pkgs := make([]dxtypes.Package, len(pkgList))
		for i, pkgInfo := range pkgList {
			pkgs[i] = dxtypes.Package{
				Name:         pkgInfo.Name,
				Version:      pkgInfo.Version,
				Verification: fmt.Sprintf("md5:%s", pkgInfo.SigMD5),
				DependsOn: dxtypes.PackageRelationShip{
					And: lo.SliceToMap(pkgInfo.Requires, func(depName string) (string, string) {
						return depName, "*" // version is not available
					}),
				},
				License: []string{licenses.Normalize(pkgInfo.License)},
			}
			for _, provide := range pkgInfo.Provides {
				provides[provide] = &pkgs[i]
			}
		}
		handleDependsOn(pkgs, provides)
		linkUpSteamAndDownStream(pkgs)

		return pkgs, nil
	}
	return nil, nil
}

func (a rpmAnalyzer) Match(info MatchInfo) int {
	if utils.StringSliceContainsAll(rpmRequiredFiles, info.path) {
		return statusRPM
	}
	return 0
}
