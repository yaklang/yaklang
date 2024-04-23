package ssaapi

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func TestMultiFile(t *testing.T) {
	outterFile := consts.TempFileFast(`
a = () => {
	return "abc"
}
`)
	defer os.Remove(outterFile)

	check := func(t *testing.T, filename string) {
		filename = strconv.Quote(filename)
		prog, err := Parse(`
include ` + filename + `

result = a()
dump(result)
`)
		if err != nil {
			t.Fatal(err)
		}
		result := prog.Show().Ref("result").GetTopDefs().Get(0)
		spew.Dump(result.String())

		if result.GetConstValue() != "abc" {
			t.Fatal("result is not abc")
		}
	}

	t.Run("absolute path", func(t *testing.T) {
		check(t, outterFile)
	})

	t.Run("relative path", func(t *testing.T) {
		currentDir, err := os.Getwd()
		require.NoError(t, err, "os.Getwd() failed")
		path, err := filepath.Rel(currentDir, outterFile)
		require.NoError(t, err, "filepath.Rel() failed")
		check(t, path)
	})
}
