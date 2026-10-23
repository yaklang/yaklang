package ssaapi

import (
	"reflect"
	"testing"
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
