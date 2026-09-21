package gradle

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParseGradleOriginalScopes(t *testing.T) {
	src := `# comment
cglib:cglib-nodep:2.1.2=testRuntimeClasspath,classpath
 org.springframework:spring-asm:3.1.3.RELEASE=classpath
org.springframework:spring-beans:5.0.5.RELEASE=compileClasspath, runtimeClasspath
empty=
`
	libs, deps, err := NewParser().Parse(nil, strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 0 {
		t.Fatalf("lockfile has no declared edges: %+v", deps)
	}
	got := map[string]types.Library{}
	for _, l := range libs {
		got[l.Name+"@"+l.Version] = l
	}
	if len(got) != 3 {
		t.Fatalf("%+v", libs)
	}
	if got["cglib:cglib-nodep@2.1.2"].Scope != "testRuntimeClasspath,classpath" {
		t.Fatalf("multi scope %q", got["cglib:cglib-nodep@2.1.2"].Scope)
	}
	if got["org.springframework:spring-asm@3.1.3.RELEASE"].Scope != "classpath" {
		t.Fatalf("classpath %q", got["org.springframework:spring-asm@3.1.3.RELEASE"].Scope)
	}
	if got["org.springframework:spring-beans@5.0.5.RELEASE"].Scope != "compileClasspath, runtimeClasspath" {
		t.Fatalf("spaced scopes %q", got["org.springframework:spring-beans@5.0.5.RELEASE"].Scope)
	}
	if got["cglib:cglib-nodep@2.1.2"].Locations[0].StartLine != 2 {
		t.Fatalf("location %+v", got["cglib:cglib-nodep@2.1.2"].Locations)
	}
}

func TestParseGradleMalformed(t *testing.T) {
	_, _, err := NewParser().Parse(nil, strings.NewReader("not-a-coordinate=classpath\n"))
	if err == nil || !strings.Contains(err.Error(), "malformed_input") {
		t.Fatalf("%v", err)
	}
}

func TestParseGradleFrozenHappyLockfile(t *testing.T) {
	f, err := os.Open("testdata/happy.lockfile")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	libs, deps, err := NewParser().Parse(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 0 {
		t.Fatalf("lockfile has no declared edges: %+v", deps)
	}
	got := map[string]types.Library{}
	for _, l := range libs {
		got[l.Name+"@"+l.Version] = l
	}
	if got["cglib:cglib-nodep@2.1.2"].Scope != "testRuntimeClasspath,classpath" {
		t.Fatalf("%q", got["cglib:cglib-nodep@2.1.2"].Scope)
	}
	if got["org.springframework:spring-asm@3.1.3.RELEASE"].Scope != "classpath" {
		t.Fatalf("%q", got["org.springframework:spring-asm@3.1.3.RELEASE"].Scope)
	}
	if got["org.springframework:spring-beans@5.0.5.RELEASE"].Scope != "compileClasspath, runtimeClasspath" {
		t.Fatalf("%q", got["org.springframework:spring-beans@5.0.5.RELEASE"].Scope)
	}
	if got["cglib:cglib-nodep@2.1.2"].Locations[0].StartLine != 4 {
		t.Fatalf("comment/blank lines must keep lock line numbers: %+v", got["cglib:cglib-nodep@2.1.2"].Locations)
	}
}
