package crep

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewWebsocketController(t *testing.T) {
	err := NewWebsocketController("a", 8881).Run()
	assert.Nil(t, err)
}
