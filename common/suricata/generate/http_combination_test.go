package generate

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"testing"
)

func TestDefaultOrRandom(t *testing.T) {
	for k, v := range defaultRandom {
		res1 := v.Generate()
		t.Log(k, res1, '\n')
		assert.NotNil(t, res1, k)
	}
}

func TestHTTPCombination(t *testing.T) {
	t.Log(string(HTTPCombination(map[modifier.Modifier][]byte{
		modifier.HTTPMethod:      []byte("GET"),
		modifier.HTTPHeader:      []byte(`Abc: efg`),
		modifier.HTTPRequestBody: []byte("Hello World"),
	})))
	t.Log(string(HTTPCombination(map[modifier.Modifier][]byte{
		modifier.HTTPStatCode:     []byte("200"),
		modifier.HTTPHeader:       []byte(`Abc: efg`),
		modifier.HTTPResponseBody: []byte("Hello World"),
	})))
}
