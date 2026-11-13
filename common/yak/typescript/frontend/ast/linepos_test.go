package ast_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/parser"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
)

// Converting a byte position to a line/character pair used to recount runes
// from the line start on every call. Minified bundles put megabytes on one
// line, so the per-node conversion became quadratic: ace-builds worker files
// spent 85% of a CPU window inside RuneCountInString.
func TestGetLineAndCharacterOfPositionIsNotQuadraticOnLongLines(t *testing.T) {
	const lineBytes = 1 << 20
	line := `var longName = "` + strings.Repeat("a", lineBytes) + `";`
	source := line + "\nvar second = 1;\n"
	var file *ast.SourceFile = parser.ParseSourceFile("long.js", "", source, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)

	start := time.Now()
	const probes = 4000
	for i := 0; i < probes; i++ {
		pos := len(line) * i / probes
		lineNo, character := scanner.GetLineAndCharacterOfPosition(file, pos)
		require.Equal(t, 0, lineNo)
		require.Equal(t, pos, character, "ASCII byte offset must equal the character offset")
	}
	elapsed := time.Since(start)
	require.Less(t, elapsed, time.Second,
		"%d position conversions on a %d byte line took %v; they must not rescan the line", probes, lineBytes, elapsed)

	// The second, short line still resolves normally.
	lineNo, character := scanner.GetLineAndCharacterOfPosition(file, len(line)+1)
	require.Equal(t, 1, lineNo)
	require.Equal(t, 0, character)
}

// Lines with multibyte runes must keep counting runes, not bytes.
func TestGetLineAndCharacterOfPositionCountsRunesOnMultibyteLines(t *testing.T) {
	source := "var 名字 = \"值\";\nvar tail = 1;\n"
	var file *ast.SourceFile = parser.ParseSourceFile("unicode.js", "", source, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
	require.False(t, file.IsASCIILine(0), "a line with CJK identifiers is not ASCII")

	for _, probe := range []string{"名字", "值", "\"值\"", "="} {
		pos := strings.Index(source, probe)
		require.GreaterOrEqual(t, pos, 0)
		lineNo, character := scanner.GetLineAndCharacterOfPosition(file, pos)
		require.Equal(t, 0, lineNo)
		require.Equal(t, utf8.RuneCountInString(source[:pos]), character,
			"character offset of %q must count runes", probe)
	}

	lineNo, character := scanner.GetLineAndCharacterOfPosition(file, strings.Index(source, "tail"))
	require.Equal(t, 1, lineNo)
	require.Equal(t, 4, character)
}
