package yak

import (
	"testing"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func TestEngineToLibDocuments(t *testing.T) {
	docs := EngineToLibDocuments(yaklang.New())
	_ = docs
}
