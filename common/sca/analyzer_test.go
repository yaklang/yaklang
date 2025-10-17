package sca

import (
	"embed"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/lazyfile"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"golang.org/x/exp/slices"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/samber/lo"
)

//go:embed testdata
var testFS embed.FS

type testcase struct {
	name           string
	filePath       string
	virtualPath    string
	wantPkgs       []*dxtypes.Package
	wantError      bool
	skipCheck      bool
	t              *testing.T
	a              analyzer.Analyzer
	matchType      int
	matchedFileMap map[string]string
}

func CreateTempFromFsFile(path string) (*os.File, error) {
	// remove ./ prefix
	if strings.HasPrefix(path, "./") {
		path = path[2:]
	}

	tempFile, err := os.CreateTemp("", "test")
	if err != nil {
		return nil, err
	}

	f, err := testFS.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := io.Copy(tempFile, f); err != nil {
		return nil, err
	}

	return tempFile, nil
}

func Check(pkgs, wantPkgs []*dxtypes.Package, name string, t *testing.T) {
	sort.Slice(pkgs, func(i, j int) bool {
		c := strings.Compare(pkgs[i].Name, pkgs[j].Name)
		if c == 0 {
			return strings.Compare(pkgs[i].Version, pkgs[j].Version) > 0
		}
		return c > 0
	})
	sort.Slice(wantPkgs, func(i, j int) bool {
		c := strings.Compare(wantPkgs[i].Name, wantPkgs[j].Name)
		if c == 0 {
			return strings.Compare(wantPkgs[i].Version, wantPkgs[j].Version) > 0
		}
		return c > 0
	})

	if len(pkgs) != len(wantPkgs) {
		t.Fatalf("%s: pkgs length error: %d(got) != %d(want)", name, len(pkgs), len(wantPkgs))
	}

	for i := 0; i < len(pkgs); i++ {
		if strings.Contains(pkgs[i].Name, "|") {
			pkgNames := strings.Split(pkgs[i].Name, "|")
			wantPkgNames := strings.Split(wantPkgs[i].Name, "|")
			sort.Strings(pkgNames)
			sort.Strings(wantPkgNames)
			if slices.CompareFunc(pkgNames, wantPkgNames, strings.Compare) != 0 {
				t.Fatalf("%s: pkgs %d name error: %#v(got) != %#v(want)", name, i, pkgNames, wantPkgNames)
			}
		} else if pkgs[i].Name != wantPkgs[i].Name {
			t.Fatalf("%s: pkgs %d name error: %s(got) != %s(want)", name, i, pkgs[i].Name, wantPkgs[i].Name)
		}
		if strings.Contains(pkgs[i].Version, "|") {
			pkgVersions := strings.Split(pkgs[i].Version, "|")
			wantPkgVersions := strings.Split(wantPkgs[i].Version, "|")
			sort.Strings(pkgVersions)
			sort.Strings(wantPkgVersions)
			if slices.CompareFunc(pkgVersions, wantPkgVersions, strings.Compare) != 0 {
				t.Fatalf("%s: pkgs %d version error: %#v(got) != %#v(want)", name, i, pkgVersions, wantPkgVersions)
			}
		} else if pkgs[i].Version != wantPkgs[i].Version {
			t.Fatalf("%s: pkgs %d(%s) version error: %s(got) != %s(want)", name, i, pkgs[i].Name, pkgs[i].Version, wantPkgs[i].Version)
		}

		if slices.CompareFunc(pkgs[i].License, wantPkgs[i].License, strings.Compare) != 0 {
			t.Fatalf("%s: pkgs %d(%s) license error: %v(got) != %v(want)", name, i, pkgs[i].Name, pkgs[i].License, wantPkgs[i].License)
		}
		if pkgs[i].Verification != wantPkgs[i].Verification {
			t.Fatalf("%s: pkgs %d(%s) verfication error: %v(got) != %v(want)", name, i, pkgs[i].Name, pkgs[i].Verification, wantPkgs[i].Verification)
		}
	}
}

