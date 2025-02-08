package har

import (
	"io"

	"github.com/mailru/easyjson"
	"github.com/yaklang/yaklang/common/utils"
)

func ImportHTTPArchive(r io.Reader) (*HTTPArchive, error) {
	var har HTTPArchive
	err := easyjson.UnmarshalFromReader(r, &har)
	return &har, err
}
func CountHTTPArchiveEntries(r io.Reader) (int, error) {
	count := 0
	har := &HTTPArchive{
		Log: &Log{
			Entries: &Entries{
				unmarshalEntryCallback: func(h *HAREntry) error {
					count++
					return nil
				},
			},
		},
	}
	err := easyjson.UnmarshalFromReader(r, har)
	return count, err
}

func ImportHTTPArchiveStream(r io.Reader, callback func(*HAREntry) error) error {
	har := &HTTPArchive{
		Log: &Log{
			Entries: &Entries{
				unmarshalEntryCallback: callback,
			},
		},
	}
	return easyjson.UnmarshalFromReader(r, har)
}

func ExportHTTPArchiveStream(w io.Writer, har *HTTPArchive) error {
	if har.Log == nil {
		return utils.Error("invalid HAR file, expect log field")
	}
	entries := har.Log.Entries
	if len(entries.Entries) == 0 && entries.entriesChannel == nil {
		return utils.Error("invalid HAR file, expect log.entries should not empty")
	}
	_, err := easyjson.MarshalToWriter(har, w)
	return err
}
