package sca

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/lazyfile"
	"golang.org/x/exp/slices"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/samber/lo"
)

type testcase struct {
	name           string
	filePath       string
	virtualPath    string
	wantPkgs       []dxtypes.Package
	wantError      bool
	t              *testing.T
	a              analyzer.Analyzer
	matchType      int
	matchedFileMap map[string]string
}

func Run(tc testcase) {
	t := tc.t
	fmt.Printf("TestCase: %s\n===============================\n", tc.name)

	f, err := os.Open(tc.filePath)
	if err != nil {
		t.Fatalf("%s: con't open file: %v", err, tc.name)
	}
	matchedFileInfos := lo.MapEntries(tc.matchedFileMap, func(k, v string) (string, analyzer.FileInfo) {
		f, err := os.Open(v)
		if err != nil {
			t.Fatalf("%s: con't open file: %v", err, tc.name)
		}
		return k, analyzer.FileInfo{
			Path:        k,
			Analyzer:    tc.a,
			LazyFile:    lazyfile.LazyOpenStreamByFile(f),
			MatchStatus: tc.matchType,
		}
	})

	pkgs, err := tc.a.Analyze(analyzer.AnalyzeFileInfo{
		Self: analyzer.FileInfo{
			Path:        tc.virtualPath,
			Analyzer:    tc.a,
			LazyFile:    lazyfile.LazyOpenStreamByFile(f),
			MatchStatus: tc.matchType,
		},
		MatchedFileInfos: matchedFileInfos,
	})
	// for _, pkg := range pkgs {
	// 	fmt.Printf("%s\n", pkg)
	// }

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
		if slices.CompareFunc(pkgs[i].License, tc.wantPkgs[i].License, strings.Compare) != 0 {
			t.Fatalf("%s: pkgs %d(%s) license error: %v(got) != %v(want)", tc.name, i, pkgs[i].Name, pkgs[i].License, tc.wantPkgs[i].License)
		}
		if pkgs[i].Verification != tc.wantPkgs[i].Verification {
			t.Fatalf("%s: pkgs %d(%s) verfication error: %v(got) != %v(want)", tc.name, i, pkgs[i].Name, pkgs[i].Verification, tc.wantPkgs[i].Verification)
		}
	}
	fmt.Println("===============================")
}

// package
func TestRPM(t *testing.T) {
	// positive
	tc := testcase{
		name:      "positive",
		filePath:  "./testdata/rpm/rpmdb.sqlite",
		wantPkgs:  RPMWantPkgs,
		t:         t,
		a:         analyzer.NewRPMAnalyzer(),
		matchType: 1,
	}
	Run(tc)
}

func TestApk(t *testing.T) {
	// positive
	tc := testcase{
		name:     "positive",
		filePath: "./testdata/apk/apk",
		wantPkgs: APKWantPkgs,

		t:         t,
		a:         analyzer.NewApkAnalyzer(),
		matchType: 1,
	}
	Run(tc)

	// negative
	tc = testcase{
		name:      "negative",
		filePath:  "./testdata/apk/negative-apk",
		wantPkgs:  APKNegativePkgs,
		t:         t,
		a:         analyzer.NewApkAnalyzer(),
		matchType: 1,
	}
	Run(tc)
}

func TestDpkg(t *testing.T) {
	// positive
	a := analyzer.NewDpkgAnalyzer()
	tc := testcase{
		name:      "positive",
		filePath:  "./testdata/dpkg/dpkg",
		t:         t,
		a:         a,
		matchType: 1,
		wantPkgs:  DPKGWantPkgs,
	}
	Run(tc)

	// negative
	tc = testcase{
		name:      "negative",
		filePath:  "./testdata/dpkg/negative-dpkg",
		t:         t,
		a:         a,
		matchType: 1,
		wantPkgs:  []dxtypes.Package{},
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
		a:         analyzer.NewConanAnalyzer(),
		matchType: 1,
		wantPkgs:  ConanWantPkgs,
	}
	Run(tc)

	// negative
	tc = testcase{
		name:      "negative",
		filePath:  "./testdata/conan/negative-conan",
		t:         t,
		a:         analyzer.NewConanAnalyzer(),
		matchType: 1,
		wantPkgs:  []dxtypes.Package{},
	}
	Run(tc)
}

