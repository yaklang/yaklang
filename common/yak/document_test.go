package yak

import (
	"testing"

	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func TestEngineToDocumentHelperWithVerboseInfo(t *testing.T) {
	engine := yaklang.New()
	docs := EngineToDocumentHelperWithVerboseInfo(engine)
	_ = docs
}

func TestDocumentHelperWithVerboseInfo(t *testing.T) {
	t.SkipNow()
	m := map[string]any{
		"BinaryRead": binx.BinaryRead,
	}
	docs := DocumentHelperWithVerboseInfo(m)
	_ = docs
}
