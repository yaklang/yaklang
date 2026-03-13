package yaklib

import (
	"bytes"
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestJsonToMapNestedStructure(t *testing.T) {
	// 嵌套 JSON：空对象、空数组、嵌套对象需完整保留，json.Marshal 可正确序列化
	input := `{"b":{},"c":[],"d":{"d1":""},"ss1s":"b"}`
	m := JsonToMap(input)
	assert.NotNil(t, m)
	assert.Equal(t, "b", m["ss1s"])
	assert.NotNil(t, m["b"])
	assert.NotNil(t, m["c"])
	assert.NotNil(t, m["d"])
	// 序列化后应与原结构一致
	out, err := json.Marshal(m)
	assert.NoError(t, err)
	var back map[string]any
	assert.NoError(t, json.Unmarshal(out, &back))
	assert.Equal(t, map[string]any{"b": map[string]any{}, "c": []any{}, "d": map[string]any{"d1": ""}, "ss1s": "b"}, back)
}

func TestJsonToMapListNestedStructure(t *testing.T) {
	// 多对象流，每个对象保留嵌套结构
	input := `{"a":1,"b":{"x":2}} {"c":[],"d":{"d1":""}}`
	list := JsonToMapList(input)
	assert.NotNil(t, list)
	assert.Len(t, list, 2)
	// 第一个对象
	assert.Equal(t, float64(1), list[0]["a"])
	assert.Equal(t, map[string]any{"x": float64(2)}, list[0]["b"])
	// 第二个对象
	assert.Equal(t, []any{}, list[1]["c"])
	assert.Equal(t, map[string]any{"d1": ""}, list[1]["d"])
	// json.Marshal 可正确序列化
	out, err := json.Marshal(list)
	assert.NoError(t, err)
	var back []map[string]any
	assert.NoError(t, json.Unmarshal(out, &back))
	assert.Len(t, back, 2)
}

func TestJsonStreamToMapListWithDepth(t *testing.T) {
	results := JsonStreamToMapListWithDepth(bytes.NewReader([]byte(`
{"a": 123}

{"b": 123}
<html>
{"c": 123}

{"e": 123}

{"e": 123}

{"f": {"123123123": 111}}
{"g": {"123123123": 111}}

`)), 0)
	spew.Dump(results)
}
