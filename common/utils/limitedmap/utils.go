package limitedmap

import "reflect"

func isMapKeyString(v any) (result bool) {
	defer func() {
		if err := recover(); err != nil {
			result = false
		}
	}()
	result = reflect.TypeOf(v).Key().Kind() == reflect.String
	return
}

func setMapKeyValue(v any, key string, value any) {
	defer func() {
		if err := recover(); err != nil {
		}
	}()
	reflect.ValueOf(v).SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(value))
}
