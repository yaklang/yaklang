package preprocess

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestApplyDefineLineObject(t *testing.T) {
	tables := NewMacroTables()
	ApplyDefineLine("#define A 1", &tables, false)
	require.Equal(t, "1", tables.Object["A"])
}

func TestCollectObjectMacroFromInclude(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/a.h", []byte("#define A 1\n"), 0o644))
	require.NoError(t, fs.WriteFile("apps/foo.c", []byte("#include \"a.h\"\n"), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include"}
	project := BuildProject(fs, cfg)
	tables := project.collectMacroEnvironment("apps/foo.c", ppMustRead(fs, "apps/foo.c"))
	require.Equal(t, "1", tables.Object["A"])
}

func TestRegistry_HInAlias(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/openssl/safestack.h.in", []byte("#define STACK_OF(type) struct stack_st_##type\n"), 0o644))
	reg := BuildHeaderRegistry(fs)
	_, ok := reg.Lookup("include/openssl/safestack.h")
	require.True(t, ok)
	_, ok = reg.Lookup("openssl/safestack.h")
	require.True(t, ok)
}

func TestResolver_QuotedInclude(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("apps/include/apps.h", []byte("#define APP 1\n"), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"apps/include", "include"}
	reg := BuildHeaderRegistry(fs)
	res := NewIncludeResolver(reg, cfg)
	stored, ok := res.Resolve("apps.h", false, "apps/verify.c")
	require.True(t, ok)
	require.Contains(t, stored, "apps.h")
}

func TestResolver_AngleInclude_HIn(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/openssl/safestack.h.in", []byte("#define STACK_OF(type) struct stack_st_##type\n"), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include", "include/openssl"}
	reg := BuildHeaderRegistry(fs)
	res := NewIncludeResolver(reg, cfg)
	stored, ok := res.Resolve("openssl/safestack.h", true, "apps/foo.c")
	require.True(t, ok)
	content, ok := reg.Lookup(stored)
	require.True(t, ok)
	require.Contains(t, string(content.Content), "STACK_OF")
}

func TestCond_Ifndef(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("apps/include/feature.h", []byte("#define FEATURE 1\n"), 0o644))
	require.NoError(t, fs.WriteFile("apps/foo.c", []byte(`
#include "feature.h"
#ifndef FEATURE
int disabled = 1;
#else
int enabled = 1;
#endif
`), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"apps/include", "include"}
	project := BuildProject(fs, cfg)
	out, err := project.PreprocessTU("apps/foo.c", ppMustRead(fs, "apps/foo.c"))
	require.NoError(t, err)
	require.NotContains(t, out, "disabled")
	require.Contains(t, out, "enabled")
}

func TestCond_Defined(t *testing.T) {
	fs := filesys.NewVirtualFs()
	cfg := DefaultConfig()
	cfg.Defines["OPENSSL_NO_SRP"] = "1"
	require.NoError(t, fs.WriteFile("apps/foo.c", []byte(`
#ifdef OPENSSL_NO_SRP
int no_srp = 1;
#else
int has_srp = 1;
#endif
`), 0o644))
	project := BuildProject(fs, cfg)
	out, err := project.PreprocessTU("apps/foo.c", ppMustRead(fs, "apps/foo.c"))
	require.NoError(t, err)
	require.Contains(t, out, "no_srp")
	require.NotContains(t, out, "has_srp")
}

func TestMacroEnv_IncludeOrder(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/a.h", []byte("#define A 1\n"), 0o644))
	require.NoError(t, fs.WriteFile("include/b.h", []byte("#define B 2\n"), 0o644))
	require.NoError(t, fs.WriteFile("apps/foo.c", []byte(`
#include "a.h"
int x = A;
`), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include"}
	project := BuildProject(fs, cfg)
	out, err := project.PreprocessTU("apps/foo.c", ppMustRead(fs, "apps/foo.c"))
	require.NoError(t, err)
	require.Contains(t, strings.ReplaceAll(out, " ", ""), "x=1")
	require.NotContains(t, out, "B")
}

func TestTU_PreservesMultilineComment(t *testing.T) {
	fs := filesys.NewVirtualFs()
	src := `/*
 * Copyright 2025-2026 The OpenSSL Project Authors.
 */
#include <stdio.h>
int main() { return 0; }
`
	require.NoError(t, fs.WriteFile("apps/configutl.c", []byte(src), 0o644))
	project := BuildProject(fs, DefaultConfig())
	out, err := project.PreprocessTU("apps/configutl.c", src)
	require.NoError(t, err)
	require.Contains(t, out, "/*")
	require.Contains(t, out, "Copyright 2025-2026")
	require.Contains(t, out, "*/")
}

func TestExpandFunctionMacros_PreservesMultilineComment(t *testing.T) {
	src := "/*\n * header\n */\nint x = 1;\n"
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, "/*")
	require.Contains(t, out, "* header")
	require.Contains(t, out, "*/")
}

func TestExpandFunctionMacros_PreservesCharLiteral(t *testing.T) {
	src := `
#define F(x) (x)
int f(int c) {
    switch (c) {
    case '\n': return 1;
    case '\t': return 2;
    default: return F(c);
    }
}
`
	out, err := ExpandFunctionMacros(src)
	require.NoError(t, err)
	require.Contains(t, out, `case '\n':`)
	require.Contains(t, out, `case '\t':`)
}

func TestTU_StackOf(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/openssl/safestack.h.in", []byte("#define STACK_OF(type) struct stack_st_##type\n"), 0o644))
	require.NoError(t, fs.WriteFile("apps/foo.c", []byte(`
#include <openssl/safestack.h>
STACK_OF(X)* p;
`), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include", "include/openssl"}
	project := BuildProject(fs, cfg)
	out, err := project.PreprocessTU("apps/foo.c", ppMustRead(fs, "apps/foo.c"))
	require.NoError(t, err)
	require.Contains(t, out, "struct stack_st_X")
	require.Contains(t, out, "#include <openssl/safestack.h>")
}

// TestTU_ConfigutlPattern mimics configutl.c: conf.h include chain + STACK_OF in function body.
func TestTU_ConfigutlPattern(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/openssl/safestack.h.in", []byte(`#ifndef OPENSSL_SAFESTACK_H
#define OPENSSL_SAFESTACK_H
#define STACK_OF(type) struct stack_st_##type
#endif
`), 0o644))
	require.NoError(t, fs.WriteFile("include/openssl/conf.h.in", []byte(`#ifndef OPENSSL_CONF_H
#define OPENSSL_CONF_H
#include <openssl/safestack.h>
#endif
`), 0o644))
	src := `#include <openssl/conf.h>
#include <openssl/safestack.h>
static void print_section(void) {
    STACK_OF(CONF_VALUE) *values;
}
`
	require.NoError(t, fs.WriteFile("apps/configutl.c", []byte(src), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include", "include/openssl"}
	project := BuildProject(fs, cfg)
	out, err := project.PreprocessTU("apps/configutl.c", src)
	require.NoError(t, err)
	require.NotContains(t, out, "STACK_OF(CONF_VALUE)")
	require.Contains(t, out, "struct stack_st_CONF_VALUE")
}

func TestCollect_ConfigutlPatternHasStackOf(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/openssl/safestack.h.in", []byte("#define STACK_OF(type) struct stack_st_##type\n"), 0o644))
	require.NoError(t, fs.WriteFile("include/openssl/conf.h.in", []byte("#include <openssl/safestack.h>\n"), 0o644))
	src := "#include <openssl/conf.h>\n"
	require.NoError(t, fs.WriteFile("apps/configutl.c", []byte(src), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include", "include/openssl"}
	project := BuildProject(fs, cfg)
	tables := project.collectMacroEnvironment("apps/configutl.c", src)
	_, ok := tables.Function["STACK_OF"]
	require.True(t, ok, "STACK_OF must be collected via conf.h -> safestack.h chain")
}

func TestResolver_SystemIncludeIncludePrefix(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/openssl/safestack.h.in", []byte("#define X 1\n"), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include"}
	reg := BuildHeaderRegistry(fs)
	res := NewIncludeResolver(reg, cfg)
	stored, ok := res.Resolve("openssl/safestack.h", true, "apps/configutl.c")
	require.True(t, ok)
	require.Contains(t, stored, "safestack")
}

func TestMacroEnv_UndefShadow(t *testing.T) {
	fs := filesys.NewVirtualFs()
	require.NoError(t, fs.WriteFile("include/a.h", []byte("#define F(x) (x)\n#undef F\n"), 0o644))
	require.NoError(t, fs.WriteFile("apps/foo.c", []byte(`
#include "a.h"
int y = F(1);
`), 0o644))
	cfg := DefaultConfig()
	cfg.IncludeDirs = []string{"include"}
	project := BuildProject(fs, cfg)
	out, err := project.PreprocessTU("apps/foo.c", ppMustRead(fs, "apps/foo.c"))
	require.NoError(t, err)
	require.Contains(t, out, "F(1)")
}

func ppMustRead(fs *filesys.VirtualFS, path string) string {
	data, err := fs.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}
