package java2ssa

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// A directory unit is one worker, because its files share one builder. An
// archive of obfuscated classes carries no package declaration, so every
// decompiled class in it lands in one dir: unit; without sharding that whole
// archive compiles on one worker while the others idle. Overflow units keep
// every unit bounded. The files are .java because the decompiled members reach
// this stage already mapped from .class to .java.
func TestPartitionCompileUnitsShardsOversizedDirectoryUnits(t *testing.T) {
	vf := filesys.NewVirtualFs()
	const classes = javaMaxFilesPerUnit*2 + 7
	files := make([]string, 0, classes)
	for i := 0; i < classes; i++ {
		name := fmt.Sprintf("webinst.jar/a/C%d.java", i)
		vf.AddFile(name, "class C {}")
		files = append(files, name)
	}

	units := (&SSABuilder{}).PartitionCompileUnits(vf, files)

	total := 0
	for _, unit := range units {
		total += len(unit.Files)
		require.LessOrEqual(t, len(unit.Files), javaMaxFilesPerUnit,
			"unit %s holds %d files", unit.Key, len(unit.Files))
	}
	require.Equal(t, classes, total, "sharding must keep every file")

	// Three units of at most javaMaxFilesPerUnit cover 135 files.
	var shardKeys []string
	for _, unit := range units {
		require.True(t, strings.HasPrefix(unit.Key, "dir:"), "unit key %s", unit.Key)
		if strings.Contains(unit.Key, "/__shard") {
			shardKeys = append(shardKeys, unit.Key)
		}
	}
	require.Len(t, units, 3)
	require.Len(t, shardKeys, 2, "two overflow units are expected: %v", shardKeys)
}

// Package units are not sharded: their classes reference each other directly,
// and splitting them would break that reference graph.
func TestPartitionCompileUnitsKeepsPackageUnitsWhole(t *testing.T) {
	vf := filesys.NewVirtualFs()
	const classes = javaMaxFilesPerUnit + 10
	files := make([]string, 0, classes)
	for i := 0; i < classes; i++ {
		name := fmt.Sprintf("src/app/C%d.java", i)
		vf.AddFile(name, "package app;\nclass C {}")
		files = append(files, name)
	}

	units := (&SSABuilder{}).PartitionCompileUnits(vf, files)

	require.Len(t, units, 1)
	require.Equal(t, "java:app", units[0].Key)
	require.Len(t, units[0].Files, classes)
}

// Small directory units stay single: sharding only applies to overflow.
func TestPartitionCompileUnitsLeavesSmallDirectoryUnitsAlone(t *testing.T) {
	vf := filesys.NewVirtualFs()
	files := []string{"lib/a/A.java", "lib/a/B.java"}
	for _, name := range files {
		vf.AddFile(name, "class X {}")
	}

	units := (&SSABuilder{}).PartitionCompileUnits(vf, files)

	require.Len(t, units, 1)
	require.Equal(t, "dir:lib/a", units[0].Key)
	require.Len(t, units[0].Files, 2)
}
