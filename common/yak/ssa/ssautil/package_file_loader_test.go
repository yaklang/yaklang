package ssautil

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func ToString(r io.Reader) string {
	raw, _ := io.ReadAll(r)
	return string(raw)
}

func Test_PackageLoader(t *testing.T) {

	// init package loader
	loader := NewPackageLoader(
		WithIncludePath("testdata"),
	)

	t.Run("check file in include path", func(t *testing.T) {
		// check file in include path
		if _, data, err := loader.LoadFilePackage("index.txt", false); err != nil {
			t.Fatal(err)
		} else {
			require.Equalf(t, "index", data.GetSourceCode(), "LoadFilePackage failed for index.txt, got: %s", data.GetSourceCode())
		}
	})

	t.Run("check file in include path once", func(t *testing.T) {
		// check file in include path
		if _, data, err := loader.LoadFilePackage("index.txt", true); err != nil {
			t.Fatal(err)
		} else {
			require.Equalf(t, "index", data.GetSourceCode(), "LoadFilePackage failed for index.txt, got: %s", data.GetSourceCode())
		}
		if _, _, err := loader.LoadFilePackage("index.txt", true); err == nil {
			t.Fatalf("LoadFilePackage should failed for index.txt")
		}
	})

	t.Run("check file not in include path", func(t *testing.T) {
		// check file not in include path
		if _, _, err := loader.LoadFilePackage("notexist.txt", false); err == nil {
			t.Fatal("LoadFilePackage should failed for notexist.txt")
		}

	})
	t.Run("add include path test", func(t *testing.T) {
		// add include path
		loader.AddIncludePath("testdata/b/c")
		if _, data, err := loader.LoadFilePackage("c.txt", false); err != nil {
			t.Fatal(err)
		} else {
			require.Equalf(t, "c", data.GetSourceCode(), "LoadFilePackage failed for c.txt, got: %s", data.GetSourceCode())
		}
	})

	t.Run("check directory in include path", func(t *testing.T) {
		ch, err := loader.LoadDirectoryPackage("b", false)
		require.NoError(t, err, "LoadDirectoryPackage failed for c", err)
		filepath := make([]string, 0)
		for v := range ch {
			filepath = append(filepath, v.FileName)
		}
		require.Equal(t, []string{"testdata/b/b.txt"}, filepath, "LoadDirectoryPackage failed for b")
	})
}
