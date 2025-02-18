package java

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestExcludeFile(t *testing.T) {
	t.Run("test exclude class file", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.java", `
class A{
	public static void main(String[] args){
		System.out.println("Hello World");
	}
}
`)
		vf.AddFile("test2.class", "class main{")
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			return nil
		}, ssaapi.WithLanguage(consts.JAVA), ssaapi.WithStrictMode(true))
	})
}
