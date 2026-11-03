package ssautils

import (
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestStripeByArchiveSpreadsJars(t *testing.T) {
	in := []string{
		"lib/a.jar/com/A.class",
		"lib/a.jar/com/B.class",
		"lib/a.jar/com/C.class",
		"lib/b.jar/com/D.class",
		"lib/b.jar/com/E.class",
		"loose/X.class",
	}
	got := stripeByArchive(in)
	want := []string{
		"lib/a.jar/com/A.class",
		"lib/b.jar/com/D.class",
		"loose/X.class",
		"lib/a.jar/com/B.class",
		"lib/b.jar/com/E.class",
		"lib/a.jar/com/C.class",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stripeByArchive() = %#v, want %#v", got, want)
	}
}

func TestArchiveGroupKeyUsesInnermostArchive(t *testing.T) {
	key := archiveGroupKey(`web/app.war/WEB-INF/lib/inner.jar/com/A.class`)
	if key != "web/app.war/web-inf/lib/inner.jar" {
		t.Fatalf("key = %q", key)
	}
	if archiveGroupKey("src/Main.java") != "src/Main.java" {
		t.Fatal("loose file should be its own group")
	}
}

func TestProbeLanguageUsesExtensionSample(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for i := 0; i < 50; i++ {
		vf.AddFile("src/"+string(rune('a'+i%26))+"_seed.js", "var a = 1;")
	}
	vf.AddFile("src/Main.java", "class Main {}")
	// Java is the first language in the probe order: a webapp's bundled
	// JavaScript must not outvote the project language.
	if got := ProbeLanguage(vf, "."); got != "java" {
		t.Fatalf("ProbeLanguage = %q, want java", got)
	}

	pythonOnly := filesys.NewVirtualFs()
	pythonOnly.AddFile("app.py", "print(1)")
	if got := ProbeLanguage(pythonOnly, "."); got != "python" {
		t.Fatalf("ProbeLanguage = %q, want python", got)
	}

	empty := filesys.NewVirtualFs()
	empty.AddFile("notes.txt", "x")
	if got := ProbeLanguage(empty, "."); got != "" {
		t.Fatalf("ProbeLanguage = %q, want no evidence", got)
	}
}
