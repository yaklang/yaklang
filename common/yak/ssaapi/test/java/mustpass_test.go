package java

import (
	"embed"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed mustpass
var mustpassFS embed.FS

//go:embed sample
var sampleFS embed.FS

func TestMustPassMapping(t *testing.T) {
	ssatest.CheckFSWithProgram(
		t, "mustpass",
		filesys.NewEmbedFS(sampleFS),
		filesys.NewEmbedFS(mustpassFS),
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}
