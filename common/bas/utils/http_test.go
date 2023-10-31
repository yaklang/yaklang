// Package utils
// @Author bcy2007  2023/9/21 14:46
package utils

import (
	"encoding/json"
	"testing"
)

func TestHttpResultGet(t *testing.T) {
	resultStr := "{\"code\": 200, \"data\": {}, \"msg\": \"abc\"}"
	resultBytes := []byte(resultStr)
	var result Result
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Error(err)
	}
	t.Log(result.Code, result.Data, result.Msg)
}
