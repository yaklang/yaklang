package yakurl

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestFuzzTagURL(t *testing.T) {
	rsp, err := LoadGetResource("fuzztag://")
	if err != nil {
		t.Fatal(err)
	}
	if len(rsp.Resources) <= 0 {
		t.Fatal("empty result for fuzztag")
	}
	spew.Dump(rsp)
	if len(rsp.Resources) < 10 {
		t.Fatal("unexpected result for fuzztag")
	}

	rsp, _ = LoadGetResource("fuzztag://randint")
	if len(rsp.Resources) < 1 {
		t.Fatal("unexpected result for fuzztag")
	}
}
