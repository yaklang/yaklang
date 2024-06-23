package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_create_simple_fix(t *testing.T) {
	fix := NewFix().
		WithDescription(NewMessage().
			WithText("fix text")).
		WithArtifactChanges([]*ArtifactChange{
			NewArtifactChange(
				NewArtifactLocation().
					WithUri("file://broken.go"),
			).WithReplacement(
				NewReplacement(NewRegion().
					WithStartLine(10).
					WithEndLine(11),
				),
			),
		})

	assert.Equal(t, `{"description":{"text":"fix text"},"artifactChanges":[{"artifactLocation":{"uri":"file://broken.go"},"replacements":[{"deletedRegion":{"startLine":10,"endLine":11}}]}]}`, getJsonString(fix))
}
