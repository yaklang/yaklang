package analyzer

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/samber/lo"
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

	sort.Slice(pkgs, func(i, j int) bool {
		c := strings.Compare(pkgs[i].Name, pkgs[j].Name)
		if c == 0 {
			return strings.Compare(pkgs[i].Version, pkgs[j].Version) > 0
		}
		return c > 0
	})
	sort.Slice(tc.wantPkgs, func(i, j int) bool {
		c := strings.Compare(tc.wantPkgs[i].Name, tc.wantPkgs[j].Name)
		if c == 0 {
			return strings.Compare(tc.wantPkgs[i].Version, tc.wantPkgs[j].Version) > 0
		}
		return c > 0
	})

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
		matchType: TypAnalyzeRPM,
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

func TestFilterAnalyzer(t *testing.T) {
	wantPkgAnalyzerTypes := []string{
		reflect.TypeOf(NewRPMAnalyzer()).String(),
		reflect.TypeOf(NewDpkgAnalyzer()).String(),
		reflect.TypeOf(NewApkAnalyzer()).String(),
	}
	wantLangAnalyzerTypes := []string{}

	wantAnalyzerTypes := []string{}
	wantAnalyzerTypes = append(wantAnalyzerTypes, wantPkgAnalyzerTypes...)
	wantAnalyzerTypes = append(wantAnalyzerTypes, wantLangAnalyzerTypes...)

	testcases := []struct {
		scanMode          ScanMode
		wantAnalyzerTypes []string
	}{
		{
			scanMode:          AllMode,
			wantAnalyzerTypes: wantAnalyzerTypes,
		},
		{
			scanMode:          AllMode | PkgMode, // mean PkgMode
			wantAnalyzerTypes: wantPkgAnalyzerTypes,
		},
		{
			scanMode:          PkgMode,
			wantAnalyzerTypes: wantPkgAnalyzerTypes,
		},
		{
			scanMode:          LanguageMode,
			wantAnalyzerTypes: wantLangAnalyzerTypes,
		},
	}

	for _, testcase := range testcases {
		wantTypes := testcase.wantAnalyzerTypes
		got := FilterAnalyzer(testcase.scanMode)
		gotTypes := lo.Map(got, func(a Analyzer, _ int) string {
			return reflect.TypeOf(a).String()
		})

		sort.Slice(wantTypes, func(i, j int) bool {
			return strings.Compare(wantTypes[i], wantTypes[j]) < 0
		})

		sort.Slice(gotTypes, func(i, j int) bool {
			return strings.Compare(gotTypes[i], gotTypes[j]) < 0
		})

		if len(got) != len(wantTypes) {
			t.Fatalf("analyzers length error: %d(got) != %d(want)", len(got), len(wantTypes))
		}
		if !reflect.DeepEqual(gotTypes, wantTypes) {
			t.Fatalf("analyzers error: %v(got) != %v(want)", gotTypes, wantTypes)
		}
	}
}
