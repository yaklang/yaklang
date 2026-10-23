package ssaapi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Several packages import one missing type. Java prehandler runs those units
// concurrently, and every unit materializes the type on the same virtual
// function builder.
func TestParallelJavaMissingImportVirtualBuilder(t *testing.T) {
	const (
		packages = 16
		rounds   = 20
	)
	for round := 0; round < rounds; round++ {
		vf := filesys.NewVirtualFs()
		for i := 0; i < packages; i++ {
			vf.AddFile(fmt.Sprintf("p%d/C.java", i), fmt.Sprintf(`package p%d;
import com.missing.Foo;
public class C {
  Foo f() { return new Foo(); }
  int n(int a) { int r = 0; for (int i = 0; i < a; i++) { r += i; } return r; }
}
`, i))
		}
		cfg, err := DefaultConfig(
			WithFileSystem(vf),
			WithLanguage(ssaconfig.JAVA),
			WithMemory(),
			ssaconfig.WithCompileConcurrency(8),
		)
		require.NoError(t, err)
		_, err = cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
		require.NoError(t, err, "round %d", round)
	}
}
