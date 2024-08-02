package multipart

import (
	"io"
	"mime/multipart"

	"github.com/yaklang/yaklang/common/utils"
)

type Writer = multipart.Writer

func randomBrowserBoundary() string {
	boundary := "----WebKitFormBoundary"
	boundary += utils.RandAlphaNumStringBytes(16)
	return boundary
}

func NewWriter(w io.Writer) *multipart.Writer {
	writer := multipart.NewWriter(w)
	writer.SetBoundary(randomBrowserBoundary())
	return writer
}
