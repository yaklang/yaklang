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
	name           string
	filePath       string
	virtualPath    string
	wantPkgs       []types.Package
	wantError      bool
	t              *testing.T
	a              Analyzer
	matchType      int
	matchedFileMap map[string]string
}

func Run(tc testcase) {
	t := tc.t
	f, err := os.Open(tc.filePath)
	if err != nil {
		t.Fatalf("%s: con't open file: %v", err, tc.name)
	}
	matchedFileInfos := lo.MapEntries(tc.matchedFileMap, func(k, v string) (string, fileInfo) {
		f, err := os.Open(v)
		if err != nil {
			t.Fatalf("%s: con't open file: %v", err, tc.name)
		}
		return k, fileInfo{
			path:        k,
			a:           tc.a,
			f:           f,
			matchStatus: tc.matchType,
		}
	})

	pkgs, err := tc.a.Analyze(AnalyzeFileInfo{
		self: fileInfo{
			path:        tc.virtualPath,
			a:           tc.a,
			f:           f,
			matchStatus: tc.matchType,
		},
		matchedFileInfos: matchedFileInfos,
	})

	if tc.wantError && err == nil {
		t.Fatalf("%s: want error but nil", tc.name)
	}
	if !tc.wantError && err != nil {
		t.Fatalf("%s: analyze error: %v", tc.name, err)
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
		t.Fatalf("%s: pkgs length error: %d(got) != %d(want)", tc.name, len(pkgs), len(tc.wantPkgs))
	}

	for i := 0; i < len(pkgs); i++ {
		if pkgs[i].Name != tc.wantPkgs[i].Name {
			t.Fatalf("%s: pkgs %d name error: %s(got) != %s(want)", tc.name, i, pkgs[i].Name, tc.wantPkgs[i].Name)
		}
		if pkgs[i].Version != tc.wantPkgs[i].Version {
			t.Fatalf("%s: pkgs %d(%s) version error: %s(got) != %s(want)", tc.name, i, pkgs[i].Name, pkgs[i].Version, tc.wantPkgs[i].Version)
		}
		if pkgs[i].Indirect != tc.wantPkgs[i].Indirect {
			t.Fatalf("%s: pkgs %d(%s) indirect error: %v(got) != %v(want)", tc.name, i, pkgs[i].Name, pkgs[i].Indirect, tc.wantPkgs[i].Indirect)
		}
	}
}

// package
func TestRPM(t *testing.T) {
	// positive
	tc := testcase{
		name:      "positive",
		filePath:  "./testdata/rpm/rpmdb.sqlite",
		wantPkgs:  RpmWantPkgs,
		t:         t,
		a:         NewRPMAnalyzer(),
		matchType: statusRPM,
	}
	Run(tc)
}

func TestApk(t *testing.T) {
	// positive
	tc := testcase{
		name:     "positive",
		filePath: "./testdata/apk/apk",
		wantPkgs: ApkWantPkgs,

		t:         t,
		a:         NewApkAnalyzer(),
		matchType: 1,
	}
	Run(tc)

	// negative
	tc = testcase{
		name:     "negative",
		filePath: "./testdata/apk/negative-apk",
		wantPkgs: []types.Package{
			{
				Name:    "ssl_client",
				Version: "1.36.1-r0",
			},
			{
				Name:    "zlib",
				Version: "1.2.13-r1",
			},
		},
		t:         t,
		a:         NewApkAnalyzer(),
		matchType: 1,
	}
	Run(tc)
}

func TestDpkg(t *testing.T) {
	// positive
	a := NewDpkgAnalyzer()
	tc := testcase{
		name:      "positive",
		filePath:  "./testdata/dpkg/dpkg",
		t:         t,
		a:         a,
		matchType: 1,
		wantPkgs:  DpkgWantPkgs,
	}
	Run(tc)

	// negative
	tc = testcase{
		name:      "negative",
		filePath:  "./testdata/dpkg/negative-dpkg",
		t:         t,
		a:         a,
		matchType: 1,
		wantPkgs:  []types.Package{},
	}
	Run(tc)
}

