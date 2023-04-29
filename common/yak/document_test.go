package yak

import (
	"yaklang/common/yak/yaklang"
	"testing"
)

func TestEngineToLibDocuments(t *testing.T) {
	docs := EngineToLibDocuments(yaklang.New())
	_ = docs
}
