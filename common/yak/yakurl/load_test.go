package yakurl

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestLoadFromRaw(t *testing.T) {

	got, err := LoadFromRaw("behinder://https://aadfasdf/c?a=x")
	if err != nil {
		t.FailNow()
	}
	spew.Dump(got)

}
