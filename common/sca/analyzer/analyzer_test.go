package analyzer

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/sca/types"
)

type testcase struct {
	filePath  string
	wantPkgs  []types.Package
	t         *testing.T
	a         Analyzer
	matchType int
}

func Run(tc testcase) {
	t := tc.t
	f, err := os.Open(tc.filePath)
	if err != nil {
		t.Fatalf("con't open file: %v", err)
	}
	pkgs, err := tc.a.Analyze(AnalyzeFileInfo{
		path:      "",
		f:         f,
		matchType: tc.matchType,
	})
	if err != nil {
		t.Fatalf("analyzer error: %v", err)
	}

	if len(pkgs) != len(tc.wantPkgs) {
		t.Fatalf("pkgs length error: %d(got) != %d(want)", len(pkgs), len(tc.wantPkgs))
	}

	for i := 0; i < len(pkgs); i++ {
		if pkgs[i].Name != tc.wantPkgs[i].Name {
			t.Fatalf("pkgs %d name error: %s(got) != %s(want)", i, pkgs[i].Name, tc.wantPkgs[i].Name)
		}
		if pkgs[i].Version != tc.wantPkgs[i].Version {
			t.Fatalf("pkgs %d(%s) version error: %s(got) != %s(want)", i, pkgs[i].Name, pkgs[i].Version, tc.wantPkgs[i].Version)
		}
	}
}

func TestRPM(t *testing.T) {
	tc := testcase{
		filePath:  "./testdata/rpmdb.sqlite",
		wantPkgs:  RpmWantPkgs,
		t:         t,
		a:         NewRPMAnalyzer(),
		matchType: TypRPM,
	}
	Run(tc)
}

func TestApk(t *testing.T) {
	tc := testcase{
		filePath: "./testdata/apk",
		wantPkgs: ApkWantPkgs,

		t:         t,
		a:         NewApkAnalyzer(),
		matchType: 1,
	}
	Run(tc)
}

func TestDpkg(t *testing.T) {
	tc := testcase{
		filePath:  "./testdata/dpkg",
		t:         t,
		a:         NewDpkgAnalyzer(),
		matchType: 1,
		wantPkgs:  DpkgWantPkgs,
	}
	Run(tc)

}
