package javaclassparser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestJarFS_NormalizesNestedWindowsPaths(t *testing.T) {
	jarPath, cleanup := createTestJarWithNestedJar(t)
	defer cleanup()

	zipFS, err := filesys.NewZipFSFromLocal(jarPath)
	require.NoError(t, err)
	jarFS := NewJarFS(zipFS)

	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "leading slash", path: "/lib/nested.jar/com", want: "example"},
		{name: "windows separators", path: `\lib\nested.jar\com`, want: "example"},
		{name: "windows class path", path: `\lib\nested.jar\com\example\NestedClass.class`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.HasSuffix(tc.path, ".class") {
				data, err := jarFS.ReadFile(tc.path)
				require.NoError(t, err)
				assert.NotEmpty(t, data)
				return
			}

			entries, err := jarFS.ReadDir(tc.path)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, tc.want, entries[0].Name())
		})
	}
}
