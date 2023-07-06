package analyzer

import (
	"database/sql"
	"io/fs"

	rpmdb "github.com/knqyf263/go-rpmdb/pkg"
	"github.com/mattn/go-sqlite3"

	"github.com/yaklang/yaklang/common/sca/types"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	TypRPM TypAnalyzer = "rpm-pkg"
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

	TypAnalyzeRPM int = 1
)

type rpmAnalyzer struct {
}

func NewRPMAnalyzer() *rpmAnalyzer {
	return &rpmAnalyzer{}
}

func (a rpmAnalyzer) Analyze(fi AnalyzeFileInfo) ([]types.Package, error) {
	switch fi.matchType {
	case TypAnalyzeRPM:
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

func (a rpmAnalyzer) Match(path string, info fs.FileInfo) int {
	if utils.StringSliceContainsAll(requiredFiles, path) {
		return TypAnalyzeRPM
	}
	return 0
}
