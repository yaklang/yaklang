package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_create_new_location_relationship(t *testing.T) {
	l := NewLocationRelationship(1).
		WithDescription(
			NewMessage().
				WithMarkdown("# markdown text"),
		).
		WithKinds([]string{"kind", "another kind"})

	assert.Equal(t, `{"target":1,"kinds":["kind","another kind"],"description":{"markdown":"# markdown text"}}`, getJsonString(l))
}
