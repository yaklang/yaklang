package utils

import (
	"fmt"
	"github.com/h2non/filetype/matchers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
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

	for m := mime; m != nil; m = m.Parent() {
		if m.Is("text/plain") {
			return true, nil
		}
	}
	return false, nil
}
