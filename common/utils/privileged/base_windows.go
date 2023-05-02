package privileged

import (
	"fmt"
	"io/ioutil"
	"os"
	"github.com/yaklang/yaklang/common/utils"
)

func isPrivileged() bool {
	f := fmt.Sprintf("C:/Windows/yak-tmp-%v.txt", utils.RandStringBytes(10))
	err := ioutil.WriteFile(f, []byte(utils.RandStringBytes(10)), 0644)
	if err != nil {
		fp, err := os.Open("\\\\.\\PHYSICALDRIVE0")
		if err != nil {
			return false
		}
		fp.Close()
		return true
	}
	defer func() {
		os.RemoveAll(f)
	}()
	return true
}
