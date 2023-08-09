package suricata

import (
	"errors"
	"io"
	"net/http"
	"strings"
)

// untested
func newFileDataMatcher(r *ContentRule, data any) func(c *matchContext) error {
	return func(c *matchContext) error {
		// 10 MB
		var files []io.Reader
		switch data := data.(type) {
		case *http.Request:
			err := data.ParseMultipartForm(10 << 20)
			if !c.Must(err == nil) {
				return nil
			}
			for _, v := range data.MultipartForm.File {
				for _, f := range v {
					file, err := f.Open()
					if err != nil {
						continue
					}
					files = append(files, file)
				}
			}
		case *http.Response:
			ctype := data.Header.Get("Content-Type")
			if !c.Must(strings.HasPrefix(ctype, "application/octet-stream") ||
				strings.HasPrefix(ctype, "application/pdf") ||
				strings.HasPrefix(ctype, "image/") ||
				strings.HasPrefix(ctype, "audio/") ||
				strings.HasPrefix(ctype, "video/")) {
				return nil
			}
			files = append(files, data.Body)
		case nil:
			c.Reject()
			return nil
		default:
			c.Reject()
			return errors.New("unknown type for filedata matcher")
		}

		for _, f := range files {
			all, err := io.ReadAll(f)
			c.SetBuffer(FileData, all)
			if !c.Must(err == nil) {
				return nil
			}
			c.Insert(newPayloadMatcher(r, FileData))
			c.Recover()
			if err := c.Next(); err == nil {
				return err
			}
			if !c.IsRejected() {
				return nil
			}
		}
		c.Reject()
		return nil
	}
}
