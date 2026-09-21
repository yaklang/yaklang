package conan

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParseConanOriginalRefs(t *testing.T) {
	f, err := os.Open("testdata/happy.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	libs, deps, err := NewParser().Parse(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, l := range libs {
		got[l.Name] = l.Source
	}
	if got["pkga"] != "pkga/0.0.1" || got["pkgb"] != "pkgb/system" || got["pkgc"] != "pkgc/0.1.1@user/testing" {
		t.Fatalf("full refs: %v", got)
	}
	var reqs int
	for _, d := range deps {
		for _, q := range d.Requirements {
			reqs++
			if strings.HasPrefix(q.Target, "unresolved-") || strings.Contains(q.Target, "#") && !strings.Contains(q.Condition, "/") {
				t.Fatalf("synthetic target: %+v", q)
			}
			if q.Target == "pkgb" && (q.Constraint != "system" || q.Condition != "pkgb/system") {
				t.Fatalf("pkgb edge %+v", q)
			}
		}
	}
	if reqs == 0 {
		t.Fatal("original requires dropped")
	}
}

func TestParseConanRevisionRef(t *testing.T) {
	f, err := os.Open("testdata/happy2.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	libs, deps, err := NewParser().Parse(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, l := range libs {
		got[l.Name+"@"+l.Version] = l.Source
	}
	if got["openssl@3.0.3"] != "openssl/3.0.3#288ab73765e69844899535609ee0dfe4" {
		t.Fatalf("revision stripped: %v", got)
	}
	found := false
	for _, d := range deps {
		for _, q := range d.Requirements {
			if q.Target == "zlib" && q.Constraint == "1.2.12" && q.Condition == "zlib/1.2.12#b76db676bd992afa93dd18a675323942" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("zlib original ref: %+v", deps)
	}
}

func TestParseConanMissingAndEmptyRef(t *testing.T) {
	parse := func(body string) ([]types.Library, []types.Dependency, error) {
		t.Helper()
		return NewParser().Parse(nil, strings.NewReader(`{"graph_lock":{"nodes":`+body+`},"version":"0.4"}`))
	}
	libs, deps, err := parse(`{"0":{"requires":["1"]},"1":{"ref":"app/1","requires":["2"]},"2":{"ref":"zlib/1.2.12"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 2 {
		t.Fatalf("valid packages %+v", libs)
	}
	found := false
	for _, d := range deps {
		for _, q := range d.Requirements {
			if q.Target == "zlib" && q.Constraint == "1.2.12" && q.Resolved != "" {
				found = true
			}
			if q.Target == "2" {
				t.Fatalf("native id as package name: %+v", q)
			}
		}
	}
	if !found {
		t.Fatalf("valid zlib edge: %+v", deps)
	}

	for _, body := range []string{
		`{"0":{"requires":["1"]},"1":{"ref":"app/1","requires":["2"]},"2":{"ref":""}}`,
		`{"0":{"requires":["1"]},"1":{"ref":"app/1","requires":["2"]},"3":{"ref":"unused/1"}}`,
		`{"0":{"requires":["1"]},"1":{"ref":"app/1","requires":["2","2"]},"3":{"ref":"unused/1"}}`,
	} {
		libs, deps, err = parse(body)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, l := range libs {
			got[l.Name] = true
		}
		if !got["app"] {
			t.Fatalf("known package dropped: %+v", libs)
		}
		if got["zlib"] || got["2"] {
			t.Fatalf("fictional package: %+v", libs)
		}
		var unresolved, named int
		for _, d := range deps {
			for _, q := range d.Requirements {
				if q.Target == "2" || q.Target == "zlib" {
					t.Fatalf("native id or invented name: %+v", q)
				}
				if q.Condition == "2" && q.Target == "" && q.Resolved == "" {
					unresolved++
				}
				if q.Target != "" {
					named++
				}
			}
		}
		if unresolved != 1 {
			t.Fatalf("want one raw unresolved node 2, got %d: %+v", unresolved, deps)
		}
		incomplete := false
		for _, l := range libs {
			for _, d := range l.Diagnostics {
				if d.Code == "evidence_insufficient" && d.Incomplete {
					incomplete = true
				}
			}
		}
		if !incomplete {
			t.Fatalf("missing material marked complete: %+v", libs)
		}
	}
}

func TestParseConanRootWithoutRef(t *testing.T) {
	f, err := os.Open("testdata/happy.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	libs, _, err := NewParser().Parse(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 3 {
		t.Fatalf("root node 0 without ref must not reject the lock: %+v", libs)
	}
}
