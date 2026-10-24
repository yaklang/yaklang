package go2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestGoUnitPackageNamesOrdersImplementationBeforeTest(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a_test.go", "package foo_test\n")
	fs.AddFile("a.go", "package foo\n")
	names := goUnitPackageNames(fs, []string{"a_test.go", "a.go"})
	require.Equal(t, []string{"foo", "foo_test"}, names)
	require.Equal(t, "", goUnitPackageName(fs, []string{"a.go", "a_test.go"}))
	require.Equal(t, "foo", goUnitPackageName(fs, []string{"a.go"}))
}

func TestPartitionKeepsBothPackagesInOneDirectory(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("p/a.go", "package foo\nfunc A() {}\n")
	fs.AddFile("p/a_test.go", "package foo_test\nfunc TestA() {}\n")
	units := (&SSABuilder{}).PartitionCompileUnits(fs, []string{"p/a.go", "p/a_test.go"})
	require.Len(t, units, 1)
	require.Equal(t, []string{"foo", "foo_test"}, units[0].LibraryNames)
	require.Empty(t, units[0].LibraryName)
	require.True(t, units[0].DeclaredLibrary)
}
