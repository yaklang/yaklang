package analyzer

import (
	"database/sql"

	rpmdb "github.com/knqyf263/go-rpmdb/pkg"
	"github.com/mattn/go-sqlite3"

	"github.com/yaklang/yaklang/common/sca/types"
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
	requiredFiles = []string{
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

func (a rpmAnalyzer) Analyze(afi AnalyzeFileInfo) ([]types.Package, error) {
	fi := afi.self
	switch fi.matchStatus {
	case statusRPM:
		db, err := rpmdb.Open(fi.f.Name())
		if err != nil {
			return nil, utils.Errorf("failed to open RPM DB: %v", err)
		}
		pkgList, err := db.ListPackages()
		if err != nil {
			return nil, utils.Errorf("failed to list packages: %v", err)
		}
		pkgs := make([]types.Package, len(pkgList))
		for i, pkgInfo := range pkgList {
			pkgs[i] = types.Package{
				Name:    pkgInfo.Name,
				Version: pkgInfo.Version,
			}
		}

		return pkgs, nil
	}
	return nil, nil
}

func (a rpmAnalyzer) Match(info MatchInfo) int {
	if utils.StringSliceContainsAll(requiredFiles, info.path) {
		return statusRPM
	}
	return 0
}
