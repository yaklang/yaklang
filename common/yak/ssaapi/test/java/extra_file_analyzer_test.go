package java

import (
	"embed"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

//go:embed sample/springboot
var springbootLoader embed.FS

func TestExtraFileAnalyzer(t *testing.T) {
	prog, err := ssaapi.ParseProject(filesys.NewEmbedFS(springbootLoader), ssaapi.WithLanguage(ssaapi.JAVA))
	if err != nil {
		t.Fatal(err)
	}
	_ = prog
}
