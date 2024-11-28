package java

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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

	_, err := ssaapi.ParseProjectWithFS(filesys.NewEmbedFS(sourceCodeSample), ssaapi.WithProgramName(MUSTPASS_JAVA_CACHE_KEY), ssaapi.WithLanguage(ssaapi.JAVA))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	defer ssadb.DeleteProgram(ssadb.GetDB(), MUSTPASS_JAVA_CACHE_KEY)
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

	prog, err := ssaapi.ParseProjectWithFS(filesys.NewEmbedFS(sourceCodeSample), ssaapi.WithProgramName(MUSTPASS_JAVA_CACHE_KEY), ssaapi.WithLanguage(ssaapi.JAVA))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	// defer ssadb.DeleteProgram(ssadb.GetDB(), MUSTPASS_JAVA_CACHE_KEY)

	keyword := "local-file-write.sf"
	// prog, err := ssaapi.FromDatabase(MUSTPASS_JAVA_CACHE_KEY)
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
		result, err := prog.SyntaxFlowWithError(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		result.Show()
		result.Dump(false)
		if len(result.GetErrors()) > 0 {
			t.Fatal("errors: ", strings.Join(result.GetErrors(), "\n"))
		}

		result.Show()

		fmt.Println("\n--------------------------------------")
		totalGraph := result.GetAllValuesChain().DotGraph()
		if err != nil {
			t.Fatalf("create dot graph failed: %v", err)
		}
		fmt.Println(totalGraph)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnnontation(t *testing.T) {

	check := func(t *testing.T, prog *ssaapi.Program) {
		rule := `
*Mapping.__ref__?{opcode: function} as $entryFunc;
$entryFunc(*?{opcode: param && !have: this} as $source);
`
		result, err := prog.SyntaxFlowWithError(rule)
		if err != nil {
			t.Fatal(err)
		}
		result.Show()
		valueName := lo.Map(result.GetValues("entryFunc"), func(value *ssaapi.Value, _ int) string {
			return value.String()
		})
		require.Greater(t, len(valueName), 10)
		require.Greater(t, len(result.GetValues("source")), 10)
	}

	t.Run("memory ", func(t *testing.T) {
		prog, err := ssaapi.ParseProjectWithFS(
			filesys.NewEmbedFS(sourceCodeSample),
			ssaapi.WithLanguage(ssaapi.JAVA),
		)
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}
		check(t, prog[0])
	})

	t.Run("db", func(t *testing.T) {
		ssadb.DeleteProgram(ssadb.GetDB(), MUSTPASS_JAVA_CACHE_KEY)
		_, err := ssaapi.ParseProjectWithFS(filesys.NewEmbedFS(sourceCodeSample), ssaapi.WithProgramName(MUSTPASS_JAVA_CACHE_KEY), ssaapi.WithLanguage(ssaapi.JAVA))
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}
		defer ssadb.DeleteProgram(ssadb.GetDB(), MUSTPASS_JAVA_CACHE_KEY)

		prog, err := ssaapi.FromDatabase(MUSTPASS_JAVA_CACHE_KEY)
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}
		check(t, prog)
	})

}
