package preprocess

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// TestDirectiveNameStopsAtComment guards the #endif// and #if(x) forms: the
// directive name is the leading identifier run, not the whole first field.
func TestDirectiveNameStopsAtComment(t *testing.T) {
	require.Equal(t, "endif", DirectiveName("#endif// no space"))
	require.Equal(t, "endif", DirectiveName("#endif/* block */"))
	require.Equal(t, "if", DirectiveName("#if(x)"))
	require.Equal(t, "define", DirectiveName("#  define X 1"))
	require.Equal(t, "", DirectiveName("#"))
}

// TestConditionalElifThenElse checks that an #elif branch does not consume the
// following #else: the else must still activate when every earlier branch is false.
func TestConditionalElifThenElse(t *testing.T) {
	cs := NewConditionalStack(&MacroEnvironment{tables: NewMacroTables()}, nil)
	cs.HandleDirective("#if 0")
	require.False(t, cs.Active())
	cs.HandleDirective("#elif 0")
	require.False(t, cs.Active())
	cs.HandleDirective("#else")
	require.True(t, cs.Active())
	cs.HandleDirective("#endif")
	require.True(t, cs.Active())
}

// TestMacroStringifyEscapesAndCollapsesWhitespace covers a multi-line argument:
// whitespace becomes one space and inner quotes are escaped.
func TestMacroStringifyEscapesAndCollapsesWhitespace(t *testing.T) {
	out, err := ExpandFunctionMacros("#define S(...) #__VA_ARGS__\nconst char* s = S(a(\"x\", \"y\"),\n  b);\n")
	require.NoError(t, err)
	require.Contains(t, out, `"a(\"x\", \"y\"), b"`)
}

// TestMacroPasteAfterStringify covers #x##suffix: the paste must apply to the
// string literal produced by #, not leave the ## behind.
func TestMacroPasteAfterStringify(t *testing.T) {
	out, err := ExpandFunctionMacros("#define M(x) #x##_tag\nauto v = M(hi);\n")
	require.NoError(t, err)
	require.Contains(t, out, `"hi"_tag`)
}

// TestMacroBodyCommentDoesNotSwallowSemicolon covers a line comment at the end
// of a macro body: phase 3 removes it, so the caller's semicolon survives.
func TestMacroBodyCommentDoesNotSwallowSemicolon(t *testing.T) {
	out, err := ExpandFunctionMacros("#define M(x) do { x; } while(false) // note\nvoid f() { M(a); }\n")
	require.NoError(t, err)
	require.NotContains(t, out, "// note")
	require.Contains(t, out, "while(false)")
	require.Regexp(t, `while\(false\)\s*;`, out)
}

// TestConcatAdjacentStringLiterals covers translation phase 6.
func TestConcatAdjacentStringLiterals(t *testing.T) {
	require.Equal(t, `const char* s = "ab";`, ConcatAdjacentStringLiterals(`const char* s = "a" "b";`))
	require.Equal(t, `const char* s = L"ab";`, ConcatAdjacentStringLiterals(`const char* s = L"a" "b";`))
	require.Equal(t, `const char* s = "ab";`, ConcatAdjacentStringLiterals("const char* s = \"a\"\n  \"b\";"))
	// Raw strings stay separate: source concatenation would change the contents.
	require.Equal(t, `auto r = R"(a)" "b";`, ConcatAdjacentStringLiterals(`auto r = R"(a)" "b";`))
}

// TestPreprocessTUExpandsMultiLineInvocation covers a macro call whose arguments
// span source lines, which the per-line expansion used to leave untouched.
func TestPreprocessTUExpandsMultiLineInvocation(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("framework.hpp", "#define T(name, tag) do { name(tag); } while(false)\n")
	entry := "#include \"framework.hpp\"\nvoid f() {\n  T(run,\n    \"multi-line\");\n}\n"
	fs.AddFile("main.cpp", entry)
	config := DefaultConfig()
	config.HeaderExtensions = []string{".hpp"}
	project := BuildProject(fs, config)
	defer project.Close()
	out, err := project.PreprocessTU("main.cpp", entry)
	require.NoError(t, err)
	require.Contains(t, out, `do { run("multi-line"); } while(false)`)
}

// A macro that appears after a multi-line block comment must still expand: the
// incremental scanner has to consume comment lines for their state even when
// they cannot contain a macro, or it never sees the closing */ and treats the
// rest of the translation unit as comment text.
func TestPreprocessTUExpandsAfterMultiLineComment(t *testing.T) {
	src := "/* first line\n * second line\n */\n#define ADD_ONE(x) ((x) + 1)\nint v = ADD_ONE(1);\n"
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("main.c", []byte(src), 0o644))
	proj := BuildProject(fs, DefaultConfig())

	out, err := proj.PreprocessTU("main.c", src)
	require.NoError(t, err)
	require.NotContains(t, out, "ADD_ONE(")
	require.Contains(t, out, "((1) + 1)")
}
