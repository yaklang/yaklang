package systemproxy

import (
	"github.com/davecgh/go-spew/spew"
	"os/exec"
	"runtime"
	"testing"
)

func TestSet(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command(`osascript`, "-e", `do shell script "networksetup -setwebproxy Wi-Fi 127.0.0.1 8083; networksetup -setsecurewebproxy Wi-Fi 127.0.0.1 8083; networksetup -setsocksfirewallproxy Wi-Fi \"\" \"\"" with administrator privileges`)
		spew.Dump(cmd.Args)
		cmd.Args[0] = "AAAA"
		spew.Dump(cmd.Args)
		err := cmd.Run()
		if err != nil {
			panic(err)
		}
	}
}

func TestSet2(t *testing.T) {
	Set(Settings{
		Enabled:       false,
		DefaultServer: "http://127.0.0.1:7890",
	})
}
