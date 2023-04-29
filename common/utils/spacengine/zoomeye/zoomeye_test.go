package zoomeye

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestZoomeyeQuery(t *testing.T) {
	hunter, err := ZoomeyeQuery("***", `port:80`, 1)
	if err != nil {
		panic(err)
		return
	}
	spew.Dump(hunter)
}
