package main

import (
	"bytes"
	"encoding/gob"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/codecutils"
	"io/ioutil"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		return
	}
	helper := yak.DocumentHelperWithVerboseInfo(map[string]interface{}{
		"newCodecFlow": codecutils.NewCodecExecFlow,
	})
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(&helper); err != nil {
		panic(err)
	}

	if newBuf, err := utils.GzipCompress(buf.Bytes()); err != nil {
		panic(err)
	} else if err = ioutil.WriteFile(os.Args[1], newBuf, 0o666); err != nil {
		panic(err)
	} else {
	}
}
