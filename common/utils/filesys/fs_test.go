package filesys

import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"os"
	"testing"
)

//go:embed testdata
var testfs embed.FS

func TestFS(t *testing.T) {
	err := Recursive(
		"/Users/v1ll4n/Projects/yaklang",
		WithDirMatch("ut*", WithFileStat(func(pathname string, info os.FileInfo) error {
			log.Infof("match: %v", pathname)
			return nil
		})))
	if err != nil {
		t.Fatal(err)
	}
}
