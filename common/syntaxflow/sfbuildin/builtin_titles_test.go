//go:build !gzip_embed && !irify_exclude

package sfbuildin

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuiltinRuleTitlesAreUnique(t *testing.T) {
	titles, titlesZh := make(map[string][]string), make(map[string][]string)
	for _, fixture := range builtinFixtures(t) {
		if title := fixture.rule.Title; title != "" {
			titles[title] = append(titles[title], fixture.path)
		}
		if title := fixture.rule.TitleZh; title != "" {
			titlesZh[title] = append(titlesZh[title], fixture.path)
		}
	}
	require.NoError(t, checkCollectedDuplicateTitles(titles, titlesZh))
}