func TestGoBinary(t *testing.T) {
	// positive
	tc := testcase{
		name:      "positive",
		filePath:  "./testdata/go_binary/go-binary",
		t:         t,
		a:         analyzer.NewGoBinaryAnalyzer(),
		matchType: 1,
		wantPkgs:  GOBianryWantPkgs,
	}
	Run(tc)

	// negative broken elf
	tc = testcase{
		name:      "negative-broken-elf",
		filePath:  "./testdata/go_binary/negative-go-binary-broken_elf",
		t:         t,
		a:         analyzer.NewGoBinaryAnalyzer(),
		matchType: 1,
		wantPkgs:  []dxtypes.Package{},
	}
	Run(tc)

	// negative bash
	tc = testcase{
		name:      "negative-bash",
		filePath:  "./testdata/go_binary/negative-go-binary-bash",
		t:         t,
		a:         analyzer.NewGoBinaryAnalyzer(),
		matchType: 1,
		wantPkgs:  []dxtypes.Package{},
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
		a:           analyzer.NewGoModAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/test/go.sum": "./testdata/go_mod/positive/sum",
		},
		wantPkgs: GoModWantPkgs,
	}
	Run(tc)

	// postivie less than golang 1.17, nedd parse go.sum
	tc = testcase{
		name:        "positive-less-than-117",
		filePath:    "./testdata/go_mod/lessthan117/mod",
		virtualPath: "/test/go.mod",
		t:           t,
		a:           analyzer.NewGoModAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/test/go.sum": "./testdata/go_mod/lessthan117/sum",
		},
		wantPkgs: GoModLess117Pkgs,
	}
	Run(tc)

	// negative
	tc = testcase{
		name:        "negative-wrongmod",
		filePath:    "./testdata/go_mod/negative/mod",
		virtualPath: "/test/go.mod",
		t:           t,
		a:           analyzer.NewGoModAnalyzer(),
		matchType:   1,
		wantPkgs:    []dxtypes.Package{},
		wantError:   true,
	}
	Run(tc)
}

func TestPHPComposer(t *testing.T) {
	tc := testcase{
		name:        "positive",
		filePath:    "./testdata/php_composer/positive/composer.lock",
		virtualPath: "/test/composer.lock",
		t:           t,
		a:           analyzer.NewPHPComposerAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/test/composer.json": "./testdata/php_composer/positive/composer.json",
		},
		wantPkgs: PHPComposerPkgs,
	}
	Run(tc)

	// json error
	tc = testcase{
		name:        "negative-wrongjson",
		filePath:    "./testdata/php_composer/negative/composer.lock",
		virtualPath: "/test/composer.lock",
		t:           t,
		a:           analyzer.NewPHPComposerAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/test/composer.json": "./testdata/php_composer/wrong.json",
		},
		wantPkgs: PHPComposerWrongJsonPkgs,
	}
	Run(tc)

	// no json file
	tc = testcase{
		name:           "negative-nojson",
		filePath:       "./testdata/php_composer/negative/composer.lock",
		virtualPath:    "/test/composer.lock",
		t:              t,
		a:              analyzer.NewPHPComposerAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       PHPComposerNoJsonPkgs,
	}
	Run(tc)

	// lock error
	tc = testcase{
		name:           "wronglock",
		filePath:       "./testdata/php_composer/wrong.json",
		virtualPath:    "/test/composer.lock",
		t:              t,
		a:              analyzer.NewPHPComposerAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       []dxtypes.Package{},
		wantError:      true,
	}
	Run(tc)
}

