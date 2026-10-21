package ssaapi

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// TestPreparedSourceParsesWithJava2SSA is the source-stage contract: whatever
// prepareProjectSources leaves behind for the compile must be Java that
// java2ssa's ANTLR frontend accepts. It covers both halves of the preparation —
// a class decompiled out of an archive and a JSP page turned into a servlet.
func TestPreparedSourceParsesWithJava2SSA(t *testing.T) {
	jarBytes, err := os.ReadFile("test/ssatest/testfile/test.jar")
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	// Named without "test": the default compile excludes drop *test*.jar.
	vf.AddFile("WEB-INF/lib/sample.jar", string(jarBytes))
	vf.AddFile("webapp/index.jsp", `<html><body>${fn:escapeXml(param.name)}</body></html>`)
	vf.AddFile("webapp/legacy.jsp", `<%@ page pageEncoding="GBK" %><c:if test="${!empty param.uid}">uid</c:if>`)

	cfg, err := DefaultConfig(WithFileSystem(vf), WithProgramPath("."))
	require.NoError(t, err)

	// The source stage runs before ScanProjectFiles, so the builder must be
	// resolved here or the archive wrap never happens.
	cfg.resolveLanguageBuilder(vf)
	require.NotNil(t, cfg.LanguageBuilder)
	require.Equal(t, "java", string(cfg.LanguageBuilder.GetLanguage()))

	wrapped := cfg.swapLanguageFs(vf)
	cfg.prepareProjectSources(wrapped, nil, nil)

	materializer := filesys.MaterializerOf(wrapped)
	require.NotNil(t, materializer, "prepare must run against the materialized filesystem")
	stats := materializer.Stats()
	require.Greater(t, stats.Paths, int64(0), "prepare materialized nothing")
	// Walk what the prepare stage actually stored. Both the decompiled class and
	// the generated servlet must be parseable Java.
	var checkedJava, checkedJSP int
	var failures []string
	err = filesys.Recursive(".",
		filesys.WithFileSystem(materializer.VirtualFS()),
		filesys.WithFileStat(func(path string, info os.FileInfo) error {
			if info == nil || info.IsDir() {
				return nil
			}
			lower := strings.ToLower(path)
			if !strings.HasSuffix(lower, ".java") && !strings.HasSuffix(lower, ".jsp") {
				return nil
			}
			src, err := materializer.VirtualFS().ReadFile(path)
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
	t.Logf("prepared: %d java, %d jsp, materializer %s", checkedJava, checkedJSP,
		filesys.FormatSourceMaterializerStats(stats))
}
