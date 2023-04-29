package yak

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"yaklang/common/yak/yaklib"
	"testing"
)

func TestLineLogParser(t *testing.T) {
	test := assert.New(t)

	parser := NewTextParser()

	flag := false
	err := parser.ParseLine(bytes.NewBufferString(`{"key1":"value1"}{"key2": "asdfasdfasdf"}`), func(line string, r map[string]string, data []map[string]string) {
		flag = true
		spew.Dump(r, data)
	})
	if err != nil {
		test.FailNow(err.Error())
	}

	test.True(flag)
}

func TestExtractJsonToMap(t *testing.T) {
	test := assert.New(t)
	a := yaklib.JsonToMapList(`{"key1":"value1"}{"key2": "asdfasdfasdf"}`)
	if a == nil {
		test.FailNow("empty result")
		return
	}
	test.Len(a, 2)
}
