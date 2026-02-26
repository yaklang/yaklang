package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var ssaCompileTool = mcp.NewTool("ssa_compile",
	mcp.WithDescription(`Compile source code project into SSA (Static Single Assignment) intermediate representation.
The compiled program is persisted to database and can be reused for multiple SyntaxFlow queries.
Returns a program_name that should be used in subsequent ssa_query calls.

Workflow:
1. First compile: provide target + language + program_name → full compilation, returns program_name
2. Query: use program_name with ssa_query (can query unlimited times without recompiling)
3. Code changed (INCREMENTAL): provide target + language + base_program_name=<previous program_name>
   → only changed files are recompiled, creates a ProgramOverLay (base layer + diff layer)
   → returns a NEW program_name (diff program), use this for subsequent queries
   → the overlay automatically merges base + diff results during query
4. Full recompile (RARE): set re_compile=true to delete old data and recompile everything from scratch

IMPORTANT: For incremental compilation, use base_program_name (NOT re_compile).
re_compile=true is a FULL recompile that discards all previous data.`),
	mcp.WithString("target",
		mcp.Description("Path to the project directory to compile"),
		mcp.Required(),
	),
	mcp.WithString("language",
		mcp.Description("Programming language of the source code"),
		mcp.Required(),
		mcp.EnumString("java", "php", "js", "golang", "yak", "c", "python"),
	),
	mcp.WithString("program_name",
		mcp.Description("Custom name for the compiled program. If not provided, auto-generated from target path. Recommended to set a meaningful name for reuse"),
	),
	mcp.WithString("base_program_name",
		mcp.Description("INCREMENTAL compilation: name of a previously compiled program to diff against. Only changed files will be recompiled. The system creates a ProgramOverLay that merges base + diff layers. Returns a NEW diff program_name"),
	),
	mcp.WithBool("re_compile",
		mcp.Description("FULL recompile: delete ALL old data and recompile from scratch. WARNING: this is NOT incremental — use base_program_name for incremental compilation"),
		mcp.Default(false),
	),
)

var ssaQueryTool = mcp.NewTool("ssa_query",
	mcp.WithDescription(`Execute a SyntaxFlow data flow query on a compiled SSA program.
SyntaxFlow is a DSL for querying data flow paths in code. It can answer questions like:
- "Which user inputs flow into dangerous functions (SQL injection, command execution)?"
- "What are the callers of a specific method?"
- "Trace the data flow from source to sink"

Key operators:
- Dot chain: Runtime.getRuntime().exec() — matches call chains
- #-> (TopDef): traces where a value comes from (Use-Def chain)
- --> (BottomUse): traces where a value flows to (Def-Use chain)
- ?{} : conditional filter, e.g. ?{opcode: call}
- as $var : capture matched values into a variable
- check $var then "msg" : assert variable is non-empty
- alert $var : mark variable as an alert/finding

Example rules:
1. Find command injection:  Runtime.getRuntime().exec(* #-> * as $source) as $sink;
2. Find SQL injection:      *sql*.append(*<slice(start=1)> as $params);
3. Find all calls to eval:  eval(*) as $dangerous;

Requires a program_name from a prior ssa_compile call.`),
	mcp.WithString("program_name",
		mcp.Description("Name of the compiled program (from ssa_compile result)"),
		mcp.Required(),
	),
	mcp.WithString("rule",
		mcp.Description("SyntaxFlow rule text to execute"),
		mcp.Required(),
	),
)

func init() {
	AddGlobalToolSet("ssa",
		WithTool(ssaCompileTool, handleSSACompile),
		WithTool(ssaQueryTool, handleSSAQuery),
	)
}