// CheckContains 验证期望的包是否都包含在实际结果中（用于处理传递依赖的情况）
func CheckContains(pkgs, wantPkgs []*dxtypes.Package, name string, t *testing.T) {
	// 创建实际包的映射，方便查找
	actualPkgs := make(map[string]*dxtypes.Package)
	for _, pkg := range pkgs {
		key := pkg.Name + "@" + pkg.Version
		actualPkgs[key] = pkg
	}

	// 检查每个期望的包是否在实际结果中
	for _, wantPkg := range wantPkgs {
		key := wantPkg.Name + "@" + wantPkg.Version
		actualPkg, found := actualPkgs[key]
		if !found {
			t.Fatalf("%s: expected package not found: %s@%s", name, wantPkg.Name, wantPkg.Version)
		}

		// 验证许可证 - 只有当期望的许可证不为空时才验证
		if len(wantPkg.License) > 0 {
			if slices.CompareFunc(actualPkg.License, wantPkg.License, strings.Compare) != 0 {
				t.Fatalf("%s: pkg %s license error: %v(got) != %v(want)", name, wantPkg.Name, actualPkg.License, wantPkg.License)
			}
		}

		// 验证验证状态 - 只有当期望的验证状态不为空时才验证
		if wantPkg.Verification != "" {
			if actualPkg.Verification != wantPkg.Verification {
				t.Fatalf("%s: pkg %s verification error: %v(got) != %v(want)", name, wantPkg.Name, actualPkg.Verification, wantPkg.Verification)
			}
		}
	}

	t.Logf("%s: Successfully found all %d expected packages in %d total packages", name, len(wantPkgs), len(pkgs))
}

func Run(tc testcase) []*dxtypes.Package {
	t := tc.t
	fmt.Printf("TestCase: %s\n===============================\n", tc.name)

	f, err := CreateTempFromFsFile(tc.filePath)
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()
	if err != nil {
		t.Fatalf("%s: con't open file: %v", err, tc.name)
	}
	fs := filesys.NewLocalFs()

	matchedFileInfos := lo.MapEntries(tc.matchedFileMap, func(k, v string) (string, *analyzer.FileInfo) {
		f, err := CreateTempFromFsFile(v)
		if err != nil {
			t.Fatalf("%s: con't open file: %v", err, tc.name)
		}
		fi := &analyzer.FileInfo{
			Path:        k,
			Analyzer:    tc.a,
			LazyFile:    lazyfile.LazyOpenStreamByFile(nil, f),
			MatchStatus: tc.matchType,
		}
		analyzer.SetFileInfoFileSystem(fi, fs)
		return k, fi
	})
	defer func() {
		for _, fi := range matchedFileInfos {
			f := fi.LazyFile
			f.Close()
			os.Remove(f.Name())
		}
	}()

	fi := &analyzer.FileInfo{
		Path:        tc.virtualPath,
		Analyzer:    tc.a,
		LazyFile:    lazyfile.LazyOpenStreamByFile(nil, f),
		MatchStatus: tc.matchType,
	}
	analyzer.SetFileInfoFileSystem(fi, fs)
	pkgs, err := tc.a.Analyze(analyzer.AnalyzeFileInfo{
		Self:             fi,
		MatchedFileInfos: matchedFileInfos,
	})
	pkgs = analyzer.MergePackages(pkgs)
	// showPkgs(pkgs)

	// for _, pkg := range pkgs {
	// 	fmt.Printf("%s\n", pkg)
	// }

	if tc.wantError && err == nil {
		t.Fatalf("%s: want error but nil", tc.name)
	}
	if !tc.wantError && err != nil {
		t.Fatalf("%s: analyze error: %v", tc.name, err)
	}

	if !tc.skipCheck {
		Check(pkgs, tc.wantPkgs, t.Name(), tc.t)
	}

	fmt.Println("===============================")
	return pkgs
}

// package
func TestRPM(t *testing.T) {
	// positive
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:      "positive",
			filePath:  "./testdata/rpm/rpmdb.sqlite",
			wantPkgs:  RPMWantPkgs,
			t:         t,
			a:         analyzer.NewRPMAnalyzer(),
			matchType: 1,
		}
		Run(tc)
	})
}

func TestApk(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:     "positive",
			filePath: "./testdata/apk/apk",
			wantPkgs: APKWantPkgs,

			t:         t,
			a:         analyzer.NewApkAnalyzer(),
			matchType: 1,
		}
		Run(tc)
	})

	t.Run("negative", func(t *testing.T) {
		tc := testcase{
			name:      "negative",
			filePath:  "./testdata/apk/negative-apk",
			wantPkgs:  APKNegativePkgs,
			t:         t,
			a:         analyzer.NewApkAnalyzer(),
			matchType: 1,
		}
		Run(tc)
	})
}

