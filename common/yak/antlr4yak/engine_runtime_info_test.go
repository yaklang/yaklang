package antlr4yak

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestEngineRuntimeInfoLineRejectsFrameWithoutCurrentCode(t *testing.T) {
	engine := New()
	var infoErr error
	err := engine.GetVM().Exec(context.Background(), func(_ *yakvm.Frame) {
		_, infoErr = engine.RuntimeInfo("line")
	})
	if err != nil {
		t.Fatal(err)
	}
	if infoErr == nil {
		t.Fatal("Engine.RuntimeInfo(line) accepted a frame without code")
	}
}
