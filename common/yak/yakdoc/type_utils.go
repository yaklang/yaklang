package yakdoc

import (
	"fmt"
	"reflect"
	"strings"
)

func GetTypeNameWithPkgPath(typ reflect.Type) (pkgPath string, pkgPathName string) {
	rawName := typ.Name()
	if rawName == "" {
		rawName = typ.String()
	}

	typKind := typ.Kind()

	// pkgPath
	pkgPath = typ.PkgPath()

	if pkgPath == "" {
		innerTyp := typ
		if typKind == reflect.Slice || typKind == reflect.Array || typKind == reflect.Chan {
			innerTyp = typ.Elem()
			typKind = innerTyp.Kind()
		}
		if typKind == reflect.Ptr {
			innerTyp = innerTyp.Elem()
		}
		pkgPath = innerTyp.PkgPath()
	}

	// ignore pointer and struct different name
	if strings.HasPrefix(rawName, "*") {
		rawName = rawName[1:]
	}

	if pkgPath != "" {
		if strings.Contains(rawName, ".") {
			fixedPkgPath := pkgPath
			index := strings.LastIndex(fixedPkgPath, "/")
			if index == -1 {
				fixedPkgPath = ""
			} else {
				fixedPkgPath = fixedPkgPath[:index+1]
			}
			return pkgPath, fmt.Sprintf("%s%s", fixedPkgPath, rawName)
		}
		return pkgPath, fmt.Sprintf("%s.%s", pkgPath, rawName)
	}
	return pkgPath, rawName
}