// language
func TestConan(t *testing.T) {
	// positive
	tc := testcase{
		name:      "positive",
		filePath:  "./testdata/conan/conan",
		t:         t,
		a:         NewConanAnalyzer(),
		matchType: 1,
		wantPkgs: []types.Package{
			{
				Name:    "openssl",
				Version: "3.0.5",
			},
			{
				Name:     "zlib",
				Version:  "1.2.12",
				Indirect: true,
			},
		},
	}
	Run(tc)

	// negative
	tc = testcase{
		name:      "negative",
		filePath:  "./testdata/conan/negative-conan",
		t:         t,
		a:         NewConanAnalyzer(),
		matchType: 1,
		wantPkgs:  []types.Package{},
	}
	Run(tc)
}

func TestGoBinary(t *testing.T) {
	// positive
	tc := testcase{
		name:      "positive",
		filePath:  "./testdata/go_binary/go-binary",
		t:         t,
		a:         NewGoBinaryAnalyzer(),
		matchType: 1,
		wantPkgs: []types.Package{
			{
				Name:    "github.com/aquasecurity/go-pep440-version",
				Version: "v0.0.0-20210121094942-22b2f8951d46",
			},
			{
				Name:    "github.com/aquasecurity/go-version",
				Version: "v0.0.0-20210121072130-637058cfe492",
			},
			{
				Name:    "golang.org/x/xerrors",
				Version: "v0.0.0-20200804184101-5ec99f83aff1",
			},
		},
	}
	Run(tc)

	// negative broken elf
	tc = testcase{
		name:      "negative-broken-elf",
		filePath:  "./testdata/go_binary/negative-go-binary-broken_elf",
		t:         t,
		a:         NewGoBinaryAnalyzer(),
		matchType: 1,
		wantPkgs:  []types.Package{},
	}
	Run(tc)

	// negative bash
	tc = testcase{
		name:      "negative-bash",
		filePath:  "./testdata/go_binary/negative-go-binary-bash",
		t:         t,
		a:         NewGoBinaryAnalyzer(),
		matchType: 1,
		wantPkgs:  []types.Package{},
	}
	Run(tc)
}

func TestGoMod(t *testing.T) {
	// positive
	tc := testcase{
		name:        "positive",
		filePath:    "./testdata/go_mod/positive/mod",
		virtualPath: "/test/go.mod",
		t:           t,
		a:           NewGoModAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/test/go.sum": "./testdata/go_mod/positive/sum",
		},
		wantPkgs: []types.Package{
			{
				Name:    "github.com/aquasecurity/go-dep-parser",
				Version: "0.0.0-20220406074731-71021a481237",
			},
			{
				Name:     "golang.org/x/xerrors",
				Version:  "0.0.0-20200804184101-5ec99f83aff1",
				Indirect: true,
			},
		},
	}
	Run(tc)

	// postivie less than golang 1.17, nedd parse go.sum
	tc = testcase{
		name:        "positive-less-than-117",
		filePath:    "./testdata/go_mod/lessthan117/mod",
		virtualPath: "/test/go.mod",
		t:           t,
		a:           NewGoModAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/test/go.sum": "./testdata/go_mod/lessthan117/sum",
		},
		wantPkgs: []types.Package{
			{
				Name:    "github.com/aquasecurity/go-dep-parser",
				Version: "0.0.0-20230219131432-590b1dfb6edd",
			},
			{
				Name:     "github.com/BurntSushi/toml",
				Version:  "0.3.1",
				Indirect: true,
			},
		},
	}
	Run(tc)

	// negative
	tc = testcase{
		name:        "negative-wrongmod",
		filePath:    "./testdata/go_mod/negative/mod",
		virtualPath: "/test/go.mod",
		t:           t,
		a:           NewGoModAnalyzer(),
		matchType:   1,
		wantPkgs:    []types.Package{},
		wantError:   true,
	}
	Run(tc)
}

