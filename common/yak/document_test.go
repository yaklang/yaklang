package yak

import (
	"testing"
	"yaklang/common/yak/yaklang"
)

func TestEngineToLibDocuments(t *testing.T) {
	docs := EngineToLibDocuments(yaklang.New())
	_ = docs
}
