package test

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/java/template2java"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const generatedJavaFixtureMaxParseDuration = 30 * time.Second

func checkJavaFront(t *testing.T, code string, fileNames ...string) {
	t.Helper()

	fileName := "generated.java"
	if len(fileNames) > 0 {
		fileName = strings.ReplaceAll(strings.TrimSpace(fileNames[0]), "\\", "/")
		if fileName == "" {
			fileName = "generated.java"
		}
	}
	virtualFileName := path.Clean(fileName)
	if ext := path.Ext(virtualFileName); ext != "" {
		virtualFileName = strings.TrimSuffix(virtualFileName, ext) + ".java"
	} else if !strings.HasSuffix(strings.ToLower(virtualFileName), ".java") {
		virtualFileName += ".java"
	}

	antlr4util.ResetSLLFirstCounters()
	start := time.Now()
	_, err := java2ssa.Frontend(code)
	parseDur := time.Since(start)
	require.NoError(t, err)
	require.LessOrEqual(t, parseDur, generatedJavaFixtureMaxParseDuration, "generated java parse took too long for %s", fileName)

	stats := antlr4util.SLLFirstCountersSnapshot()
	t.Logf("generated java=%s parse=%s sll_attempts=%d fallbacks=%d cancelled=%d errors=%d", fileName, parseDur, stats.SLLAttempts, stats.Fallbacks, stats.FallbackCancelled, stats.FallbackError)

	require.NotPanics(t, func() {
		vf := filesys.NewVirtualFs()
		vf.AddFile(virtualFileName, code)
		progs, err := ssaapi.ParseProjectWithFS(
			vf,
			ssaapi.WithLanguage(ssaconfig.JAVA),
			ssaapi.WithMemory(),
		)
		require.NoError(t, err)
		require.NotEmpty(t, progs)
	}, "generated java build panicked for %s", fileName)
}

func TestCreateJavaTemplate(t *testing.T) {
	tests := []struct {
		fileName  string
		filePath  string
		modelName string
		wants     []string
	}{
		{"demo.jsp", "D:\\java_project\\jspDemo\\src\\main\\webapp\\WEB-INF\\jsp", "demo", []string{
			"public class demo_jsp",
			"out.write(\"<html>\");",
			`public void _JavaTemplateService(HttpServletRequest request, HttpServletResponse response)`,
			`out.print(var1);`,
		}},
		{"++dmo.jsp.", "../com/org", "demo", []string{
			"package tmp2java.com.org;",
			"public class dmo_jsp_",
		}},
		{"qqq.ftl", "/com.org/A", "demo", []string{
			"public class qqq_ftl",
			"package tmp2java.com.org.A;",
		}},
		{"versioned.ftl", "/tmp/decompiled-code-target/229653a8-4112-4374-b95f-2151c702d832/decompiled/MFH-COMN-SERVER-MODULES-1.0/com/hzecool/codegen/freemarker", "demo", []string{
			"public class versioned_ftl",
			"package tmp2java.tmp.decompiled_code_target._229653a8_4112_4374_b95f_2151c702d832.decompiled.MFH_COMN_SERVER_MODULES_1._0.com.hzecool.codegen.freemarker;",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			jt, err := template2java.CreateJavaTemplate(filepath.Join(tt.filePath, tt.fileName))
			require.NoError(t, err)
			require.NotNil(t, jt)
			jt.WritePureText("<html>")
			jt.WriteOutput("var1")
			jt.WriteEscapeOutput("var2")
			jt.Finish()
			fmt.Println(jt.String())
			checkJavaFront(t, jt.String())
			for _, want := range tt.wants {
				require.Contains(t, jt.String(), want)
			}
		})
	}
}
