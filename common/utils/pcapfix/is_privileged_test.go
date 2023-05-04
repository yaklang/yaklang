package pcapfix

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestIsPrivilegedForNetRaw(t *testing.T) {
	spew.Dump(IsPrivilegedForNetRaw())
}
