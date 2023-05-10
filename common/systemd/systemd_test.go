package systemd

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestNewSystemServiceConfig(t *testing.T) {
	f, c := NewSystemServiceConfig("test", WithServiceExecStart("test")).ToServiceFile()
	spew.Dump(f, c)
	println(string(c))
}
