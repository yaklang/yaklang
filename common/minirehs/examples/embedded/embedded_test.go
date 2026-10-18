package embedded

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/yaklang/yaklang/common/minirehs"
)

//go:embed patterns.json
var patternsJSON []byte

func TestEmbeddedImageFresh(t *testing.T) {
	var pats []minirehs.Pattern
	if err := json.Unmarshal(patternsJSON, &pats); err != nil {
		t.Fatal(err)
	}
	db, err := minirehs.Compile(pats, minirehs.WithBackend(minirehs.BackendMVS), minirehs.WithReportLocation(false))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	image, err := db.MarshalImage()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(image, rulesImage) {
		t.Fatal("embedded image is stale; run go generate ./common/minirehs/examples/embedded")
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	if err = loaded.Scan([]byte("HTTP/1.1 200 OK\r\nServer: example\r\n"), nil, nil); err != nil {
		t.Fatal(err)
	}
}