func TestPHPComposer(t *testing.T) {
	tc := testcase{
		name:        "positive",
		filePath:    "./testdata/php_composer/composer.lock",
		virtualPath: "/test/composer.lock",
		t:           t,
		a:           NewPHPComposerAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/test/composer.json": "./testdata/php_composer/composer.json",
		},
		wantPkgs: []types.Package{
			{
				Name:     "pear/log",
				Version:  "1.13.3",
				Indirect: false,
			},
			{
				Name:     "pear/pear_exception",
				Version:  "v1.0.2",
				Indirect: true,
			},
		},
	}
	Run(tc)

	// json error
	tc = testcase{
		name:        "negative-wrongjson",
		filePath:    "./testdata/php_composer/composer.lock",
		virtualPath: "/test/composer.lock",
		t:           t,
		a:           NewPHPComposerAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/test/composer.json": "./testdata/php_composer/wrong.json",
		},
		wantPkgs: []types.Package{
			{
				Name:     "pear/log",
				Version:  "1.13.3",
				Indirect: false,
			},
			{
				Name:     "pear/pear_exception",
				Version:  "v1.0.2",
				Indirect: false,
			},
		},
	}
	Run(tc)

	// no json file
	tc = testcase{
		name:           "negative-nojson",
		filePath:       "./testdata/php_composer/composer.lock",
		virtualPath:    "/test/composer.lock",
		t:              t,
		a:              NewPHPComposerAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs: []types.Package{
			{
				Name:     "pear/log",
				Version:  "1.13.3",
				Indirect: false,
			},
			{
				Name:     "pear/pear_exception",
				Version:  "v1.0.2",
				Indirect: false,
			},
		},
	}
	tc.matchedFileMap = map[string]string{}
	tc.name = "nojson"
	Run(tc)

	// lock error
	// tc.wantPkgs = []types.Package{}
	// tc.filePath = "./testdata/php_composer/wrong.json"
	// tc.name = "wronglock"
	// Run(tc)
}

func TestPythonPackaging(t *testing.T) {
	// positive
	tc := testcase{
		name:           "positive-egg-zip",
		filePath:       "./testdata/python_packaging/egg/kitchen-1.2.6-py2.7.egg",
		t:              t,
		a:              NewPythonPackagingAnalyzer(),
		matchType:      2,
		matchedFileMap: map[string]string{},
		wantPkgs: []types.Package{
			{
				Name:    "kitchen",
				Version: "1.2.6",
			},
		},
	}
	Run(tc)

	tc = testcase{
		name:           "positive-egg-info",
		filePath:       "./testdata/python_packaging/egg-info/PKG-INFO",
		t:              t,
		a:              NewPythonPackagingAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs: []types.Package{
			{
				Name:    "distlib",
				Version: "0.3.1",
			},
		},
	}
	Run(tc)

	tc = testcase{
		name:           "positive-wheel",
		filePath:       "./testdata/python_packaging/dist-info/METADATA",
		t:              t,
		a:              NewPythonPackagingAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs: []types.Package{
			{
				Name:    "distlib",
				Version: "0.3.1",
			},
		},
	}
	Run(tc)

	// negative
	tc = testcase{
		name:           "positive-no-required-files",
		filePath:       "./testdata/python_packaging/egg/no-required-files.egg",
		t:              t,
		a:              NewPythonPackagingAnalyzer(),
		matchType:      2,
		matchedFileMap: map[string]string{},
		wantPkgs:       []types.Package{},
	}
	Run(tc)
}

func TestFilterAnalyzer(t *testing.T) {
	wantPkgAnalyzerTypes := []string{
		reflect.TypeOf(NewRPMAnalyzer()).String(),
		reflect.TypeOf(NewDpkgAnalyzer()).String(),
		reflect.TypeOf(NewApkAnalyzer()).String(),
	}
	wantLangAnalyzerTypes := []string{
		reflect.TypeOf(NewConanAnalyzer()).String(),
		reflect.TypeOf(NewGoBinaryAnalyzer()).String(),
		reflect.TypeOf(NewGoModAnalyzer()).String(),
	}

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
