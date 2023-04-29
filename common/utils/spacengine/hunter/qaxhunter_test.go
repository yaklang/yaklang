package hunter

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestHunterQuery(t *testing.T) {
	hunter, err := HunterQuery("v1ll4n", "***", `web.title="北京"`, 1, 10)
	if err != nil {
		panic(err)
		return
	}
	spew.Dump(hunter)
}
