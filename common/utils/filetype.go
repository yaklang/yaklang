package utils

import (
	"fmt"
	"github.com/h2non/filetype/matchers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"strings"
)

func IsImage(i []byte) bool {
	for t, f := range matchers.Image {
		wrapper := func() bool {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("cannot identify image type: %s(%v)", t.MIME, t.Extension)
				}
			}()
			return f(i)
		}
		if wrapper() {
			return true
		}
	}
	return false
}

func IsGenericTextFile(filePath string) (bool, error) {
	mime, err := mimetype.DetectFile(filePath)
	if err != nil {
		return false, fmt.Errorf("connot detect mime type '%s': %w", filePath, err)
	}

	return IsGenericTextType(mime), nil
}

func IsGenericTextType(fileType *mimetype.MIME) bool {
	for m := fileType; m != nil; m = m.Parent() {
		if m.Is("text/plain") {
			return true
		}
	}
	return false
}

func IsMedia(filePath string) (bool, error) {
	mime, err := mimetype.DetectFile(filePath)
	if err != nil {
		return false, fmt.Errorf("cannot detect mime type for '%s': %w", filePath, err)
	}
	log.Printf("File '%s' detected as MIME: %s", filePath, mime.String())
	mimeStr := mime.String()
	isMedia := strings.HasPrefix(mimeStr, "video/") || strings.HasPrefix(mimeStr, "audio/")
	return isMedia, nil
}

func IsVideo(filePath string) (bool, error) {
	mime, err := mimetype.DetectFile(filePath)
	if err != nil {
		return false, fmt.Errorf("connot detect mime type '%s': %w", filePath, err)
	}
	return strings.HasPrefix(mime.String(), "video/"), nil
}

func IsAudio(filePath string) (bool, error) {
	mime, err := mimetype.DetectFile(filePath)
	if err != nil {
		return false, fmt.Errorf("cannot detect mime type for '%s': %w", filePath, err)
	}
	log.Infof("File '%s' detected as MIME: %s", filePath, mime.String())
	return strings.HasPrefix(mime.String(), "audio/"), nil
}
