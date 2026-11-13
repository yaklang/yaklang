package ssaapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestResolveSyntaxFlowIncludeRuleUsesTaskLocalSnapshotWithoutProfileDB(t *testing.T) {
	bindUnavailableProfileDatabase(t)

	localRule := &schema.SyntaxFlowRule{
		RuleName:     "snapshot-lib.sf",
		IncludedName: "snapshot-lib",
		Content:      `desc(lib: "snapshot-lib"); * as $output; alert $output;`,
	}
	ctx := WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), map[string]*schema.SyntaxFlowRule{
		"snapshot-lib": localRule,
	})

	got, taskLocal, err := resolveSyntaxFlowIncludeRule(ctx, "snapshot-lib")
	if err != nil {
		t.Fatalf("resolve task-local include with nil profile DB: %v", err)
	}
	if !taskLocal || got != localRule {
		t.Fatalf("unexpected task-local resolution: local=%v got=%#v", taskLocal, got)
	}
}

func TestResolveSyntaxFlowIncludeRuleDoesNotFallbackWhenSnapshotLibraryMissing(t *testing.T) {
	bindUnavailableProfileDatabase(t)

	ctx := WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), nil)
	got, taskLocal, err := resolveSyntaxFlowIncludeRule(ctx, "shared-db-only")
	if got != nil || !taskLocal || err == nil || !strings.Contains(err.Error(), "not present in the prepared rule snapshot") {
		t.Fatalf("missing snapshot library fell back outside task scope: local=%v got=%#v err=%v", taskLocal, got, err)
	}
}

func TestTaskLocalSyntaxFlowIncludeExecutesWithoutProfileDatabase(t *testing.T) {
	bindUnavailableProfileDatabase(t)
	GetSFIncludeCache().Purge()
	t.Cleanup(func() { GetSFIncludeCache().Purge() })

	program, err := Parse(
		`a1 = 1; a2 = "hello world";`,
		WithLanguage(ssaconfig.Yak),
	)
	if err != nil {
		t.Fatalf("compile test program: %v", err)
	}
	library, err := sfdb.CheckSyntaxFlowRuleContent(`
desc(lib: "snapshot-lib");
$input?{have: "hello world"} as $output;
alert $output;
`)
	if err != nil {
		t.Fatalf("compile task-local library: %v", err)
	}
	ctx := WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), map[string]*schema.SyntaxFlowRule{
		"snapshot-lib": library,
	})
	result, err := program.SyntaxFlowWithError(`
a* as $check;
$check<include("snapshot-lib")> as $target;
`, QueryWithContext(ctx), QueryWithMemory())
	if err != nil {
		t.Fatalf("execute task-local include with unavailable profile DB: %v", err)
	}
	if got := result.GetValues("target"); len(got) != 1 || !strings.Contains(got.String(), "hello world") {
		t.Fatalf("task-local include returned unexpected values: %v", got)
	}
}

func TestTaskLocalSyntaxFlowIncludeCacheIsIsolatedBySnapshotContent(t *testing.T) {
	bindUnavailableProfileDatabase(t)
	GetSFIncludeCache().Purge()
	t.Cleanup(func() { GetSFIncludeCache().Purge() })

	program, err := Parse(
		`a1 = 1; a2 = "hello world";`,
		WithLanguage(ssaconfig.Yak),
	)
	if err != nil {
		t.Fatalf("compile test program: %v", err)
	}
	compileLibrary := func(content string) *schema.SyntaxFlowRule {
		t.Helper()
		rule, compileErr := sfdb.CheckSyntaxFlowRuleContent(content)
		if compileErr != nil {
			t.Fatalf("compile task-local library: %v", compileErr)
		}
		return rule
	}
	query := func(library *schema.SyntaxFlowRule) string {
		t.Helper()
		ctx := WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), map[string]*schema.SyntaxFlowRule{
			"snapshot-lib": library,
		})
		result, queryErr := program.SyntaxFlowWithError(`
a* as $check;
$check<include("snapshot-lib")> as $target;
`, QueryWithContext(ctx), QueryWithMemory())
		if queryErr != nil {
			t.Fatalf("execute task-local include: %v", queryErr)
		}
		return result.GetValues("target").String()
	}

	first := compileLibrary(`
desc(lib: "snapshot-lib");
$input?{have: "hello world"} as $output;
alert $output;
`)
	second := compileLibrary(`
desc(lib: "snapshot-lib");
$input?{have: "1"} as $output;
alert $output;
`)
	if got := query(first); !strings.Contains(got, "hello world") {
		t.Fatalf("first snapshot include result: %s", got)
	}
	if got := query(second); strings.Contains(got, "hello world") || !strings.Contains(got, "1") {
		t.Fatalf("second snapshot reused stale include cache: %s", got)
	}
}

