package imageutils

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
)

func ExtractDocumentPagesContext(ctx context.Context, input string) (chan *ImageResult, error) {
	if utils.GetFirstExistedFile(input) == "" {
		return nil, utils.Errorf("%s file not existed", input)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	// todo like extract video frame ctx
	return nil, nil
}
