package jsonpath

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/tidwall/gjson"
	"testing"
)

func TestRecursiveDeepJsonPath(t *testing.T) {

	res := RecursiveDeepJsonPath(`{"abc":12,"b":["12",{"efg":4,"ag":"123"}],"c":1}`)
	spew.Dump(res)

	bodyRaw := `{"abc":12,"b":["12",{"efg":4,"ag":"123"}],"c":1}`

	var walk func([]byte, gjson.Result, string, string)
	walk = func(raw []byte, value gjson.Result, gPrefix string, jPrefix string) {
		// 遍历当前层级的所有键
		value.ForEach(func(key, val gjson.Result) bool {
			var jPath string
			if key.Type == gjson.Number {
				jPath = fmt.Sprintf("%s[%d]", jPrefix, key.Int())
			} else {
				jPath = fmt.Sprintf("%s.%s", jPrefix, key.String())
			}
			// 构建当前键的完整路径
			gPath := key.String()
			if gPrefix != "" {
				gPath = gPrefix + "." + key.String()
			}
			fmt.Println("gPath ", gPath)
			fmt.Println("jPath ", jPath)
			// 如果当前值是对象或数组，递归遍历
			if val.IsObject() || val.IsArray() {
				walk(raw, val, gPath, jPath)
			}

			return true // 继续遍历
		})
	}

	// 从根对象开始遍历
	walk([]byte(bodyRaw), gjson.ParseBytes([]byte(bodyRaw)), "", "$")
}
