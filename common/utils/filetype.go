package utils

import (
	"github.com/h2non/filetype/matchers"
	"yaklang.io/yaklang/common/log"
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
