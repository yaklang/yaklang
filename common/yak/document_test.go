package yak

import (
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"testing"
)

func TestEngineToLibDocuments(t *testing.T) {
	docs := EngineToLibDocuments(yaklang.New())
	_ = docs
}
