package main

import (
	"bytes"
	"encoding/gob"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"testing"
)

func TestGetnerateDoc(t *testing.T) {
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(&helper); err != nil {
		t.Fatal(err)
	}
	newBuf, err := utils.GzipCompress(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	var newHelper *yakdoc.DocumentHelper
	newBuf2, err := utils.GzipDeCompress(newBuf)
	if err != nil {
		t.Fatal(err)
	}

	decoder := gob.NewDecoder(bytes.NewReader(newBuf2))
	if err := decoder.Decode(&newHelper); err != nil {
		t.Fatalf("load embed yak document error: %v", err)
	}

}
