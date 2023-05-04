package mutate

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestStrVisible(t *testing.T) {
	assert.True(t, strVisible("adasdfasdfas"))
	assert.True(t, strVisible("adasdfasdfas"))
	assert.False(t, strVisible("adasdfasdfa\x0a"))
	assert.False(t, strVisible("adasdfa\x00fas"))
	assert.True(t, strVisible("123ada_123dfasdfas"))
}
