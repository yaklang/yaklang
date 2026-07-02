package preprocess

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	cparser "github.com/yaklang/yaklang/common/yak/antlr4c/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func loadTinycppTestFS(t *testing.T) *filesys.VirtualFS {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(file), "testdata", "tinycpp")
	local := filesys.NewRelLocalFs(root)
	vf := filesys.NewVirtualFs()
	err := filesys.Recursive(".", filesys.WithFileSystem(local), filesys.WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
		if isDir {
			return nil
		}
		data, err := local.ReadFile(pathname)
		if err != nil {
			return err
		}
		vf.AddFile(pathname, string(data))
		return nil
	}))
	require.NoError(t, err)
	return vf
}

func tinycppCFiles(t *testing.T, vf *filesys.VirtualFS) []string {
	t.Helper()
	var names []string
	err := filesys.Recursive(".", filesys.WithFileSystem(vf), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".c") {
			return nil
		}
		names = append(names, filepath.ToSlash(path))
		return nil
	}))
	require.NoError(t, err)
	require.NotEmpty(t, names)
	return names
}

func assertLibulzMacroExpanded(t *testing.T, out string) {
	t.Helper()
	require.NotContains(t, out, "tglist(char*)", "tglist type macro should expand")
	require.NotContains(t, out, "hbmap(char*", "hbmap type macro should expand")
	require.NotContains(t, out, "hbmap_foreach(cpp", "hbmap_foreach should expand to for-loop")
	require.NotContains(t, out, "tglist_foreach(&cpp", "tglist_foreach should expand to for-loop")
	require.NotContains(t, out, "tglist _impl", "token paste tglist_impl should expand")
}

func assertLibulzIncludesProcessed(t *testing.T, out string) {
	t.Helper()
	require.NotContains(t, out, `#include "tglist.h"`)
	require.NotContains(t, out, `#include "hbmap.h"`)
	require.NotContains(t, out, `#include "bmap.h"`)
}

func assertMacroStructFieldsExpanded(t *testing.T, out string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "argnames") && strings.Contains(line, "struct macro") {
			require.NotContains(t, line, "tglist", "struct field should use expanded anonymous struct")
		}
	}
}

func parseCSource(src string) error {
	_, err := antlr4util.ParseASTWithSLLFirst(
		src,
		cparser.NewCLexer,
		cparser.NewCParser,
		nil,
		nil,
		func(parser *cparser.CParser) *cparser.CompilationUnitContext {
			return parser.CompilationUnit().(*cparser.CompilationUnitContext)
		},
	)
	return err
}

func TestTU_Tinycpp(t *testing.T) {
	vf := loadTinycppTestFS(t)
	project := BuildProject(vf, DefaultConfig())

	for _, name := range tinycppCFiles(t, vf) {
		src, err := vf.ReadFile(name)
		require.NoError(t, err, name)

		out, err := project.PreprocessTU(name, string(src))
		require.NoError(t, err, name)

		if name == "preproc.c" || name == "preproc_snippet.c" {
			assertLibulzMacroExpanded(t, out)
			assertLibulzIncludesProcessed(t, out)
			assertMacroStructFieldsExpanded(t, out)
		}

		if name == "cppmain.c" || name == "tokenizer.c" {
			parseErr := parseCSource(out)
			require.NoError(t, parseErr, "AST parse failed for %s", name)
		}
	}
}
