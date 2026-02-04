package coreplugin_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/coreplugin"
)

func TestInitEmbedFS(t *testing.T) {
	coreplugin.InitEmbedFS()

	hash, err := coreplugin.CorePluginHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}
