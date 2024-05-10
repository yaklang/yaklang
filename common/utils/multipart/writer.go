package multipart

import (
	"io"
	"mime/multipart"
)

func NewWriter(w io.Writer) *multipart.Writer {
	return multipart.NewWriter(w)
}
