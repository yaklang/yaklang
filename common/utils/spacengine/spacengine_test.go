package spacengine

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestZoomeye(t *testing.T) {
	ch, err := ZoomeyeQuery("***", `port:80`, 1, 20)
	for result := range ch {
		if err != nil {
			t.Fatal(err)
			return
		}
		spew.Dump(result)
	}
}

func TestShodan(t *testing.T) {
	ch, err := ShodanQuery("***", `port:80`, 1, 20)
	for result := range ch {
		if err != nil {
			t.Fatal(err)
			return
		}
		spew.Dump(result)
	}
}
