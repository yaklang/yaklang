package filesys

import (
	"io/fs"
	"testing"
)

type pathEchoFS struct {
	*VirtualFS
	reads []string
	dirs  []string
}

func (e *pathEchoFS) ReadFile(name string) ([]byte, error) {
	e.reads = append(e.reads, name)
	return []byte("ok:" + name), nil
}

func (e *pathEchoFS) ReadDir(name string) ([]fs.DirEntry, error) {
	e.dirs = append(e.dirs, name)
	return nil, nil
}

func TestHookFSPathHook(t *testing.T) {
	inner := &pathEchoFS{VirtualFS: NewVirtualFs()}
	stripped := NewHookFS(inner)
	stripped.SetPathHook(StripPathPrefix("demo"))

	var hookName string
	stripped.AddReadHook(&ReadHook{
		Matcher: HookMatcherFunc(func(name string) bool { return true }),
		AfterRead: func(ctx *ReadHookContext, data []byte) ([]byte, error) {
			hookName = ctx.Name
			return append([]byte("hooked:"), data...), nil
		},
	})

	got, err := stripped.ReadFile("/demo/A.java")
	if err != nil {
		t.Fatalf("strip read: %v", err)
	}
	if string(got) != "hooked:ok:/A.java" {
		t.Fatalf("strip read content: %q", got)
	}
	if hookName != "/demo/A.java" {
		t.Fatalf("read hook saw %q", hookName)
	}

	if _, err := stripped.ReadFile("src/B.java"); err != nil {
		t.Fatalf("plain read: %v", err)
	}
	if _, err := stripped.ReadDir("/demo"); err != nil {
		t.Fatalf("strip dir: %v", err)
	}
	if _, err := stripped.ReadDir("."); err != nil {
		t.Fatalf("dot dir: %v", err)
	}

	wantReads := []string{"/A.java", "/src/B.java"}
	if len(inner.reads) != len(wantReads) {
		t.Fatalf("reads: %#v", inner.reads)
	}
	for i, want := range wantReads {
		if inner.reads[i] != want {
			t.Fatalf("reads: %#v", inner.reads)
		}
	}
	if len(inner.dirs) != 2 || inner.dirs[0] != "/" || inner.dirs[1] != "/" {
		t.Fatalf("dirs: %#v", inner.dirs)
	}

	prefixedInner := &pathEchoFS{VirtualFS: NewVirtualFs()}
	prefixed := NewHookFS(prefixedInner)
	prefixed.SetPathHook(AddPathPrefix("demo"))
	if _, err := prefixed.ReadFile("/A.java"); err != nil {
		t.Fatalf("add read: %v", err)
	}
	if _, err := prefixed.ReadFile("/demo/A.java"); err != nil {
		t.Fatalf("already prefixed: %v", err)
	}
	if _, err := prefixed.ReadDir("/"); err != nil {
		t.Fatalf("add dir: %v", err)
	}
	if prefixedInner.reads[0] != "/demo/A.java" || prefixedInner.reads[1] != "/demo/A.java" {
		t.Fatalf("add reads: %#v", prefixedInner.reads)
	}
	if len(prefixedInner.dirs) != 1 || prefixedInner.dirs[0] != "/demo" {
		t.Fatalf("add dirs: %#v", prefixedInner.dirs)
	}
}
