package yak

import (
	"testing"
	"yaklang.io/yaklang/common/yak/yaklang"
)

func TestEngineToLibDocuments(t *testing.T) {
	docs := EngineToLibDocuments(yaklang.New())
	_ = docs
}