func TestDpkg(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
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
	})

	t.Run("negative", func(t *testing.T) {
		a := analyzer.NewDpkgAnalyzer()
		tc := testcase{
			name:      "negative",
			filePath:  "./testdata/dpkg/negative-dpkg",
			t:         t,
			a:         a,
			matchType: 1,
			wantPkgs:  []*dxtypes.Package{},
		}
		Run(tc)
	})
}

// language
func TestConan(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:      "positive",
			filePath:  "./testdata/conan/conan",
			t:         t,
			a:         analyzer.NewConanAnalyzer(),
			matchType: 1,
			wantPkgs:  ConanWantPkgs,
		}
		Run(tc)
	})

	t.Run("negative", func(t *testing.T) {
		tc := testcase{
			name:      "negative",
			filePath:  "./testdata/conan/negative-conan",
			t:         t,
			a:         analyzer.NewConanAnalyzer(),
			matchType: 1,
			wantPkgs:  []*dxtypes.Package{},
		}
		Run(tc)
	})
}

func TestGoBinary(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:      "positive",
			filePath:  "./testdata/go_binary/go-binary",
			t:         t,
			a:         analyzer.NewGoBinaryAnalyzer(),
			matchType: 1,
			wantPkgs:  GOBianryWantPkgs,
		}
		Run(tc)
	})

	t.Run("negative-broken-elf", func(t *testing.T) {
		tc := testcase{
			name:      "negative-broken-elf",
			filePath:  "./testdata/go_binary/negative-go-binary-broken_elf",
			t:         t,
			a:         analyzer.NewGoBinaryAnalyzer(),
			matchType: 1,
			wantPkgs:  []*dxtypes.Package{},
		}
		Run(tc)
	})

	t.Run("negative-bash", func(t *testing.T) {
		tc := testcase{
			name:      "negative-bash",
			filePath:  "./testdata/go_binary/negative-go-binary-bash",
			t:         t,
			a:         analyzer.NewGoBinaryAnalyzer(),
			matchType: 1,
			wantPkgs:  []*dxtypes.Package{},
		}
		Run(tc)
	})
}

func TestGoMod(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
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
	})

	t.Run("positive-less-than-117", func(t *testing.T) {
		tc := testcase{
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
	})

	t.Run("negative-wrongmod", func(t *testing.T) {
		tc := testcase{
			name:        "negative-wrongmod",
			filePath:    "./testdata/go_mod/negative/mod",
			virtualPath: "/test/go.mod",
			t:           t,
			a:           analyzer.NewGoModAnalyzer(),
			matchType:   1,
			wantPkgs:    []*dxtypes.Package{},
			wantError:   true,
		}
		Run(tc)
	})
}

func TestPHPComposer(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
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
	})

	t.Run("negative-wrongjson", func(t *testing.T) {
		tc := testcase{
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
	})

	t.Run("negative-nojson", func(t *testing.T) {
		tc := testcase{
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
	})

	t.Run("wronglock", func(t *testing.T) {
		tc := testcase{
			name:           "wronglock",
			filePath:       "./testdata/php_composer/wrong.json",
			virtualPath:    "/test/composer.lock",
			t:              t,
			a:              analyzer.NewPHPComposerAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       []*dxtypes.Package{},
			wantError:      true,
		}
		Run(tc)
	})
}

func TestPythonPackaging(t *testing.T) {
	t.Run("positive-egg-zip", func(t *testing.T) {
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
	})

	t.Run("positive-egg-info", func(t *testing.T) {
		tc := testcase{
			name:           "positive-egg-info",
			filePath:       "./testdata/python_packaging/egg-info/PKG-INFO",
			t:              t,
			a:              analyzer.NewPythonPackagingAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       PythonPackagingEggPkg,
		}
		Run(tc)
	})

	t.Run("positive-wheel", func(t *testing.T) {
		tc := testcase{
			name:           "positive-wheel",
			filePath:       "./testdata/python_packaging/dist-info/METADATA",
			t:              t,
			a:              analyzer.NewPythonPackagingAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       PythonPackagingWheel,
		}
		Run(tc)
	})

	t.Run("positive-no-required-files", func(t *testing.T) {
		tc := testcase{
			name:           "positive-no-required-files",
			filePath:       "./testdata/python_packaging/egg/no-required-files.egg",
			t:              t,
			a:              analyzer.NewPythonPackagingAnalyzer(),
			matchType:      2,
			matchedFileMap: map[string]string{},
			wantPkgs:       []*dxtypes.Package{},
		}
		Run(tc)
	})
}

func TestPythonPIP(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
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
	})

	t.Run("positive-empty", func(t *testing.T) {
		tc := testcase{
			name:           "positive-empty",
			filePath:       "./testdata/python_pip/empty.txt",
			t:              t,
			a:              analyzer.NewPythonPIPAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       []*dxtypes.Package{},
		}
		Run(tc)
	})
}

