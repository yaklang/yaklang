package javascript

import (
	"reflect"
	"strings"
)

func GetStatementType(st interface{}) string {
	typ := strings.Replace(reflect.TypeOf(st).String(), "*ast.", "", 1)
	return typ
}
