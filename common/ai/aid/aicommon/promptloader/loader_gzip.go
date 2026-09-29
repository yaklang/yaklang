//go:build gzip_embed

package promptloader

import (
	"embed"
	"sync"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed prompts.tar.gz
var archiveFS embed.FS

var (
	archiveOnce sync.Once
	archive     *gzip_embed.PreprocessingEmbed
	archiveErr  error
)

func readPrompt(name string) ([]byte, error) {
	archiveOnce.Do(func() {
		archive, archiveErr = gzip_embed.NewPreprocessingEmbed(&archiveFS, "prompts.tar.gz", true)
	})
	if archiveErr != nil {
		return nil, archiveErr
	}
	return archive.ReadFile("prompts/" + name)
}
