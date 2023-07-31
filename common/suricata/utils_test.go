package suricata

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSetIfNotZero(t *testing.T) {
	var a int
	assert.Equal(t, setIfNotZero(&a, 1), true)
	assert.Equal(t, a, 1)
	assert.Equal(t, setIfNotZero(&a, 0), false)
	assert.Equal(t, a, 1)
	var b string
	assert.Equal(t, setIfNotZero(&b, "1"), true)
	assert.Equal(t, b, "1")
	assert.Equal(t, setIfNotZero(&b, ""), false)
	assert.Equal(t, b, "1")
}
