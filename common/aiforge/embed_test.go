package aiforge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
)

func TestInitEmbedFS(t *testing.T) {
	InitEmbedFS()
	hash, err := BuildInForgeHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestGetBuildInForgeFromFS_DefaultAuthor(t *testing.T) {
	InitEmbedFS()

	forge, err := getBuildInForgeFromFS("web_log_monitor")
	assert.NoError(t, err)
	assert.NotNil(t, forge)
	assert.Equal(t, schema.AIResourceAuthorBuiltin, forge.Author)
}
