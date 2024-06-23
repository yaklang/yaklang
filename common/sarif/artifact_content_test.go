package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_create_artifact_content(t *testing.T) {
	ac := NewArtifactContent()
	ac.WithText("artifact body").
		WithBinary("broken.exe").
		WithRendered(NewMultiformatMessageString("mms string content"))

	assert.Equal(t, `{"text":"artifact body","binary":"broken.exe","rendered":{"text":"mms string content"}}`, getJsonString(ac))
}
