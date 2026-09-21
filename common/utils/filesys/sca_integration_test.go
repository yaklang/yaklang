package filesys_test

import (
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"strings"
	"testing"
)

func TestYakVirtualFilesystem(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("pom.xml", `<project><groupId>test</groupId><artifactId>root</artifactId><version>1</version><dependencies><dependency><groupId>test</groupId><artifactId>lib</artifactId><version>2</version></dependency></dependencies></project>`)
	info, err := vfs.Stat("pom.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("virtual file mode is not regular: %v", info.Mode())
	}
	pkgs, err := sca.ScanFilesystem(vfs)
	if err == nil || !strings.Contains(err.Error(), "evidence_insufficient") {
		t.Fatalf("missing metadata was not reported: %v", err)
	}
	names := map[string]string{}
	for _, p := range pkgs {
		names[p.Name] = p.Version
	}
	if len(names) != 2 || names["test:root"] != "1" || names["test:lib"] != "2" {
		t.Fatalf("virtual POM lost: %#v", names)
	}
	vfs.AddFile("repository/test/lib/2/lib-2.pom", `<project><groupId>test</groupId><artifactId>lib</artifactId><version>2</version></project>`)
	pkgs, err = sca.ScanFilesystem(vfs)
	if err != nil {
		t.Fatal(err)
	}
	names = map[string]string{}
	for _, p := range pkgs {
		names[p.Name] = p.Version
	}
	if len(names) != 2 || names["test:root"] != "1" || names["test:lib"] != "2" {
		t.Fatalf("explicit metadata lost: %+v", names)
	}
}
