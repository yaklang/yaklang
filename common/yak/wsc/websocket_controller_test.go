package wsc

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewWebsocketController(t *testing.T) {
	t.SkipNow()

	err := NewWebsocketController("", 8881).Run()
	assert.Nil(t, err)
}