func TestTaskLocalSyntaxFlowNestedIncludeCacheIsIsolatedByCompleteSnapshot(t *testing.T) {
	bindUnavailableProfileDatabase(t)
	GetSFIncludeCache().Purge()
	t.Cleanup(func() { GetSFIncludeCache().Purge() })

	program, err := Parse(
		`a1 = 1; a2 = "hello world";`,
		WithLanguage(ssaconfig.Yak),
	)
	if err != nil {
		t.Fatalf("compile test program: %v", err)
	}
	compileLibrary := func(content string) *schema.SyntaxFlowRule {
		t.Helper()
		rule, compileErr := sfdb.CheckSyntaxFlowRuleContent(content)
		if compileErr != nil {
			t.Fatalf("compile task-local library: %v", compileErr)
		}
		return rule
	}
	outer := compileLibrary(`
desc(lib: "outer");
$input<include("helper")> as $output;
alert $output;
`)
	query := func(helper *schema.SyntaxFlowRule) string {
		t.Helper()
		ctx := WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), map[string]*schema.SyntaxFlowRule{
			"outer":  outer,
			"helper": helper,
		})
		result, queryErr := program.SyntaxFlowWithError(`
a* as $check;
$check<include("outer")> as $target;
`, QueryWithContext(ctx), QueryWithMemory())
		if queryErr != nil {
			t.Fatalf("execute nested task-local include: %v", queryErr)
		}
		return result.GetValues("target").String()
	}

	firstHelper := compileLibrary(`
desc(lib: "helper");
$input?{have: "hello world"} as $output;
alert $output;
`)
	secondHelper := compileLibrary(`
desc(lib: "helper");
$input?{have: "1"} as $output;
alert $output;
`)
	if got := query(firstHelper); !strings.Contains(got, "hello world") {
		t.Fatalf("first nested snapshot include result: %s", got)
	}
	if got := query(secondHelper); strings.Contains(got, "hello world") || !strings.Contains(got, "1") {
		t.Fatalf("nested include reused stale helper cache: %s", got)
	}
}

func TestResolveSyntaxFlowIncludeRulePreservesSharedProfileDatabaseBehavior(t *testing.T) {
	oldProfileDB := consts.GetGormProfileDatabase()
	dbPath := filepath.Join(t.TempDir(), "profile.db")
	profileDB, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("create isolated profile DB: %v", err)
	}
	consts.BindProfileDatabase(profileDB, dbPath)
	t.Cleanup(func() {
		consts.BindProfileDatabase(oldProfileDB, "")
		_ = profileDB.Close()
	})

	want, err := sfdb.ImportRuleWithoutValidExWithDB(
		profileDB,
		"shared-lib.sf",
		`desc(lib: "shared-lib"); * as $output; alert $output;`,
		"",
		false,
	)
	if err != nil {
		t.Fatalf("import shared profile rule: %v", err)
	}
	got, taskLocal, err := resolveSyntaxFlowIncludeRule(context.Background(), "shared-lib")
	if err != nil {
		t.Fatalf("resolve ordinary shared include: %v", err)
	}
	if taskLocal || got == nil || got.RuleName != want.RuleName {
		t.Fatalf("ordinary include no longer used shared profile DB: local=%v got=%#v want=%#v", taskLocal, got, want)
	}
}

func bindUnavailableProfileDatabase(t *testing.T) {
	t.Helper()
	oldProfileDB := consts.GetGormProfileDatabase()
	dbPath := filepath.Join(t.TempDir(), "closed-profile.db")
	closedProfileDB, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("create isolated profile DB: %v", err)
	}
	if err := closedProfileDB.Close(); err != nil {
		t.Fatalf("close isolated profile DB: %v", err)
	}
	consts.BindProfileDatabase(closedProfileDB, dbPath)
	t.Cleanup(func() { consts.BindProfileDatabase(oldProfileDB, "") })
}

// TestStructParentIncludesStructLibrary keeps the struct-in-struct include
// working once library rules declare mode: "struct". QueryWithStruct alone
// leaves the query without a program value, so the include would return
// "no value found" and the parent would silently report nothing.
func TestStructParentIncludesStructLibrary(t *testing.T) {
	bindUnavailableProfileDatabase(t)

	fs := filesys.NewVirtualFs()
	fs.AddFile("main.go", "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n")
	programs, err := ParseProjectWithFS(fs, WithLanguage(ssaconfig.GO))
	if err != nil {
		t.Fatalf("compile test program: %v", err)
	}
	if len(programs) == 0 {
		t.Fatalf("compile test program: no program")
	}
	program := programs[0]
	library, err := sfdb.CheckSyntaxFlowRuleContent(`
desc(
	title: "snapshot-struct-lib"
	lib: "snapshot-struct-lib"
	mode: "struct"
)
println(* as $output);
alert $output for { title: "sink", name: "output" }
`)
	if err != nil {
		t.Fatalf("compile struct library: %v", err)
	}
	if !library.IsStructMode() {
		t.Fatalf("library did not keep struct mode: %q", library.Mode)
	}
	ctx := WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), map[string]*schema.SyntaxFlowRule{
		"snapshot-struct-lib": library,
	})

	result := runStructRuleOnUnit(t, program, `
desc(
	title: "snapshot-struct-parent"
	mode: "struct"
)
<include("snapshot-struct-lib")> as $hit;
alert $hit for { title: "parent hit" }
`, ctx)
	if got := len(result.GetValues("hit")); got == 0 {
		t.Fatalf("struct parent lost the values of its struct library")
	}
}

// runStructRuleOnUnit executes a struct rule against the program's first
// compile unit, which is what a struct scan does per unit.
func runStructRuleOnUnit(t *testing.T, program *Program, content string, ctx context.Context) *SyntaxFlowResult {
	t.Helper()
	unit := program.Program.GetApplication().CompileUnits
	if len(unit) == 0 {
		t.Fatalf("program has no compile unit")
	}
	result, err := QuerySyntaxflow(
		QueryWithProgram(program),
		QueryWithStruct(unit[0]),
		QueryWithRuleContent(content),
		QueryWithContext(ctx),
	)
	if err != nil {
		t.Fatalf("struct query: %v", err)
	}
	return result
}