func handleSSACompile(_ *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments

		target, ok := args["target"].(string)
		if !ok || target == "" {
			return nil, utils.Error("missing required argument: target")
		}
		language, ok := args["language"].(string)
		if !ok || language == "" {
			return nil, utils.Error("missing required argument: language")
		}

		lang, err := ssaconfig.ValidateLanguage(language)
		if err != nil {
			return nil, utils.Wrapf(err, "invalid language: %s", language)
		}

		programName, _ := args["program_name"].(string)
		reCompile, _ := args["re_compile"].(bool)
		baseProgramName, _ := args["base_program_name"].(string)

		// Cache-hit detection: skip compilation if the program already exists and source files haven't changed
		if !reCompile && baseProgramName == "" && programName != "" {
			if result, ok := tryCompileCache(target, programName, lang); ok {
				return result, nil
			}
		}

		opts := []ssaconfig.Option{
			ssaapi.WithLanguage(lang),
			ssaapi.WithContext(ctx),
		}
		if programName != "" {
			opts = append(opts, ssaapi.WithProgramName(programName))
		}
		if reCompile {
			opts = append(opts, ssaapi.WithReCompile(reCompile))
		}
		if baseProgramName != "" {
			opts = append(opts,
				ssaapi.WithBaseProgramName(baseProgramName),
				ssaapi.WithEnableIncrementalCompile(true),
			)
		}

		progs, err := ssaapi.ParseProjectFromPath(target, opts...)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to compile project")
		}
		if len(progs) == 0 {
			return nil, utils.Error("compilation produced no programs")
		}

		prog := progs[0]
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Compilation successful.\nProgram Name: %s\nLanguage: %s\nFiles: %d\n",
			prog.GetProgramName(), prog.GetLanguage(), len(prog.Program.FileList)))

		if overlay := prog.GetOverlay(); overlay != nil {
			sb.WriteString(fmt.Sprintf("\nIncremental compilation: ProgramOverLay created\n"))
			sb.WriteString(fmt.Sprintf("  Layers: %d\n", overlay.GetLayerCount()))
			for i, name := range overlay.GetLayerProgramNames() {
				sb.WriteString(fmt.Sprintf("  Layer %d: %s\n", i+1, name))
			}
			sb.WriteString(fmt.Sprintf("  Aggregated files: %d\n", overlay.GetFileCount()))
		}

		sb.WriteString("\nUse this program_name in ssa_query to perform data flow analysis.")

		return &mcp.CallToolResult{
			Content: []any{
				mcp.TextContent{Type: "text", Text: sb.String()},
			},
		}, nil
	}
}

// tryCompileCache checks if a compiled program already exists and source files haven't been modified.
// Returns (result, true) on cache hit, (nil, false) on miss.
func tryCompileCache(target, programName string, lang ssaconfig.Language) (*mcp.CallToolResult, bool) {
	irProg, err := ssadb.GetProgram(programName, ssadb.Application)
	if err != nil || irProg == nil {
		return nil, false
	}

	if string(irProg.Language) != string(lang) {
		return nil, false
	}

	compiledAt := irProg.UpdatedAt
	if compiledAt.IsZero() {
		return nil, false
	}

	changed, err := hasSourceChanged(target, compiledAt)
	if err != nil || changed {
		return nil, false
	}

	log.Infof("[Cache Hit] Program %q already compiled, no source changes detected", programName)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Cache Hit] Program already compiled — no source files changed since last compilation.\n"))
	sb.WriteString(fmt.Sprintf("Program Name: %s\nLanguage: %s\nCompiled Files: %d\nLast Compiled: %s\n",
		irProg.ProgramName, irProg.Language, len(irProg.FileList), compiledAt.Format(time.RFC3339)))
	sb.WriteString("\nUse this program_name in ssa_query to perform data flow analysis.")

	return &mcp.CallToolResult{
		Content: []any{mcp.TextContent{Type: "text", Text: sb.String()}},
	}, true
}

// hasSourceChanged walks the target directory and checks if any file was modified after compiledAt.
func hasSourceChanged(target string, compiledAt time.Time) (bool, error) {
	info, err := os.Stat(target)
	if err != nil {
		return true, err
	}
	if !info.IsDir() {
		return info.ModTime().After(compiledAt), nil
	}

	changed := false
	err = filepath.Walk(target, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			base := filepath.Base(path)
			for _, skip := range []string{"vendor", "Vendor", "node_modules", ".git", "target", "classes", "build", ".idea"} {
				if base == skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if fi.ModTime().After(compiledAt) {
			changed = true
			return filepath.SkipAll
		}
		return nil
	})
	return changed, err
}

