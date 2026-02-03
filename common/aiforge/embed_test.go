package aiforge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitEmbedFS(t *testing.T) {
	InitEmbedFS()
	hash, err := BuildInForgeHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}
