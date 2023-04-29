package main

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"os"
	"yaklang/common/utils"
	"yaklang/common/yak"
	_ "yaklang/common/yak"
	"yaklang/common/yak/yaklang"
)

func main() {
	if len(os.Args) < 2 {
		return
	}

	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(&helper); err != nil {
		panic(err)
	}

	if newBuf, err := utils.GzipCompress(buf.Bytes()); err != nil {
		panic(err)
	} else if err = ioutil.WriteFile(os.Args[1], newBuf, 0666); err != nil {
		panic(err)
	} else {
	}

}