func handleSSAQuery(_ *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments

		programName, ok := args["program_name"].(string)
		if !ok || programName == "" {
			return nil, utils.Error("missing required argument: program_name")
		}
		rule, ok := args["rule"].(string)
		if !ok || rule == "" {
			return nil, utils.Error("missing required argument: rule")
		}

		prog, err := ssaapi.FromDatabase(programName)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to load program %q from database, please run ssa_compile first", programName)
		}

		var queryTarget ssaapi.SyntaxFlowQueryInstance = prog
		if overlay := prog.GetOverlay(); overlay != nil {
			queryTarget = overlay
		}

		result, err := queryTarget.SyntaxFlowWithError(rule, ssaapi.QueryWithContext(ctx))
		if err != nil {
			return nil, utils.Wrapf(err, "SyntaxFlow query failed")
		}

		return formatSSAQueryResult(result, programName, rule)
	}
}

func formatSSAQueryResult(result *ssaapi.SyntaxFlowResult, programName, rule string) (*mcp.CallToolResult, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Program: %s\nRule: %s\n", programName, rule))

	if errs := result.GetErrors(); len(errs) > 0 {
		sb.WriteString(fmt.Sprintf("\nErrors: %s\n", strings.Join(errs, "; ")))
	}
	if msgs := result.GetCheckMsg(); len(msgs) > 0 {
		sb.WriteString("\nCheck Messages:\n")
		for _, msg := range msgs {
			sb.WriteString(fmt.Sprintf("  - %s\n", msg))
		}
	}

	alertVars := result.GetAlertVariables()
	allVars := result.GetAllVariable()

	if len(alertVars) > 0 {
		sb.WriteString(fmt.Sprintf("\n=== Alert Variables (%d) ===\n", len(alertVars)))
		for _, name := range alertVars {
			values := result.GetValues(name)
			sb.WriteString(fmt.Sprintf("\n[ALERT] $%s (%d values):\n", name, len(values)))
			if msg, ok := result.GetAlertMsg(name); ok {
				sb.WriteString(fmt.Sprintf("  Message: %s\n", msg))
			}
			writeSSAValues(&sb, values)
		}
	}

	if allVars != nil {
		nonAlertCount := 0
		alertSet := make(map[string]bool)
		for _, name := range alertVars {
			alertSet[name] = true
		}
		allVars.ForEach(func(name string, value any) {
			if !alertSet[name] && name != "_" {
				nonAlertCount++
			}
		})
		if nonAlertCount > 0 {
			sb.WriteString(fmt.Sprintf("\n=== Other Variables (%d) ===\n", nonAlertCount))
			allVars.ForEach(func(name string, value any) {
				if alertSet[name] || name == "_" {
					return
				}
				count, _ := value.(int)
				values := result.GetValues(name)
				sb.WriteString(fmt.Sprintf("\n$%s (%d values):\n", name, count))
				writeSSAValues(&sb, values)
			})
		}
	}

	if sb.Len() < 100 {
		sb.WriteString("\nNo results found for this query.\n")
	}

	return &mcp.CallToolResult{
		Content: []any{
			mcp.TextContent{
				Type: "text",
				Text: sb.String(),
			},
		},
	}, nil
}

func writeSSAValues(sb *strings.Builder, values ssaapi.Values) {
	for i, val := range values {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("  ... and %d more values\n", len(values)-20))
			break
		}
		sb.WriteString(fmt.Sprintf("  [%d] %s\n", i, val.String()))
		if rng := val.GetRange(); rng != nil {
			if editor := rng.GetEditor(); editor != nil {
				sb.WriteString(fmt.Sprintf("       File: %s\n", editor.GetFilename()))
			}
			start, end := rng.GetStart(), rng.GetEnd()
			sb.WriteString(fmt.Sprintf("       Position: %d:%d - %d:%d\n",
				start.GetLine(), start.GetColumn(), end.GetLine(), end.GetColumn()))
			if textCtx := rng.GetTextContext(2); textCtx != "" {
				sb.WriteString("       Context:\n")
				for _, line := range strings.Split(textCtx, "\n") {
					sb.WriteString(fmt.Sprintf("         %s\n", line))
				}
			}
		}
	}
}

