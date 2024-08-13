package yak

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func TestEngineToDocumentHelperWithVerboseInfo(t *testing.T) {
	engine := yaklang.New()
	docs := EngineToDocumentHelperWithVerboseInfo(engine)
	_ = docs
}
