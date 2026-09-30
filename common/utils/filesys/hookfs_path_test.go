package filesys

import (
	"strings"
	"testing"
)

type pathEchoFS struct {
	*VirtualFS
	reads []string
}

func (e *pathEchoFS) ReadFile(name string) ([]byte, error) {
	e.reads = append(e.reads, name)
	return []byte("ok:" + name), nil
}

func TestHookFSPathHook(t *testing.T) {
	inner := &pathEchoFS{VirtualFS: NewVirtualFs()}
	hooked := NewHookFS(inner)
	hooked.SetPathHook(func(name string) (string, error) {
		if name == "" || name == "." || name == "/" {
			return name, nil
		}
		const prefix = "demo/"
		cleaned := strings.TrimPrefix(name, "/")
		if strings.HasPrefix(cleaned, prefix) {
			return strings.TrimPrefix(cleaned, prefix), nil
		}
		return cleaned, nil
	})

	var hookName string
	hooked.AddReadHook(&ReadHook{
		Matcher: HookMatcherFunc(func(name string) bool { return true }),
		AfterRead: func(ctx *ReadHookContext, data []byte) ([]byte, error) {
			hookName = ctx.Name
			return append([]byte("hooked:"), data...), nil
		},
	})

	got, err := hooked.ReadFile("/demo/A.java")
	if err != nil {
		t.Fatalf("mapped read: %v", err)
	}
	if string(got) != "hooked:ok:A.java" {
		t.Fatalf("mapped read content: %q", got)
	}
	if hookName != "/demo/A.java" {
		t.Fatalf("read hook saw %q", hookName)
	}

	if _, err := hooked.ReadFile("src/B.java"); err != nil {
		t.Fatalf("plain read: %v", err)
	}
	if _, err := hooked.ReadFile("."); err != nil {
		t.Fatalf("dot read: %v", err)
	}
	want := []string{"A.java", "src/B.java", "."}
	if len(inner.reads) != len(want) {
		t.Fatalf("reads: %#v", inner.reads)
	}
	for i := range want {
		if inner.reads[i] != want[i] {
			t.Fatalf("reads: %#v", inner.reads)
		}
	}
}