func TestPythonPIPEnv(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
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
	})

	t.Run("positive-empty", func(t *testing.T) {
		tc := testcase{
			name:           "positive-empty",
			filePath:       "./testdata/python_pipenv/empty.lock",
			t:              t,
			a:              analyzer.NewPythonPIPEnvAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       []*dxtypes.Package{},
			wantError:      true,
		}
		Run(tc)
	})
}

func TestPythonPoetry(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
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
	})

	t.Run("positive-nopyproject", func(t *testing.T) {
		tc := testcase{
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
	})

	t.Run("negative", func(t *testing.T) {
		tc := testcase{
			name:           "negative",
			filePath:       "./testdata/python_poetry/negative/poetry.lock",
			virtualPath:    "/poetry.lock",
			t:              t,
			a:              analyzer.NewPythonPoetryAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       []*dxtypes.Package{},
			wantError:      true,
		}
		Run(tc)
	})

	t.Run("negative-wrong-project", func(t *testing.T) {
		tc := testcase{
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
	})
}

func TestJavaJar(t *testing.T) {
	t.Run("positive-war", func(t *testing.T) {
		tc := testcase{
			name:           "positive-war",
			filePath:       "./testdata/java_jar/positive/test.war",
			t:              t,
			a:              analyzer.NewJavaJarAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       JavaJarWarPkgs,
		}
		Run(tc)
	})

	t.Run("positive-par", func(t *testing.T) {
		tc := testcase{
			name:           "positive-par",
			filePath:       "./testdata/java_jar/positive/test.par",
			t:              t,
			a:              analyzer.NewJavaJarAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       JavaJarParPkgs,
		}
		Run(tc)
	})

	t.Run("positive-jar", func(t *testing.T) {
		tc := testcase{
			name:           "positive-jar",
			filePath:       "./testdata/java_jar/positive/test.jar",
			t:              t,
			a:              analyzer.NewJavaJarAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       JavaJarJarPkgs,
		}
		Run(tc)
	})

	t.Run("negative-broken-jar", func(t *testing.T) {
		tc := testcase{
			name:           "negative-broken-jar",
			filePath:       "./testdata/java_jar/negative/test.txt",
			t:              t,
			a:              analyzer.NewPythonPIPEnvAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       []*dxtypes.Package{},
			wantError:      true,
		}
		Run(tc)
	})
}

func TestJavaGradle(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
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
	})

	t.Run("negative", func(t *testing.T) {
		tc := testcase{
			name:        "negative",
			filePath:    "./testdata/java_gradle/negative.lockfile",
			virtualPath: "/test/gradle.lockfile",
			t:           t,
			a:           analyzer.NewJavaGradleAnalyzer(),
			matchType:   1,
			wantPkgs:    []*dxtypes.Package{},
		}
		Run(tc)
	})
}

func TestJavaPom(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
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
	})

	t.Run("positive2", func(t *testing.T) {
		tc := testcase{
			name:           "positive",
			filePath:       "./testdata/java_pom/positive2/pom.xml",
			virtualPath:    "/test/pom.xml",
			t:              t,
			a:              analyzer.NewJavaPomAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       JavaPom2Pkgs,
			skipCheck:      true, // 跳过默认检查，使用自定义检查
		}
		pkgs := Run(tc)
		// 使用CheckContains检查期望的包是否都包含在实际结果中（处理传递依赖）
		CheckContains(pkgs, JavaPom2Pkgs, t.Name(), t)
	})

	t.Run("positive-requirement", func(t *testing.T) {
		tc := testcase{
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
	})

	t.Run("negative", func(t *testing.T) {
		tc := testcase{
			name:           "negative",
			filePath:       "./testdata/java_pom/negative/pom.xml",
			virtualPath:    "/test/pom.xml",
			t:              t,
			a:              analyzer.NewJavaPomAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       []*dxtypes.Package{},
			wantError:      true,
		}
		Run(tc)
	})
}

