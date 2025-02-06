package ssaapi_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestMultiFile(t *testing.T) {
	currentDir, err := os.MkdirTemp("", "multiple_file")
	require.NoError(t, err)

	outterFile, err := os.Create(path.Join(currentDir, "a.yak"))
	require.NoError(t, err)
	code := `
a = () => {
	return "abc"
}
`
	_, err = outterFile.WriteString(code)
	require.NoError(t, err)
	defer os.Remove(outterFile.Name())

	mainFP, err := os.Create(path.Join(currentDir, "main.yak"))
	mainFP.Close()
	require.NoError(t, err)
	defer os.Remove(mainFP.Name())

	// get name
	mainFile, err := filepath.Abs(mainFP.Name())
	_ = mainFile
	require.NoError(t, err)

	check := func(t *testing.T, filename string) {
		filename = strconv.Quote(filename)
		code := fmt.Sprintf(`
include ` + filename + `

result = a()
dump(result)
		`)
		err := os.WriteFile(mainFile, []byte(code), 0644)
		require.NoError(t, err)

		ssatest.CheckSyntaxFlowWithFS(t,
			filesys.NewRelLocalFs(currentDir),
			`result #-> as $result`,
			map[string][]string{
				"result": {"abc"},
			}, true,
			ssaapi.WithFileSystemEntry(mainFile),
			ssaapi.WithLanguage(ssaapi.Yak),
		)
	}

	t.Run("absolute path", func(t *testing.T) {
		// get outerFile absolute path
		path, err := filepath.Abs(outterFile.Name())
		require.NoError(t, err)
		check(t, path)
	})

	t.Run("relative path", func(t *testing.T) {
		path, err := filepath.Rel(currentDir, outterFile.Name())
		require.NoError(t, err, "filepath.Rel() failed")
		check(t, path)
	})
}
