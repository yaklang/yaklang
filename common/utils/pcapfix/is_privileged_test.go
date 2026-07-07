package pcapfix

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestIsPrivilegedForNetRaw(t *testing.T) {
	spew.Dump(IsPrivilegedForNetRaw())
}