func TestPythonPackaging(t *testing.T) {
	// positive
	tc := testcase{
		name:           "positive-egg-zip",
		filePath:       "./testdata/python_packaging/egg/kitchen-1.2.6-py2.7.egg",
		t:              t,
		a:              analyzer.NewPythonPackagingAnalyzer(),
		matchType:      2,
		matchedFileMap: map[string]string{},
		wantPkgs:       PythonPackagingPkgs,
	}
	Run(tc)

	tc = testcase{
		name:           "positive-egg-info",
		filePath:       "./testdata/python_packaging/egg-info/PKG-INFO",
		t:              t,
		a:              analyzer.NewPythonPackagingAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       PythonPackagingEggPkg,
	}
	Run(tc)

	tc = testcase{
		name:           "positive-wheel",
		filePath:       "./testdata/python_packaging/dist-info/METADATA",
		t:              t,
		a:              analyzer.NewPythonPackagingAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       PythonPackagingWheel,
	}
	Run(tc)

	tc = testcase{
		name:           "positive-no-required-files",
		filePath:       "./testdata/python_packaging/egg/no-required-files.egg",
		t:              t,
		a:              analyzer.NewPythonPackagingAnalyzer(),
		matchType:      2,
		matchedFileMap: map[string]string{},
		wantPkgs:       []dxtypes.Package{},
	}
	Run(tc)
}

func TestPythonPIP(t *testing.T) {
	// positive
	tc := testcase{
		name:           "positive",
		filePath:       "./testdata/python_pip/requirements.txt",
		t:              t,
		a:              analyzer.NewPythonPIPAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       PythonPIPPkgs,
	}
	Run(tc)

	tc = testcase{
		name:           "positive-empty",
		filePath:       "./testdata/python_pip/empty.txt",
		t:              t,
		a:              analyzer.NewPythonPIPAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       []dxtypes.Package{},
	}
	Run(tc)
}

func TestPythonPIPEnv(t *testing.T) {
	// positive
	tc := testcase{
		name:           "positive",
		filePath:       "./testdata/python_pipenv/Pipfile.lock",
		t:              t,
		a:              analyzer.NewPythonPIPEnvAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       PythonPIPEnvPkgs,
	}
	Run(tc)

	tc = testcase{
		name:           "positive-empty",
		filePath:       "./testdata/python_pipenv/empty.lock",
		t:              t,
		a:              analyzer.NewPythonPIPEnvAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       []dxtypes.Package{},
		wantError:      true,
	}
	Run(tc)
}

func TestPythonPoetry(t *testing.T) {
	// positive
	tc := testcase{
		name:        "positive",
		filePath:    "./testdata/python_poetry/positive/poetry.lock",
		virtualPath: "/poetry.lock",
		t:           t,
		a:           analyzer.NewPythonPoetryAnalyzer(),
		matchType:   1,
		matchedFileMap: map[string]string{
			"/pyproject.toml": "./testdata/python_poetry/positive/pyproject.toml",
		},
		wantPkgs: PythonPoetryPkgs,
	}
	Run(tc)

	tc = testcase{
		name:           "positive-nopyproject",
		filePath:       "./testdata/python_poetry/positive-nopyproject/poetry.lock",
		virtualPath:    "/poetry.lock",
		t:              t,
		a:              analyzer.NewPythonPoetryAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       PythonPoetryNoProjectPkgs,
	}
	Run(tc)

	tc = testcase{
		name:           "negative",
		filePath:       "./testdata/python_poetry/negative/poetry.lock",
		virtualPath:    "/poetry.lock",
		t:              t,
		a:              analyzer.NewPythonPoetryAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       []dxtypes.Package{},
		wantError:      true,
	}
	Run(tc)

	tc = testcase{
		name:           "negative-wrong-project",
		filePath:       "./testdata/python_poetry/negative-wrong-project/poetry.lock",
		virtualPath:    "/poetry.lock",
		t:              t,
		a:              analyzer.NewPythonPoetryAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       PythonPoetryWrongProjectPkgs,
	}
	Run(tc)
}