func TestNodeNpm(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:        "positive",
			filePath:    "./testdata/node_npm/positive_file/package.json",
			virtualPath: "/test/package.json",
			t:           t,
			a:           analyzer.NewNodeNpmAnalyzer(),
			matchType:   1,
			wantPkgs:    NodeNpmPkgs,
		}
		Run(tc)
	})
	// folder

	t.Run("positive-folder", func(t *testing.T) {
		tc := testcase{
			name:      "positive-folder",
			t:         t,
			a:         analyzer.NewNodeNpmAnalyzer(),
			skipCheck: true,
		}
		pkgs := make([]*dxtypes.Package, 0)
		{
			tc.filePath = "./testdata/node_npm/positive_folder/package-lock.json"
			tc.matchType = 2
			pkgs = append(pkgs, Run(tc)...)

			tc.filePath = "./testdata/node_npm/positive_folder/package.json"
			tc.matchType = 1
			pkgs = append(pkgs, Run(tc)...)

			tc.filePath = "./testdata/node_npm/positive_folder/test_node_modules/ms/package.json"
			tc.matchType = 1
			pkgs = append(pkgs, Run(tc)...)

			tc.filePath = "./testdata/node_npm/positive_folder/test_node_modules/express/package.json"
			tc.matchType = 1
			pkgs = append(pkgs, Run(tc)...)

			tc.filePath = "./testdata/node_npm/positive_folder/test_node_modules/express/test_node_modules/debug/package.json"
			tc.matchType = 1
			pkgs = append(pkgs, Run(tc)...)

			tc.filePath = "./testdata/node_npm/positive_folder/test_node_modules/express/test_node_modules/ms/package.json"
			tc.matchType = 1
			pkgs = append(pkgs, Run(tc)...)

			tc.filePath = "./testdata/node_npm/positive_folder/test_node_modules/body-parser/package.json"
			tc.matchType = 1
			pkgs = append(pkgs, Run(tc)...)

			tc.filePath = "./testdata/node_npm/positive_folder/test_node_modules/body-parser/test_node_modules/debug/package.json"
			tc.matchType = 1
			pkgs = append(pkgs, Run(tc)...)

			tc.filePath = "./testdata/node_npm/positive_folder/test_node_modules/body-parser/test_node_modules/ms/package.json"
			tc.matchType = 1
			pkgs = append(pkgs, Run(tc)...)

		}
		if len(pkgs) != 62 {
			t.Fatalf("%s: package length error: %d(get)", tc.name, len(pkgs))
		}
		// fmt.Println("before: ", len(pkgs))
		// analyzer.DrawPackagesDOT(pkgs)
		ret := analyzer.MergePackages(pkgs)
		// fmt.Println("after: ", len(ret))
		// showPkgs(ret)
		Check(ret, NodeNpmPkgsFolder, tc.name, t)
		// analyzer.DrawPackagesDOT(ret)
	})
}

func TestNodePnpm(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:        "positive",
			filePath:    "./testdata/node_pnpm/pnpm-lock.yaml",
			virtualPath: "/test/pnpm-lock.yaml",
			t:           t,
			a:           analyzer.NewNodePnpmAnalyzer(),
			matchType:   1,
			wantPkgs:    NodePnpmPkgs,
		}
		Run(tc)
	})
}

func TestNodeYarn(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:        "positive",
			filePath:    "./testdata/node_yarn/positive/yarn.lock",
			virtualPath: "/test/yarn.lock",
			t:           t,
			a:           analyzer.NewNodeYarnAnalyzer(),
			matchType:   1,
			wantPkgs:    NodeYarnPkgs,
		}
		Run(tc)
	})

	t.Run("positive-protocol", func(t *testing.T) {
		tc := testcase{
			name:        "positive-protocol",
			filePath:    "./testdata/node_yarn/positive_protocol/yarn.lock",
			virtualPath: "/test/yarn.lock",
			t:           t,
			a:           analyzer.NewNodeYarnAnalyzer(),
			matchType:   1,
			wantPkgs:    NodeYarnProtocolPkgs,
		}
		Run(tc)
	})
}

