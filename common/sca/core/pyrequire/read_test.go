package pyrequire

import (
	"context"
	"strings"
	"testing"
)

func TestDeclarationsPreserveUnknownEnvironment(t *testing.T) {
	r, e := Declaration(`demo[security] >= 1.0, < 2; python_version < "3.12" and sys_platform == 'win32'`)
	if e != nil || r.Version != "" || r.Name != "demo" || r.Extras != "security" || r.Constraint != ">=1.0,<2" || r.Marker == "" {
		t.Fatalf("%+v %v", r, e)
	}
	r, e = Declaration(`demo @ https://example.org/a.whl#sha256=abc`)
	if e != nil || !strings.HasSuffix(r.URL, "#sha256=abc") {
		t.Fatalf("URI fragment lost: %+v %v", r, e)
	}
}
func TestBoundaries(t *testing.T) {
	for _, s := range []string{"-r ../secret", "a[", "a >=", "a;", "a; os_name == 'broken", "a; (x"} {
		if _, e := Declaration(s); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	if _, e := Parse(context.Background(), []byte("a==1 \\")); e == nil {
		t.Fatal("accepted truncated continuation")
	}
}
func FuzzDeclaration(f *testing.F) {
	f.Add("a[extra]>=1; python_version < '3.12'")
	f.Add("a @ file:../a")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<16 {
			return
		}
		Declaration(s)
	})
}
