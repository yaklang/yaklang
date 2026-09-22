package sfbuildin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitEmbedFS(t *testing.T) {
	hash, err := ruleFSWithHash.GetHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}
