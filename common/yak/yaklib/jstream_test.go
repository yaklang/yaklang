package yaklib

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

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
