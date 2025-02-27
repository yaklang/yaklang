package systemutils

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/privileged"
	"os/exec"
)

//网卡权限那边需要两个命令：
//
//1. chmod +rw /dev/bpf*
//2. chmod -rw /dev/bpf*
//
//一个是开放用户态网卡读写权限，一个是撤销用户网卡读写权限

var bpfSet = "chmod +rw /dev/bpf*"
var bpfUnset = "chmod -rw /dev/bpf*"

func ChmodBpfSet() error {
	var output []byte
	var err error
	if privileged.GetIsPrivileged() {
		cmd := exec.Command("sh", "-c", bpfSet)
		output, err = cmd.CombinedOutput()
	} else {
		output, err = privileged.NewExecutor("ChmodBpfSet").Execute(privileged.ExecuteOptions{
			Command:     bpfSet,
			Title:       "ChmodBpfSet",
			Prompt:      "To change the bpf permissions, please enter the administrator password.",
			Description: bpfSet,
		})
	}

	if err != nil {
		return utils.Errorf("chmod +rw fail: %v, output: %s", err, output)
	}
	return nil
}

func ChmodBbfUnset() error {
	var output []byte
	var err error
	if privileged.GetIsPrivileged() {
		cmd := exec.Command("sh", "-c", bpfUnset)
		output, err = cmd.CombinedOutput()
	} else {
		output, err = privileged.NewExecutor("ChmodBpfSet").Execute(privileged.ExecuteOptions{
			Command:     bpfUnset,
			Title:       "ChmodBpfUnSet",
			Prompt:      "To change the bpf permissions, please enter the administrator password.",
			Description: bpfUnset,
		})
	}

	if err != nil {
		return utils.Errorf("chmod +rw fail: %v, output: %s", err, output)
	}
	return nil
}
