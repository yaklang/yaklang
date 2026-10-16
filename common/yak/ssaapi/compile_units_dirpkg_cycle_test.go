package ssaapi

import (
	"fmt"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

// compileUnitWorker matches prehandleBatchEnv.unitWorker for a library name.
// workers == 2 pins every library to worker 1, so a 2-cycle never overlaps.
func compileUnitWorker(libraryName string, workers int) int {
	if workers <= 1 {
		return 0
	}
	if workers == 2 {
		return 1
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(libraryName))
	return int(h.Sum32()%uint32(workers-1)) + 1
}

func splitPackageNames(workers int) (string, string) {
	for i := 0; i < 64; i++ {
		left := fmt.Sprintf("cyc%d", i)
		for j := i + 1; j < 64; j++ {
			right := fmt.Sprintf("cyc%d", j)
			if compileUnitWorker(left, workers) != compileUnitWorker(right, workers) {
				return left, right
			}
		}
	}
	return "leftpkg", "rightpkg"
}

// Two project packages import each other and are pre-handled on different
// workers. Each import misses GetLibrary (the bool is "this file belongs to
// that library", not "the library exists") and calls BuildDirectoryPackage.
// The directory compile then imports the other package. If each worker owns
// one slot and waits for the other, this test hangs.
func TestDirectoryPackageMutualImportDoesNotDeadlock(t *testing.T) {
	const workers = 8
	left, right := splitPackageNames(workers)
	require.NotEqual(t, compileUnitWorker(left, workers), compileUnitWorker(right, workers),
		"fixture must land on two workers or it cannot see the cross-goroutine wait")

	vf := filesys.NewVirtualFs()
	vf.AddFile(left+"/L.java", fmt.Sprintf(`package %s;
import %s.R;
public class L {
  public static String value() { return "from-left"; }
  public static void run() { System.out.println(R.value()); }
}
`, left, right))
	vf.AddFile(right+"/R.java", fmt.Sprintf(`package %s;
import %s.L;
public class R {
  public static String value() { return L.value(); }
}
`, right, left))

	cfg, err := DefaultConfig(
		WithFileSystem(vf),
		WithLanguage(ssaconfig.JAVA),
		WithMemory(),
		ssaconfig.WithCompileConcurrency(workers),
		WithASTOrder(ssareducer.Order),
	)
	require.NoError(t, err)
	program, err := cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.NotNil(t, program)
	result, err := program.SyntaxFlowWithError(`println(* #-> * as $source)`)
	require.NoError(t, err)
	require.Contains(t, result.GetValues("source").String(), "from-left")
}