func TestRubyBundler(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:           "positive",
			filePath:       "./testdata/ruby_bundler/positive/Gemfile.lock",
			virtualPath:    "/test/Gemfile.lock",
			t:              t,
			a:              analyzer.NewRubyBundlerAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       RubyBundlerPkgs,
		}
		Run(tc)
	})

	t.Run("negative", func(t *testing.T) {
		tc := testcase{
			name:           "negative",
			filePath:       "./testdata/ruby_bundler/negative/Gemfile.lock",
			virtualPath:    "/test/Gemfile.lock",
			t:              t,
			a:              analyzer.NewRubyBundlerAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       nil,
		}
		Run(tc)
	})
}

func TestRubyGemspec(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:           "positive",
			filePath:       "./testdata/ruby_gemspec/positive/multiple_licenses.gemspec",
			virtualPath:    "/test/multiple_licenses.gemspec",
			t:              t,
			a:              analyzer.NewRubyGemSpecAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       RubyGemspecPkgs,
		}
		Run(tc)
	})

	t.Run("negative", func(t *testing.T) {
		tc := testcase{
			name:           "negative",
			filePath:       "./testdata/ruby_gemspec/negative/empty_name.gemspec",
			virtualPath:    "/test/empty_name.gemspec",
			t:              t,
			a:              analyzer.NewRubyGemSpecAnalyzer(),
			matchType:      1,
			matchedFileMap: map[string]string{},
			wantPkgs:       nil,
			wantError:      true,
		}
		Run(tc)
	})
}

func TestRustCargo(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		tc := testcase{
			name:        "positive",
			filePath:    "./testdata/rust_cargo/positive/Cargo.lock",
			virtualPath: "/test/Cargo.lock",
			t:           t,
			a:           analyzer.NewRustCargoAnalyzer(),
			matchType:   1,
			wantPkgs:    RustCargoPkgs,
		}
		Run(tc)
	})
	t.Run("negative", func(t *testing.T) {
		tc := testcase{
			name:        "negative",
			filePath:    "./testdata/rust_cargo/negative/Cargo.lock",
			virtualPath: "/test/Cargo.lock",
			t:           t,
			a:           analyzer.NewRustCargoAnalyzer(),
			matchType:   1,
			wantPkgs:    []*dxtypes.Package{},
			wantError:   true,
		}
		Run(tc)
	})
}

func TestCustomAnalyzer(t *testing.T) {
	tc := testcase{
		name:        "positive",
		filePath:    "./testdata/go_mod/positive/mod",
		virtualPath: "/test/go.mod",
		t:           t,
		a: analyzer.NewCustomAnalyzer(
			func(info analyzer.MatchInfo) int {
				if strings.HasSuffix(info.Path, "go.mod") {
					return 1
				}
				return 0
			},
			func(fi *analyzer.FileInfo, otherFi map[string]*analyzer.FileInfo) []*analyzer.CustomPackage {
				return []*analyzer.CustomPackage{
					{
						Name:    "github.com/aquasecurity/go-dep-parser",
						Version: "0.0.0-20220406074731-71021a481237",
					},
					{
						Name:    "golang.org/x/xerrors",
						Version: "0.0.0-20200804184101-5ec99f83aff1",
					},
				}
			},
		),
		matchType: 1,
		wantPkgs:  GoModWantPkgs,
	}
	Run(tc)
}

func showPkgs(pkgs []*dxtypes.Package) {
	for _, p := range pkgs {
		license := "nil"
		if len(p.License) > 0 {
			license = fmt.Sprintf(`[]string{"%s"}`, strings.Join(p.License, `", "`))
		}
		fmt.Printf(`{
	Name:         "%s",
	Version:      "%s",
	Verification: "%s",
	License:      %s,
	Potential:    %t,
},
`, p.Name, p.Version, p.Verification, license, p.Potential)
	}
}

