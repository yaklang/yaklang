package multipart

import (
	"io"
	"mime/multipart"
)

type Writer = multipart.Writer

func NewWriter(w io.Writer) *multipart.Writer {
	return multipart.NewWriter(w)
}
