package suricata

import (
	"github.com/stretchr/testify/assert"
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
	t.Log(string(HTTPCombination(map[Modifier][]byte{
		HTTPMethod:      []byte("GET"),
		HTTPHeader:      []byte(`Abc: efg`),
		HTTPRequestBody: []byte("Hello World"),
	})))
}
