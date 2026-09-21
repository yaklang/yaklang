package packaging_test

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/python/packaging"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParsePackagingOriginalMetadata(t *testing.T) {
	src, err := os.ReadFile("testdata/distlib-0.3.1.METADATA")
	if err != nil {
		t.Fatal(err)
	}
	libs, deps, err := packaging.NewParser().Parse(nil, strings.NewReader(string(src)))
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 1 || libs[0].Name != "distlib" || libs[0].Version != "0.3.1" {
		t.Fatalf("%+v", libs)
	}
	if libs[0].License != "Python license" {
		t.Fatalf("license %q", libs[0].License)
	}
	if libs[0].Source != "https://bitbucket.org/pypa/distlib" {
		t.Fatalf("home-page source %q", libs[0].Source)
	}
	if libs[0].Locations == nil || libs[0].Locations[0].StartLine != 1 || libs[0].Locations[0].EndLine < 10 {
		t.Fatalf("location %+v", libs[0].Locations)
	}
	if len(deps) != 0 {
		t.Fatalf("distlib METADATA has no Requires-Dist, got %+v", deps)
	}
}

func TestParsePackagingRequiresDistExtrasAndMarkers(t *testing.T) {
	src, err := os.ReadFile("testdata/zipp-3.12.1.METADATA")
	if err != nil {
		t.Fatal(err)
	}
	libs, deps, err := packaging.NewParser().Parse(nil, strings.NewReader(string(src)))
	if err != nil {
		t.Fatal(err)
	}
	if libs[0].Name != "zipp" || libs[0].Source != "https://github.com/jaraco/zipp" {
		t.Fatalf("%+v", libs)
	}
	if libs[0].License != "MIT License" {
		t.Fatalf("classifier license %q", libs[0].License)
	}
	got := map[string]types.Requirement{}
	var extrasDocs, extrasTesting, markerCombo bool
	for _, q := range deps[0].Requirements {
		got[q.Target+"|"+q.Constraint+"|"+q.Condition] = q
		if q.Target == "sphinx" && q.Constraint != ">=3.5" {
			t.Fatalf("original range lost: %+v", q)
		}
		if q.Target == "sphinx" && q.Condition != "extra == 'docs'" {
			t.Fatalf("docs extra marker lost: %+v", q)
		}
		if q.Target == "sphinx" {
			extrasDocs = true
		}
		if q.Target == "pytest" && q.Condition == "extra == 'testing'" {
			extrasTesting = true
		}
		if q.Target == "pytest-black" && q.Constraint == ">=0.3.7" &&
			q.Condition == `(platform_python_implementation != "PyPy") and extra == 'testing'` {
			markerCombo = true
		}
		if q.Resolved != "" {
			t.Fatalf("METADATA must not invent resolved: %+v", q)
		}
	}
	if !extrasDocs || !extrasTesting || !markerCombo {
		t.Fatalf("extras/markers dropped: %+v", deps[0].Requirements)
	}
	if _, ok := got["zipp|"+libs[0].Version+"|"]; ok {
		t.Fatal("locked/self version used as requirement")
	}
}

func TestParsePackagingNameExtrasAndEmptyRange(t *testing.T) {
	src := "Name: app\nVersion: 1.0\nHome-page: https://example.test/app\n" +
		"Requires-Dist: requests[security]>=2.0,<3; python_version < \"3.12\"\n" +
		"Requires-Dist: localpkg\n" +
		"Requires-Dist: demo @ https://example.test/demo.whl\n" +
		"Provides-Extra: security\n"
	libs, deps, err := packaging.NewParser().Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if libs[0].Source != "https://example.test/app" {
		t.Fatalf("source %q", libs[0].Source)
	}
	if libs[0].Name == "security" {
		t.Fatal("Provides-Extra became a component")
	}
	got := map[string]types.Requirement{}
	for _, q := range deps[0].Requirements {
		got[q.Target] = q
	}
	if got["requests"].Constraint != ">=2.0,<3" || got["requests"].Condition != `python_version < "3.12"; extras=[security]` {
		t.Fatalf("name extras/marker %+v", got["requests"])
	}
	if got["localpkg"].Constraint != "" || got["localpkg"].Condition != "" {
		t.Fatalf("empty range %+v", got["localpkg"])
	}
	if got["demo"].Constraint != "" || !strings.Contains(got["demo"].Condition, "https://example.test/demo.whl") {
		t.Fatalf("url %+v", got["demo"])
	}
	if len(libs) != 1 {
		t.Fatalf("extra identities %+v", libs)
	}
}

