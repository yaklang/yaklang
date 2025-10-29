package java

import (
	"embed"
	_ "embed"
	"fmt"
	"path"
	"testing"

	"golang.org/x/exp/slices"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed sample
var sample_code embed.FS

//go:embed mustpass
var sf_rules embed.FS

func Test_Debug(t *testing.T) {
	programID := uuid.NewString()
	progs, err := ssaapi.ParseProjectWithFS(
		filesys.NewEmbedFS(sample_code),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		// ssaapi.WithDatabaseProgramName(programID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
	}()
	if err != nil {
		t.Fatalf("parse project error: %v", err)
	}
	Check(t, progs, "jseval.sf")
}

func TestCheckRuleInSource(t *testing.T) {
	// source
	prog, err := ssaapi.ParseProjectWithFS(
		filesys.NewEmbedFS(sample_code),
		ssaapi.WithLanguage(ssaconfig.JAVA),
	)
	if err != nil {
		t.Fatalf("parse project error: %v", err)
	}
	Check(t, prog)
}

func TestCheckRuleWithDatabase(t *testing.T) {
	programID := uuid.NewString()
	prog, err := ssaapi.ParseProjectWithFS(
		filesys.NewEmbedFS(sample_code),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(programID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
	}()
	if err != nil {
		t.Fatalf("parse project error: %v", err)
	}
	Check(t, prog)
}

func TestCheckRuleOnlyDatabase(t *testing.T) {
	programID := uuid.NewString()
	// compile with database
	{
		_, err := ssaapi.ParseProjectWithFS(
			filesys.NewEmbedFS(sample_code),
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithProgramName(programID),
		)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
		}()
		if err != nil {
			t.Fatalf("parse project error: %v", err)
		}
	}

	//  only database
	{
		prog, err := ssaapi.FromDatabase(programID)
		if err != nil {
			t.Fatalf("parse project error: %v", err)
		}
		Check(t, []*ssaapi.Program{prog})
	}
}

func Check(t *testing.T, progs []*ssaapi.Program, include ...string) {
	entry, err := sf_rules.ReadDir("mustpass")
	if err != nil {
		t.Fatalf("no embed syntax files found: %v", err)
	}
	for _, f := range entry {
		if f.IsDir() {
			continue
		}
		rulePath := path.Join("mustpass", f.Name())
		rule, err := sf_rules.ReadFile(rulePath)
		if err != nil {
			t.Fatalf("cannot found syntax fs: %v", rulePath)
		}
		if len(include) != 0 && !slices.Contains(include, f.Name()) {
			continue
		}
		t.Log("compile success: ", rulePath)

		t.Run(f.Name(), func(t *testing.T) {
			res, err := ssaapi.QuerySyntaxflow(
				ssaapi.QueryWithPrograms(progs),
				ssaapi.QueryWithRuleContent(string(rule)),
				ssaapi.QueryWithEnableDebug(),
				ssaapi.QueryWithFailFast(),
			)
			if err != nil {
				t.Fatalf("feed error: %v", err)
			}
			fmt.Println(res.String())
		})
	}
}
