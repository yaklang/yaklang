package openapi

import (
	"fmt"
	"strconv"
	"strings"
)

// IsValidResponseStatusCode 判断 responses 对象的 key 是否符合 OpenAPI 规范。
// 合法值：HTTP 状态码（100-599）或保留字 "default"。
func IsValidResponseStatusCode(code string) bool {
	code = strings.TrimSpace(code)
	if code == "default" {
		return true
	}
	if len(code) != 3 {
		return false
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(code)
	if err != nil {
		return false
	}
	return n >= 100 && n <= 599
}

var swaggerV2ParameterIn = map[string]struct{}{
	"query": {}, "header": {}, "path": {}, "formData": {}, "body": {},
}

var openAPIV3ParameterIn = map[string]struct{}{
	"query": {}, "header": {}, "path": {}, "cookie": {},
}

// IsValidParameterIn 判断 parameter.in 是否为规范允许的位置。
func IsValidParameterIn(in string, isSwaggerV2 bool) bool {
	in = strings.TrimSpace(in)
	if in == "" {
		return false
	}
	if isSwaggerV2 {
		_, ok := swaggerV2ParameterIn[in]
		return ok
	}
	_, ok := openAPIV3ParameterIn[in]
	return ok
}

var httpMethods = map[string]struct{}{
	"GET": {}, "POST": {}, "PUT": {}, "DELETE": {}, "PATCH": {}, "HEAD": {}, "OPTIONS": {}, "TRACE": {},
}

// IsValidHTTPMethod 判断 paths 下的 method key 是否为标准 HTTP 方法。
func IsValidHTTPMethod(method string) bool {
	_, ok := httpMethods[strings.ToUpper(strings.TrimSpace(method))]
	return ok
}

func formatOperationRef(method, path string) string {
	return fmt.Sprintf("%s %s", strings.ToUpper(method), path)
}