func TestParsePackagingProjectURLHomepage(t *testing.T) {
	src, err := os.ReadFile("testdata/iniconfig-2.0.0.METADATA")
	if err != nil {
		t.Fatal(err)
	}
	libs, _, err := packaging.NewParser().Parse(nil, strings.NewReader(string(src)))
	if err != nil {
		t.Fatal(err)
	}
	if libs[0].Source != "https://github.com/pytest-dev/iniconfig" {
		t.Fatalf("Project-URL Homepage %q", libs[0].Source)
	}
	if libs[0].License != "MIT" {
		t.Fatalf("License-Expression %q", libs[0].License)
	}
}

func TestParsePackagingFoldedRequiresDist(t *testing.T) {
	src := "Name: app\nVersion: 1.0\nRequires-Dist: requests (>=2.0)\n  ; extra == 'docs'\n"
	_, deps, err := packaging.NewParser().Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || len(deps[0].Requirements) != 1 {
		t.Fatalf("%+v", deps)
	}
	q := deps[0].Requirements[0]
	if q.Target != "requests" || q.Constraint != ">=2.0" || q.Condition != "extra == 'docs'" {
		t.Fatalf("folded Requires-Dist %+v", q)
	}
}

func TestParsePackagingDuplicateHeaders(t *testing.T) {
	_, _, err := packaging.NewParser().Parse(nil, strings.NewReader("Name: foo\nName: bar\nVersion: 1.0\n"))
	if err == nil || !strings.Contains(err.Error(), "malformed_input") || !strings.Contains(err.Error(), "Name") {
		t.Fatalf("conflicting Name: %v", err)
	}
	_, _, err = packaging.NewParser().Parse(nil, strings.NewReader("Name: foo\nVersion: 1.0\nVersion: 2.0\n"))
	if err == nil || !strings.Contains(err.Error(), "malformed_input") || !strings.Contains(err.Error(), "Version") {
		t.Fatalf("conflicting Version: %v", err)
	}
	libs, _, err := packaging.NewParser().Parse(nil, strings.NewReader("Name: foo\nName: foo\nVersion: 1.0\n"))
	if err != nil || len(libs) != 1 || libs[0].Name != "foo" {
		t.Fatalf("duplicate identical Name: %+v %v", libs, err)
	}
}

func TestParsePackagingTruncated(t *testing.T) {
	_, _, err := packaging.NewParser().Parse(nil, strings.NewReader("Name: foo\n"))
	if err == nil || !strings.Contains(err.Error(), "malformed_input") {
		t.Fatalf("truncated: %v", err)
	}
	_, _, err = packaging.NewParser().Parse(nil, strings.NewReader("Version: 1.0\n"))
	if err == nil || !strings.Contains(err.Error(), "malformed_input") {
		t.Fatalf("missing name: %v", err)
	}
}

func TestParsePackagingNetworkxLicenseFileNotHash(t *testing.T) {
	src, err := os.ReadFile("testdata/networkx-3.0.METADATA")
	if err != nil {
		t.Fatal(err)
	}
	libs, deps, err := packaging.NewParser().Parse(nil, strings.NewReader(string(src)))
	if err != nil {
		t.Fatal(err)
	}
	if libs[0].License != "file://LICENSE.txt" {
		t.Fatalf("license-file %q", libs[0].License)
	}
	if libs[0].Source != "https://networkx.org/" {
		t.Fatalf("home-page %q", libs[0].Source)
	}
	found := false
	for _, q := range deps[0].Requirements {
		if q.Target == "numpy" && q.Constraint == ">=1.20" && q.Condition == "extra == 'default'" {
			found = true
		}
	}
	if !found {
		t.Fatalf("numpy extra default dropped: %+v", deps[0].Requirements)
	}
}
