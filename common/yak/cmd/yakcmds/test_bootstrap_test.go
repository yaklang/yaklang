package yakcmds_test

import (
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/utils/yakitenv"
)

// The process owns a fresh test home. Import the builtin fixtures once instead
// of relying on the developer's installation or a different CI test process.
func TestMain(m *testing.M) {
	if yakitenv.TestRoot() == "" {
		panic("builtin fixtures require an isolated test environment")
	}
	coreplugin.InitDBForTest()
	os.Exit(m.Run())
}
