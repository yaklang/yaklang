package rag

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
)

func TestExports(t *testing.T) {
	Get("test")
	info, err := GetCollectionInfo(consts.GetGormProfileDatabase(), "test")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(info)
}
