package yak

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func TestEngineToDocumentHelperWithVerboseInfo(t *testing.T) {
	docs := EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	_ = docs
}
