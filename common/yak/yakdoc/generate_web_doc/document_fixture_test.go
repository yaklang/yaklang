package main

import (
	"runtime/debug"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

var documentFixture struct {
	sync.Once
	helper *yakdoc.DocumentHelper
}

// These checks only read the generated declarations. Build them from the live
// exports once, keeping all libraries/examples in each check without repeatedly
// parsing the source tree and reflecting the same functions. Execution tests
// still create a fresh engine for every example.
func testDocumentHelper(t testing.TB) *yakdoc.DocumentHelper {
	t.Helper()
	documentFixture.Do(func() {
		// Match the generator's workaround for the vendored ANTLR runtime.
		debug.SetGCPercent(-1)
		documentFixture.helper = yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	})
	if len(documentFixture.helper.Libs) == 0 {
		t.Fatal("document fixture contains no exported libraries")
	}
	return documentFixture.helper
}
