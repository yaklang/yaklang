package coreplugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitEmbedFS(t *testing.T) {
	InitEmbedFS()

	hash, err := CorePluginHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}
