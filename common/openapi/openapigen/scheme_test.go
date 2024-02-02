package openapigen

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestJSONToScheme_INT(t *testing.T) {
	var raw, _ = anyToScheme(1).MarshalJSON()
	raw, _ = openapiyaml.JSONToYAML(raw)
	fmt.Println(string(raw))
	test := assert.New(t)
	test.True(utils.MatchAllOfSubString(string(raw), `type: integer`))
}

func TestJSONToScheme_FLOAT(t *testing.T) {
	var raw, _ = anyToScheme(1.1).MarshalJSON()
	raw, _ = openapiyaml.JSONToYAML(raw)
	fmt.Println(string(raw))
	test := assert.New(t)
	test.True(utils.MatchAnyOfSubString(string(raw), `type: float`, `type: double`, `type: number`), string(raw))
}

func TestJSONToScheme_B(t *testing.T) {
	var raw, _ = anyToScheme(true).MarshalJSON()
	raw, _ = openapiyaml.JSONToYAML(raw)
	test := assert.New(t)
	test.True(utils.MatchAllOfSubString(string(raw), `type: bool`), string(raw))
}

func TestJSONToScheme_Null(t *testing.T) {
	var raw, _ = anyToScheme(nil).MarshalJSON()
	raw, _ = openapiyaml.JSONToYAML(raw)
	test := assert.New(t)
	test.True(utils.MatchAllOfSubString(string(raw), `type: object`), string(raw))
}

func TestJToScheme_Object(t *testing.T) {
	var raw, _ = anyToScheme(`{"abc": 1}`).MarshalJSON()
	raw, _ = openapiyaml.JSONToYAML(raw)
	test := assert.New(t)
	test.True(utils.MatchAllOfSubString(string(raw), `type: object`, `abc:`, `properties:`), string(raw))
	fmt.Println(string(raw))
}

func TestJToScheme_Object1(t *testing.T) {
	var raw, _ = anyToScheme(`{"abc": 1, "b": {"e": "FFF"}}`).MarshalJSON()
	raw, _ = openapiyaml.JSONToYAML(raw)
	test := assert.New(t)
	test.True(utils.MatchAllOfSubString(string(raw), `type: object`, `abc:`, `properties:`), string(raw))
	fmt.Println(string(raw))
}
