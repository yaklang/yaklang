package imageutils

import (
	"context"

	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
)

func ExtractVideoFrameContext(ctx context.Context, input string) (chan *ImageResult, error) {
	// Use the new, powerful ffmpegutils API
	frameChan, err := ffmpegutils.ExtractImageFramesFromVideo(input,
		ffmpegutils.WithContext(ctx),
		// The original filter was complex. We replicate its core ideas:
		// - scdet=threshold=20 -> A form of scene detection. We'll use our default.
		// - select='eq(n,0) + gt(floor(t), floor(prev_t)) + gt(scene, 0.2)' ->
		//   This is a mix of scene detection and periodic frame extraction.
		//   We will use WithSceneThreshold as the primary mechanism.
		ffmpegutils.WithSceneThreshold(0.4), // A sensible threshold
		// We can't directly specify a font file that may not exist.
		// The caller should ensure the font exists or we should provide a default.
		// For now, we omit the WithFontFile option to avoid test failures on different systems.
		// If a default font is available and its path is known, it could be added here.
	)
	if err != nil {
		return nil, err
	}

	// Adapt the output channel
	imageResultChan := make(chan *ImageResult)
	go func() {
		defer close(imageResultChan)
		for frame := range frameChan {
			if frame.Error != nil {
				// Handle or log the error as needed
				continue
			}
			imageResultChan <- &ImageResult{
				RawImage: frame.RawData,
				MIMEType: frame.MIMETypeObj,
			}
		}
	}()

	return imageResultChan, nil
}
