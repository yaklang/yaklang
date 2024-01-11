package bruteutils

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestSSH(t *testing.T) {
	res := sshAuth.UnAuthVerify(&BruteItem{
		Target: "172.89.179.132",
	})
	spew.Dump(res)
}
