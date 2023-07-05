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
		t.Fatalf("pkgs length[%d] error: %#v", len(pkgs), pkgs)
	}

	for i := 0; i < len(pkgs); i++ {
		if pkgs[i].Name != tc.wantPkgs[i].Name {
			t.Fatalf("pkgs error name at (%v) vs want: (%v)\npkgs: %#v", pkgs[i], tc.wantPkgs[i], pkgs)
		}
		if pkgs[i].Version != tc.wantPkgs[i].Version {
			t.Fatalf("pkgs error version at (%v) vs want: (%v)\npkgs: %#v", pkgs[i], tc.wantPkgs[i], pkgs)
		}
	}
}

func TestApk(t *testing.T) {
	tc := testcase{
		filePath: "./testdata/apk",
		wantPkgs: []types.Package{
			{
				Name:    "alpine-baselayout",
				Version: "3.4.3-r1",
			},
			{
				Name:    "alpine-baselayout-data",
				Version: "3.4.3-r1",
			},
			{
				Name:    "alpine-keys",
				Version: "2.4-r1",
			},
			{
				Name:    "apk-tools",
				Version: "2.14.0-r2",
			},
			{
				Name:    "busybox",
				Version: "1.36.1-r0",
			},
			{
				Name:    "busybox-binsh",
				Version: "1.36.1-r0",
			},
			{
				Name:    "ca-certificates-bundle",
				Version: "20230506-r0",
			},
			{
				Name:    "libc-utils",
				Version: "0.7.2-r5",
			},
			{
				Name:    "libcrypto3",
				Version: "3.1.1-r1",
			},
			{
				Name:    "libssl3",
				Version: "3.1.1-r1",
			},
			{
				Name:    "musl",
				Version: "1.2.4-r0",
			},
			{
				Name:    "musl-utils",
				Version: "1.2.4-r0",
			},
			{
				Name:    "scanelf",
				Version: "1.3.7-r1",
			},
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
