package java

import (
	"embed"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"os"
	"path/filepath"
	"testing"
)

//go:embed sample/jar/helloworld.jar
var JavaTestFile embed.FS

func GetJarFile() (string, error) {
	dir := os.TempDir()
	jar, err := JavaTestFile.ReadFile("sample/jar/helloworld.jar")
	if err != nil {
		return "", err
	}

	jarPath := filepath.Join(dir, "test.jar")
	err = os.WriteFile(jarPath, jar, 0644)
	if err != nil {
		return "", err
	}
	return jarPath, nil
}

func GetJarContent() ([]byte, error) {
	jar, err := JavaTestFile.ReadFile("sample/jar/helloworld.jar")
	if err != nil {
		return []byte{}, err
	}
	return jar, nil
}

func TestCompile_Jar(t *testing.T) {
	t.Run("test should compile jar file", func(t *testing.T) {
		jarPath, err := GetJarFile()
		require.NoError(t, err)
		// test jar filesystem
		jarFs, err := javaclassparser.NewJarFSFromLocal(jarPath)
		require.NoError(t, err)

		ssatest.CheckWithFS(jarFs, t, func(programs ssaapi.Programs) error {
			programs.Show()
			vals, err := programs.SyntaxFlowWithError(`System.out.println(* as $a)`)
			require.NoError(t, err)
			res := vals.GetValues("a")
			require.Contains(t, res.String(), "Hello world!")
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test should not compile jar file", func(t *testing.T) {
		jarRaw, err := GetJarContent()
		require.NoError(t, err)

		vf := filesys.NewVirtualFs()
		vf.AddFile("B.class", string(jarRaw)) // B.class will skip
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			programs.Show()
			vals, err := programs.SyntaxFlowWithError(`System.out.println(* as $a)`)
			require.NoError(t, err)
			res := vals.GetValues("a")
			require.Equal(t, 0, res.Len())
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}