func TestFilterAnalyzer(t *testing.T) {
	getName := func(i interface{}) string {
		return reflect.TypeOf(i).String()
	}

	compare := func(got, wanted []string) {
		sort.Slice(wanted, func(i, j int) bool {
			return strings.Compare(wanted[i], wanted[j]) < 0
		})

		sort.Slice(got, func(i, j int) bool {
			return strings.Compare(got[i], got[j]) < 0
		})

		if len(got) != len(wanted) {
			t.Fatalf("analyzers length error: %d(got) != %d(want)", len(got), len(wanted))
		}
		if !reflect.DeepEqual(got, wanted) {
			t.Fatalf("analyzers error: %v(got) != %v(want)", got, wanted)
		}
	}

	wantPkgAnalyzerTypes := []string{
		getName(analyzer.NewRPMAnalyzer()),
		getName(analyzer.NewDpkgAnalyzer()),
		getName(analyzer.NewApkAnalyzer()),
	}

	wantLangAnalyzerTypes := []string{
		getName(analyzer.NewConanAnalyzer()),
		getName(analyzer.NewGoBinaryAnalyzer()),
		getName(analyzer.NewGoModAnalyzer()),
		getName(analyzer.NewPHPComposerAnalyzer()),
		getName(analyzer.NewJavaGradleAnalyzer()),
		getName(analyzer.NewJavaPomAnalyzer()),
		getName(analyzer.NewJavaJarAnalyzer()),
		getName(analyzer.NewPythonPIPAnalyzer()),
		getName(analyzer.NewPythonPackagingAnalyzer()),
		getName(analyzer.NewPythonPIPEnvAnalyzer()),
		getName(analyzer.NewPythonPoetryAnalyzer()),
		getName(analyzer.NewNodeNpmAnalyzer()),
		getName(analyzer.NewNodePnpmAnalyzer()),
		getName(analyzer.NewNodeYarnAnalyzer()),
		getName(analyzer.NewRubyBundlerAnalyzer()),
		getName(analyzer.NewRubyGemSpecAnalyzer()),
		getName(analyzer.NewRustCargoAnalyzer()),
	}

	t.Run("filter-by-mode", func(t *testing.T) {
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
			got := analyzer.FilterAnalyzer(testcase.scanMode, nil)
			gotTypes := lo.Map(got, func(a analyzer.Analyzer, _ int) string {
				return getName(a)
			})
			compare(gotTypes, wantTypes)

		}
	})

	t.Run("filter-by-analyzer-name", func(t *testing.T) {
		testcases := []struct {
			usedAnalayzers    []analyzer.TypAnalyzer
			wantAnalyzerTypes []string
		}{
			{
				usedAnalayzers:    []analyzer.TypAnalyzer{analyzer.TypRPM},
				wantAnalyzerTypes: []string{getName(analyzer.NewRPMAnalyzer())},
			},
			{
				usedAnalayzers:    []analyzer.TypAnalyzer{analyzer.TypRPM, analyzer.TypAPK},
				wantAnalyzerTypes: []string{getName(analyzer.NewRPMAnalyzer()), getName(analyzer.NewApkAnalyzer())},
			},
		}
		for _, testcase := range testcases {
			wantTypes := testcase.wantAnalyzerTypes
			got := analyzer.FilterAnalyzer(analyzer.AllMode, testcase.usedAnalayzers)
			gotTypes := lo.Map(got, func(a analyzer.Analyzer, _ int) string {
				return getName(a)
			})
			compare(gotTypes, wantTypes)
		}
	})

	t.Run("filter-by-analyzer-name-and-mode", func(t *testing.T) {
		wantAnalyzerTypes1 := make([]string, 0)
		wantAnalyzerTypes1 = append(wantAnalyzerTypes1, wantPkgAnalyzerTypes...)
		wantAnalyzerTypes1 = append(wantAnalyzerTypes1, getName(analyzer.NewConanAnalyzer()))

		wantAnalyzerTypes2 := make([]string, 0)
		wantAnalyzerTypes2 = append(wantAnalyzerTypes2, wantLangAnalyzerTypes...)
		wantAnalyzerTypes2 = append(wantAnalyzerTypes2, getName(analyzer.NewDpkgAnalyzer()))

		testcases := []struct {
			scanMode          analyzer.ScanMode
			usedAnalayzers    []analyzer.TypAnalyzer
			wantAnalyzerTypes []string
		}{
			{
				scanMode:          analyzer.PkgMode,
				usedAnalayzers:    []analyzer.TypAnalyzer{analyzer.TypClangConan},
				wantAnalyzerTypes: wantAnalyzerTypes1,
			},
			{
				scanMode:          analyzer.LanguageMode,
				usedAnalayzers:    []analyzer.TypAnalyzer{analyzer.TypDPKG},
				wantAnalyzerTypes: wantAnalyzerTypes2,
			},
		}
		for _, testcase := range testcases {
			wantTypes := testcase.wantAnalyzerTypes
			got := analyzer.FilterAnalyzer(testcase.scanMode, testcase.usedAnalayzers)
			gotTypes := lo.Map(got, func(a analyzer.Analyzer, _ int) string {
				return getName(a)
			})
			compare(gotTypes, wantTypes)
		}
	})
}
