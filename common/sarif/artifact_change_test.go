package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_create_simple_artifact_change(t *testing.T) {

	ac := NewArtifactChange(NewArtifactLocation().
		WithIndex(0).
		WithUri("file://broken.go").
		WithDescription(NewMessage().WithText("message text")))

	ac.WithReplacement(
		NewReplacement(
			NewRegion().
				WithSnippet(NewArtifactContent().WithText("file://broken.go")).
				WithStartLine(1).
				WithEndLine(10),
		),
	)

	assert.Equal(t, `{"artifactLocation":{"uri":"file://broken.go","index":0,"description":{"text":"message text"}},"replacements":[{"deletedRegion":{"startLine":1,"endLine":10,"snippet":{"text":"file://broken.go"}}}]}`, getJsonString(ac))
}
