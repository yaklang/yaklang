package yaklib

import (
	"encoding/xml"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func Escape(s []byte) string {
	var w strings.Builder
	xml.Escape(&w, s)
	return w.String()
}

func _xmldumps(v interface{}) []byte {
	buf, err := xml.Marshal(v)
	if err != nil {
		return []byte{}
	}
	return buf
}

func _xmlloads(v interface{}) interface{} {
	var i interface{}
	buf := utils.InterfaceToBytes(v)
	err := xml.Unmarshal(buf, &i)
	if err != nil {
		return nil
	}
	return i
}

var XMLExports = map[string]interface{}{
	"Marshal":   xml.Marshal,
	"UnMarshal": xml.Unmarshal,
	"dumps":     _xmldumps,
	"loads":     _xmlloads,
}
