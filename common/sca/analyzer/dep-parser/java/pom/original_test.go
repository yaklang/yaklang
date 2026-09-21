package pom

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/internal/fsio"
)

func TestParsePOMHappyOriginalEdges(t *testing.T) {
	f, err := os.Open("testdata/happy/pom.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	libs, deps, err := NewParser("happy/pom.xml").Parse(fsio.New(os.DirFS("testdata")), f)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]types.Library{}
	for _, l := range libs {
		got[l.Name+"@"+l.Version] = l
	}
	if got["com.example:happy@1.0.0"].License != "BSD-3-Clause" {
		t.Fatalf("license %q", got["com.example:happy@1.0.0"].License)
	}
	if _, ok := got["org.example:example-api@1.7.30"]; !ok {
		t.Fatalf("interpolated component missing: %+v", libs)
	}
	if _, ok := got["org.example:example-provided@999"]; !ok {
		t.Fatalf("provided component missing: %+v", libs)
	}
	var reqs []types.Requirement
	for _, d := range deps {
		if d.ID == packageID("com.example:happy", "1.0.0") {
			reqs = d.Requirements
		}
	}
	if len(reqs) == 0 {
		t.Fatalf("happy edges missing: %+v", deps)
	}
	foundAPI, foundProv := false, false
	for _, q := range reqs {
		if q.Scope == "runtime" {
			t.Fatalf("empty POM scope invented as runtime: %+v", q)
		}
		if q.Target == "org.example:example-api" {
			foundAPI = true
			if q.Constraint != "${api.version}" {
				t.Fatalf("original property token dropped: %+v", q)
			}
			if q.Resolved != packageID("org.example:example-api", "1.7.30") {
				t.Fatalf("property interpolation must resolve in-POM: %+v", q)
			}
		}
		if q.Target == "org.example:example-provided" {
			foundProv = true
			if q.Constraint != "999" || q.Scope != "provided" {
				t.Fatalf("provided scope/version: %+v", q)
			}
		}
	}
	if !foundAPI || !foundProv {
		t.Fatalf("original happy edges: %+v", reqs)
	}
}

func TestParsePOMOptionalAndRange(t *testing.T) {
	src := `<project><modelVersion>4.0.0</modelVersion><groupId>g</groupId><artifactId>a</artifactId><version>1</version>
<dependencies>
<dependency><groupId>g</groupId><artifactId>opt</artifactId><version>1</version><optional>true</optional></dependency>
<dependency><groupId>g</groupId><artifactId>plain</artifactId><version>2</version></dependency>
<dependency><groupId>g</groupId><artifactId>rng</artifactId><version>(,1.0]</version></dependency>
</dependencies></project>`
	libs, deps, err := NewParser("pom.xml").Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) == 0 {
		t.Fatal("root missing")
	}
	var reqs []types.Requirement
	for _, d := range deps {
		if strings.HasPrefix(d.ID, "g:a:") {
			reqs = d.Requirements
		}
	}
	got := map[string]types.Requirement{}
	for _, q := range reqs {
		got[q.Target] = q
		if q.Scope == "runtime" {
			t.Fatalf("invented runtime: %+v", q)
		}
	}
	if got["g:opt"].Condition != "optional" || got["g:opt"].Constraint != "1" {
		t.Fatalf("optional: %+v", got["g:opt"])
	}
	if got["g:plain"].Condition != "" || got["g:plain"].Scope != "" || got["g:plain"].Constraint != "2" {
		t.Fatalf("plain: %+v", got["g:plain"])
	}
	if got["g:rng"].Constraint != "(,1.0]" {
		t.Fatalf("range constraint: %+v", got["g:rng"])
	}
	if got["g:rng"].Resolved != "" {
		t.Fatalf("range uniquely resolved: %+v", got["g:rng"])
	}
}

func TestParsePOMMissingParentNoHostLookup(t *testing.T) {
	f, err := os.Open("testdata/not-found-parent/pom.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	libs, _, err := NewParser("not-found-parent/pom.xml").Parse(fsio.New(os.DirFS("testdata")), f)
	if err != nil {
		t.Fatal(err)
	}
	missing := false
	for _, l := range libs {
		for _, d := range l.Diagnostics {
			if d.Code == "evidence_insufficient" && strings.Contains(d.Reason, "com.example:parent:1.0.0") {
				missing = true
			}
		}
	}
	if !missing {
		t.Fatalf("missing parent must stay evidence_insufficient, not host lookup: %+v", libs)
	}
	got := map[string]bool{}
	for _, l := range libs {
		got[l.Name+"@"+l.Version] = true
	}
	if !got["com.example:no-parent@1.0-SNAPSHOT"] || !got["org.example:example-api@1.7.30"] {
		t.Fatalf("declared identities dropped: %+v", libs)
	}
}
