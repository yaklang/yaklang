package jsonextractor

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"reflect"
	"testing"
)

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
