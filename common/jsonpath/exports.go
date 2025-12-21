package jsonpath

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
)

func ReplaceAll(j interface{}, jpath string, replaceValue interface{}) any {
	raw := utils.InterfaceToBytes(j)
	var m interface{}
	err := json.Unmarshal(raw, &m)
	if err != nil {
		log.Errorf("unmarshal json failed: %s", err)
		return nil
	}
	result, err := Replace(j, jpath, replaceValue)
	if err != nil {
		log.Errorf("replace jsonpath failed: %s", err)
		return nil
	}
	return result
}

func Find(j interface{}, jpath string) interface{} {
	raw := utils.InterfaceToBytes(j)
	var i interface{}
	err := json.Unmarshal(raw, &i)
	if err != nil {
		log.Errorf("unmarshal json failed: %s, raw: %v", err, utils.ShrinkTextBlock(j, 256))
		return nil
	}
	result, err := Read(i, jpath)
	if err != nil {
		log.Errorf("read jsonpath failed: %s", err)
		return nil
	}
	return result
}

func FindFirst(j interface{}, jpath string) interface{} {
	result := Find(j, jpath)
	if result == nil {
		return result
	}
	switch reflect.TypeOf(result).Kind() {
	case reflect.Slice, reflect.Array:
		value := reflect.ValueOf(result)
		if value.Len() > 0 {
			return value.Index(0).Interface()
		}
		return nil
	default:
		return result
	}
}
