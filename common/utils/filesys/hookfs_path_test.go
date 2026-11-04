package filesys

import (
	"errors"
	"io/fs"
	"slices"
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

// A skip matcher hides entries from the walk: ReadDir drops them and every
// direct lookup reports absence, so a caller cannot reach them around the
// listing.
func TestHookFSSkipMatcher(t *testing.T) {
	inner := NewVirtualFs()
	inner.AddFile("src/Keep.java", "class Keep {}")
	inner.AddFile("src/drop.jar", "binary")
	inner.AddFile("lib/nested.war", "binary")

	hooked := NewHookFS(inner)
	hooked.SetSkipMatcher(SuffixMatcher(".jar", ".war"))

	walked := map[string]bool{}
	err := Recursive(".",
		WithFileSystem(hooked),
		WithFileStat(func(pathname string, _ fs.FileInfo) error {
			walked[pathname] = true
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if !walked["src/Keep.java"] {
		t.Fatalf("kept file missing from walk: %#v", walked)
	}
	for _, hidden := range []string{"src/drop.jar", "lib/nested.war"} {
		if walked[hidden] {
			t.Fatalf("%s must not be walked: %#v", hidden, walked)
		}
		if _, err := hooked.Stat(hidden); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Stat(%s) err = %v, want fs.ErrNotExist", hidden, err)
		}
		if ok, err := hooked.Exists(hidden); err != nil || ok {
			t.Fatalf("Exists(%s) = %v, %v; want false, nil", hidden, ok, err)
		}
		if _, err := hooked.ReadFile(hidden); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("ReadFile(%s) err = %v, want fs.ErrNotExist", hidden, err)
		}
	}

	// Disabling the hook restores the full view.
	hooked.SetEnabled(false)
	if entries, err := hooked.ReadDir("src"); err != nil {
		t.Fatalf("disabled ReadDir: %v", err)
	} else {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		slices.Sort(names)
		if strings.Join(names, ",") != "Keep.java,drop.jar" {
			t.Fatalf("disabled ReadDir names = %v", names)
		}
	}
}
