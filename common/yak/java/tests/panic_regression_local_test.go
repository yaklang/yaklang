package tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestRealJavaFiles_NoPanicStackOutput(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only panic regression test")
	}
	if os.Getenv("YAK_RUN_JAVA_PANIC_FILE_TEST") == "" {
		t.Skip("set YAK_RUN_JAVA_PANIC_FILE_TEST=1 to run local panic regression files")
	}

	files := []string{
		"/home/wlz/Target/decompiled-code-target/229653a8-4112-4374-b95f-2151c702d832/decompiled/yyt-medicinal-serv-1.0-SNAPSHOT/com/yyt/medical/service/BizElderTakeMedicineInfoService.java",
		"/home/wlz/Target/decompiled-code-target/229653a8-4112-4374-b95f-2151c702d832/decompiled/standard-1.0.6/org/apache/taglibs/standard/extra/spath/SPathParserTokenManager.java",
		"/home/wlz/Target/decompiled-code-target/229653a8-4112-4374-b95f-2151c702d832/decompiled/standard-1.0.6/org/apache/taglibs/standard/extra/spath/TokenMgrError.java",
	}

	for _, filePath := range files {
		filePath := filePath
		t.Run(filepath.Base(filePath), func(t *testing.T) {
			content, err := os.ReadFile(filePath)
			require.NoError(t, err)

			output := captureJavaParseOutput(t, func() {
				prog, parseErr := ssaapi.Parse(
					string(content),
					ssaapi.WithLanguage(ssaconfig.JAVA),
					ssaapi.WithProgramName("panic_regression_"+filepath.Base(filePath)),
				)
				require.NoError(t, parseErr)
				require.NotNil(t, prog)
			})

			require.NotContains(t, output, "Current goroutine call stack")
			require.NotContains(t, output, "panic(")
		})
	}
}

func captureJavaParseOutput(t *testing.T, fn func()) string {
	t.Helper()

	originStdout := os.Stdout
	originStderr := os.Stderr

	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = writer
	os.Stderr = writer
	log.SetOutput(writer)

	outputDone := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		outputDone <- buf.String()
	}()

	fn()

	os.Stdout = originStdout
	os.Stderr = originStderr
	log.SetOutput(originStdout)

	_ = writer.Close()
	output := <-outputDone
	_ = reader.Close()
	return strings.ReplaceAll(output, "\x00", "")
}
