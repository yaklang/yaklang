package har

import (
	"encoding/json"
	"io"

	"github.com/mailru/easyjson"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/jsonstream"
)

func ImportHTTPArchive(r io.Reader) (*HTTPArchive, error) {
	var har HTTPArchive
	err := easyjson.UnmarshalFromReader(r, &har)
	return &har, err
}

func ImportHTTPArchiveStream(r io.Reader, callback func(*HAREntry) error) error {
	dec := json.NewDecoder(r)
	jsonstream.Iterate(dec, func(path []json.Token) error {
		if !jsonstream.Match(path, []json.Token{"log", "entries", "*"}) {
			return nil
		}
		var entry HAREntry
		if err := dec.Decode(&entry); err != nil {
			return err
		}
		if callback != nil {
			return callback(&entry)
		}
		return nil
	})
	return nil
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
