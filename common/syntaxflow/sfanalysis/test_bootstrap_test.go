package sfanalysis

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils/yakitenv"
)

// The process owns a fresh test home. Import the builtin fixtures once instead
// of relying on the developer's installation or a different CI test process.
func TestMain(m *testing.M) {
	if yakitenv.TestRoot() == "" {
		panic("builtin fixtures require an isolated test environment")
	}
	if err := sfbuildin.SyncEmbedRule(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
