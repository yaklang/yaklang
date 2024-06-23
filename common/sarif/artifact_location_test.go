package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_create_new_artifact_location(t *testing.T) {
	al := NewArtifactLocation().
		WithIndex(0).
		WithUri("file://broken.go").
		WithUriBaseId("baseId").
		WithDescription(NewMessage().WithText("message text"))

	assert.Equal(t, `{"uri":"file://broken.go","uriBaseId":"baseId","index":0,"description":{"text":"message text"}}`, getJsonString(al))
}
