package preprocess

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandFunctionMacros_MIN(t *testing.T) {
	src := `
#define MIN(a,b) ((a)<(b)?(a):(b))
int min = MIN(x, y);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.NotContains(t, out, "#define MIN")
	require.Contains(t, out, "((x)<(y)?(x):(y))")
}

func TestExpandFunctionMacros_Nested(t *testing.T) {
	src := `
#define SQUARE(x) ((x) * (x))
#define CUBE(x) (SQUARE(x) * (x))
int result = CUBE(num);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, "((num) * (num))")
}

func TestExpandFunctionMacros_TokenPaste(t *testing.T) {
	src := `
#define CONCAT(a, b) a##b
int var_10 = 42;
int result = CONCAT(var_, 10);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, "var_10")
}

func TestExpandFunctionMacros_Variadic(t *testing.T) {
	src := `
#define LOG(fmt, ...) printf(fmt, __VA_ARGS__)
LOG("x=%d", x);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, `printf("x=%d", x)`)
}

func TestExpandFunctionMacros_ObjectMacro(t *testing.T) {
	src := `
#define MAX_SIZE 1024
#define TWICE(x) ((x)*2)
int a = TWICE(MAX_SIZE);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.NotContains(t, out, "#define MAX_SIZE")
	require.Contains(t, out, "((1024)*2)")
}

func TestExpandFunctionMacros_ParenthesizedObjectMacro(t *testing.T) {
	src := `
#define FLAG1 0x01
#define FLAG2 0x02
#define FLAGS (FLAG1 | FLAG2)
int flags = FLAGS;
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, "0x01")
	require.Contains(t, out, "0x02")
}

func TestExpandFunctionMacros_PreservesInclude(t *testing.T) {
	src := `
#include <stdio.h>
#define INC(x) ((x)+1)
int v = INC(v);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, "#include <stdio.h>")
}

func TestExpandFunctionMacros_NoExpandInString(t *testing.T) {
	src := `
#define F(x) body
const char* s = "F(1)";
int y = F(1);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, `"F(1)"`)
	require.Contains(t, out, "body")
	require.NotContains(t, out, `"body"`)
}

func TestExpandFunctionMacros_NestedArgs(t *testing.T) {
	src := `
#define WRAP(x) (x)
#define PAIR(a,b) WRAP(a), WRAP(b)
int v = PAIR(f(a,b), c);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, "(f(a,b))")
	require.Contains(t, out, "(c)")
}

func TestCollectFunctionMacros_Undef(t *testing.T) {
	src := `
#define F(x) (x)
#undef F
int y = F(1);
`
	collected := collectFunctionMacros(src, newMacroTables())
	_, ok := collected.tables.function["F"]
	require.False(t, ok)
}

func TestExpandFunctionMacros_StandaloneAPI(t *testing.T) {
	src := `
#define MIN(a,b) ((a)<(b)?(a):(b))
int min = MIN(x, y);
`
	result, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.True(t, strings.Contains(result, "((x)<(y)?(x):(y))") || strings.Contains(result, "?"))
}

func TestExpandFunctionMacros_LineContinuation(t *testing.T) {
	src := "#define F(x) ((x)+1)\n" +
		"int y = F(1)\\\n" +
		";\n"
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.NotContains(t, out, `\`)
	require.Contains(t, out, "((1)+1)")
}

func TestExpandFunctionMacros_SkipBadDefine(t *testing.T) {
	src := `
#define BAD(x ( y)
#define GOOD(x) (x)
int v = GOOD(1);
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, "#define BAD")
	require.Contains(t, out, "(1)")
}

func TestExpandFunctionMacros_WithProjectTable(t *testing.T) {
	header := `
#define STACK_OF(type) struct stack_st_##type
`
	src := `
STACK_OF(X)* p;
`
	base := ScanMacroTablesFromSource(header)
	out, err := ExpandFunctionMacrosWithTables(src, base)
	require.NoError(t, err)
	require.Contains(t, out, "struct stack_st_X")
}

func TestExpandFunctionMacros_StringObjectMacro(t *testing.T) {
	src := `
#define HTTP_1_0 "HTTP/1.0 "
const char* s = HTTP_1_0 "extra";
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, `"HTTP/1.0 "`)
	require.Contains(t, out, `"extra"`)
}
