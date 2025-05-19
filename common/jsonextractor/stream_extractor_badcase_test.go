package jsonextractor

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
)

func TestJSONinONE(t *testing.T) {
	data := `{
  "name": "测试数据",
  "version": 1.0,
  "description": "用于测试JSON解析器的示例数据。",
  "isActive": true,
  "score": null,
  "configuration": {
    "isEnabled": false,
    "retryAttempts": 3,
    "settings": {
      "timeout": 5000,
      "mode": "strict",
      "advanced": {
        "featureA": true,
        "featureB": "off",
        "featureC": [1, 2, 3, "mixed"]
      }
    }
  },
  "emptyObject": {},
  "emptyArray": [],
  "unicodeString": "你好，世界！🌍"
}
`

	unicodeString := ""
	ExtractJSONStream(
		data,
		WithObjectCallback(func(data map[string]any) {
			fmt.Println("-------------------------------")
			if result, ok := data[`unicodeString`]; ok {
				unicodeString = fmt.Sprint(result)
			}
			fmt.Println("-------------------------------")
		}),
	)
	fmt.Println(unicodeString)
	assert.Equal(t, unicodeString, "你好，世界！🌍")
}

func TestStreamExtractor_BadCase(t *testing.T) {
	haveInt64 := false
	haveBool := false
	haveNull := false
	haveFullMap := false

	ExtractJSONStream(`{"name": "John Deo", 
"age": 30,

"isActive": true,

"address": null
}`, WithObjectKeyValue(func(key string, data any) {
		fmt.Println("--------------------------------------")
		spew.Dump(key, data)
		if data == 30 {
			log.Info("int64 found")
			haveInt64 = true
		}
		if data == true {
			haveBool = true
		}
		if data == nil {
			log.Info("nil found")
			haveNull = true
		}

		if reflect.ValueOf(data).Kind() == reflect.Map {
			haveFullMap = true
		}
		fmt.Println("--------------------------------------")
	}))
	assert.True(t, haveInt64)
	assert.True(t, haveBool)
	assert.True(t, haveNull)
	assert.True(t, haveFullMap)
}