func TestJavaGradle(t *testing.T) {
	// positive
	tc := testcase{
		name:           "positive",
		filePath:       "./testdata/java_gradle/positive.lockfile",
		virtualPath:    "/test/gradle.lockfile",
		t:              t,
		a:              analyzer.NewJavaGradleAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       JavaGradlePkgs,
	}
	Run(tc)
	tc = testcase{
		name:        "negative",
		filePath:    "./testdata/java_gradle/negative.lockfile",
		virtualPath: "/test/gradle.lockfile",
		t:           t,
		a:           analyzer.NewJavaGradleAnalyzer(),
		matchType:   1,
		wantPkgs:    []dxtypes.Package{},
	}
	Run(tc)
}

func TestJavaPom(t *testing.T) {
	tc := testcase{
		name:           "positive",
		filePath:       "./testdata/java_pom/positive/pom.xml",
		virtualPath:    "/test/pom.xml",
		t:              t,
		a:              analyzer.NewJavaPomAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       JavaPomPkgs,
	}
	Run(tc)

	tc = testcase{
		name:           "positive-requirement",
		filePath:       "./testdata/java_pom/requirements/pom.xml",
		virtualPath:    "/test/pom.xml",
		t:              t,
		a:              analyzer.NewJavaPomAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       JavaPomRequirementPkgs,
	}
	Run(tc)

	tc = testcase{
		name:           "negative",
		filePath:       "./testdata/java_pom/negative/pom.xml",
		virtualPath:    "/test/pom.xml",
		t:              t,
		a:              analyzer.NewJavaPomAnalyzer(),
		matchType:      1,
		matchedFileMap: map[string]string{},
		wantPkgs:       []dxtypes.Package{},
		wantError:      true,
	}
	Run(tc)
}

func TestFilterAnalyzer(t *testing.T) {
	wantPkgAnalyzerTypes := []string{
		reflect.TypeOf(analyzer.NewRPMAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewDpkgAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewApkAnalyzer()).String(),
	}
	wantLangAnalyzerTypes := []string{
		reflect.TypeOf(analyzer.NewConanAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewGoBinaryAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewGoModAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewPHPComposerAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewJavaGradleAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewJavaPomAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewPythonPIPAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewPythonPackagingAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewPythonPIPEnvAnalyzer()).String(),
		reflect.TypeOf(analyzer.NewPythonPoetryAnalyzer()).String(),
	}

	wantAnalyzerTypes := []string{}
	wantAnalyzerTypes = append(wantAnalyzerTypes, wantPkgAnalyzerTypes...)
	wantAnalyzerTypes = append(wantAnalyzerTypes, wantLangAnalyzerTypes...)

	testcases := []struct {
		scanMode          analyzer.ScanMode
		wantAnalyzerTypes []string
	}{
		{
			scanMode:          analyzer.AllMode,
			wantAnalyzerTypes: wantAnalyzerTypes,
		},
		{
			scanMode:          analyzer.AllMode | analyzer.PkgMode, // mean PkgMode
			wantAnalyzerTypes: wantPkgAnalyzerTypes,
		},
		{
			scanMode:          analyzer.PkgMode,
			wantAnalyzerTypes: wantPkgAnalyzerTypes,
		},
		{
			scanMode:          analyzer.LanguageMode,
			wantAnalyzerTypes: wantLangAnalyzerTypes,
		},
	}

	for _, testcase := range testcases {
		wantTypes := testcase.wantAnalyzerTypes
		got := analyzer.FilterAnalyzer(testcase.scanMode)
		gotTypes := lo.Map(got, func(a analyzer.Analyzer, _ int) string {
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
