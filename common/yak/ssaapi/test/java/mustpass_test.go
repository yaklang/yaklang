package java

import (
	"embed"
	"fmt"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"io/fs"
	"strings"
	"testing"
)

//go:embed mustpass
var mustpassFS embed.FS

//go:embed sample
var sourceCodeSample embed.FS

const MUSTPASS_JAVA_CACHE_KEY = "54Ot5qCH562+77yM5Y+v5Lul5Lit6Ze05aSE55CGZGVzIGFlc+etieWKoOWvhu+8jOaXoOmcgOWGjeeisHB5IOKAlOKAlOaYr+aenOWunuiPjOWVig==a-"

func TestMustPassMapping(t *testing.T) {
	ssatest.CheckFSWithProgram(
		t, MUSTPASS_JAVA_CACHE_KEY,
		filesys.NewEmbedFS(sourceCodeSample),
		filesys.NewEmbedFS(mustpassFS),
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestMustPass_JAVA_Debug_Compile(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}

	ssadb.DeleteProgram(ssadb.GetDB(), MUSTPASS_JAVA_CACHE_KEY)

	_, err := ssaapi.ParseProject(filesys.NewEmbedFS(sourceCodeSample), ssaapi.WithDatabaseProgramName(MUSTPASS_JAVA_CACHE_KEY), ssaapi.WithLanguage(ssaapi.JAVA))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	program, err := ssaapi.FromDatabase(MUSTPASS_JAVA_CACHE_KEY)
	if err != nil {
		t.Fatalf("get program from database failed: %v", err)
	}
	_ = program
}

func TestMustPass_Debug(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}

	keyword := "xxe.sf"
	prog, err := ssaapi.FromDatabase(MUSTPASS_JAVA_CACHE_KEY)
	if err != nil {
		t.Fatal(err)
	}

	code := filesys.NewEmbedFS(mustpassFS)

	err = filesys.Recursive(".", filesys.WithEmbedFS(mustpassFS), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if !strings.Contains(s, keyword) {
			return nil
		}
		raw, err := code.ReadFile(s)
		if err != nil {
			return err
		}
		vm, _, err := prog.SyntaxFlowEx(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		result, err := vm.FirstResult()
		if err != nil {
			t.Fatal(err)
		}
		result.Show()
		fmt.Println("\n--------------------------------------")
		result.Vars.ForEach(func(i string, v sfvm.ValueOperator) bool {
			for _, raw := range ssaapi.SyntaxFlowVariableToValues(v).DotGraph() {
				fmt.Println(raw)
				fmt.Println()
			}
			return true
		})
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
}
