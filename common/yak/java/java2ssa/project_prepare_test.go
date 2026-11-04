package ssautils

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
)

// TestPrepareReleasesJSPRuntimeBeforePrehandler is the memory boundary: JSP
// prediction state is scratch for generating the source tree. Once that Java
// is stored, the shared DFA must be empty before prehandler starts.
func TestPrepareReleasesJSPRuntimeBeforePrehandler(t *testing.T) {
	jspparser.ResetJSPRuntimeCaches()
	lexer := jspparser.NewJSPLexer(antlr.NewInputStream(`<html><body>seed</body></html>`))
	parser := jspparser.NewJSPParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	parser.RemoveErrorListeners()
	_ = parser.JspDocuments()
	require.Greater(t, jspparser.JSPRuntimeDFAStates(), 0)

	vf := filesys.NewVirtualFs()
	vf.AddFile("webapp/index.jsp", `<html><body>hello</body></html>`)
	builder, ok := java2ssa.CreateBuilder().(*java2ssa.SSABuilder)
	require.True(t, ok)
	wrapped := builder.WrapWithPreprocessedFS(vf, true)

	sources := NewPreparedSources(0, "")
	defer sources.Close()
	sources.Prepare(wrapped, PrepareOptions{Root: ".", Concurrency: 2})

	require.Zero(t, jspparser.JSPRuntimeDFAStates())
}

// TestPreparedSourceParsesWithJava2SSA is the source-stage contract: whatever
// Prepare leaves behind for the compile must be Java that java2ssa's ANTLR
// frontend accepts. It covers both halves — a class decompiled out of an
// archive and a JSP page turned into a servlet.
func TestPreparedSourceParsesWithJava2SSA(t *testing.T) {
	jarBytes, err := os.ReadFile("../ssaapi/test/ssatest/testfile/test.jar")
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	// Named without "test": the default compile excludes drop *test*.jar.
	vf.AddFile("WEB-INF/lib/sample.jar", string(jarBytes))
	vf.AddFile("webapp/index.jsp", `<html><body>${fn:escapeXml(param.name)}</body></html>`)
	vf.AddFile("webapp/legacy.jsp", `<%@ page pageEncoding="GBK" %><c:if test="${!empty param.uid}">uid</c:if>`)

	builder, ok := java2ssa.CreateBuilder().(*java2ssa.SSABuilder)
	require.True(t, ok)
	wrapped := builder.WrapWithPreprocessedFS(vf, true)

	sources := NewPreparedSources(0, "")
	defer sources.Close()
	fsys := sources.Wrap(wrapped)
	sources.Prepare(fsys, PrepareOptions{Root: ".", Concurrency: 4})

	stats := sources.Stats()
	require.Greater(t, stats.Paths, int64(0), "prepare materialized nothing")
	// Walk what the prepare stage actually served. Both the decompiled class
	// and the generated servlet must be parseable Java.
	var checkedJava, checkedJSP int
	var failures []string
	err = filesys.Recursive(".",
		filesys.WithFileSystem(fsys),
		filesys.WithFileStat(func(path string, info os.FileInfo) error {
			if info == nil || info.IsDir() {
				return nil
			}
			lower := strings.ToLower(path)
			if !strings.HasSuffix(lower, ".java") && !strings.HasSuffix(lower, ".jsp") {
				return nil
			}
			src, err := fsys.ReadFile(path)
			if err != nil {
				failures = append(failures, path+": read: "+err.Error())
				return nil
			}
			if strings.HasSuffix(lower, ".jsp") {
				if !strings.Contains(string(src), "void _JavaTemplateService(") {
					failures = append(failures, path+": jsp was not converted to java")
					return nil
				}
				checkedJSP++
			} else {
				checkedJava++
			}
			if _, err := java2ssa.Frontend(string(src)); err != nil {
				msg := err.Error()
				if len(msg) > 200 {
					msg = msg[:200]
				}
				failures = append(failures, path+": java: "+msg)
			}
			return nil
		}),
	)
	require.NoError(t, err)
	require.Greater(t, checkedJava, 0, "the jar class must be decompiled into a .java source")
	require.Greater(t, checkedJSP, 0, "the jsp pages must be converted")
	require.Empty(t, failures, "prepared sources must parse with java2ssa")
	t.Logf("prepared: %d java, %d jsp, %s", checkedJava, checkedJSP, filesys.FormatSpillFSStats(stats))
}

// TestPrepareMaterializeOutWritesUsableJava checks the tree mirror: classes
// become .java files, pages become page.jsp.java and never collide.
func TestPrepareMaterializeOutWritesUsableJava(t *testing.T) {
	dir := t.TempDir()
	vf := filesys.NewVirtualFs()
	vf.AddFile("pkg/A.class", "class A {}")
	vf.AddFile("web/index.jsp", `<html>${1}</html>`)

	sources := NewPreparedSources(0, "")
	defer sources.Close()
	sources.SetMaterializeOut(dir)
	// The publisher writes whatever the cache stores; this test seeds the two
	// shapes directly because real class decompilation is covered above.
	sources.spill.Put("pkg/A.class", []byte("class A {}"))
	sources.spill.Put("web/index.jsp", []byte("class Index {}"))

	javaBytes, err := os.ReadFile(dir + "/pkg/A.java")
	require.NoError(t, err)
	require.Equal(t, "class A {}", string(javaBytes))
	jspBytes, err := os.ReadFile(dir + "/web/index.jsp.java")
	require.NoError(t, err)
	require.Equal(t, "class Index {}", string(jspBytes))

	rel, err := treeRel("../../etc/passwd.java")
	require.NoError(t, err)
	require.NotContains(t, rel, "..", "treeRel must not escape the output root")
}
