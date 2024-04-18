package crep

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"os"
)

//go:embed scripts/auto-install-cert.bat
var batScript []byte

//go:embed scripts/auto-install-cert.sh
var shScript []byte

func genScriptFile() {
	// 如果证书已经安装，不再生成
	if err := VerifySystemCertificate(); err == nil {
		return
	}
	if utils.IsWindows() {
		if utils.IsFile(batScriptFile) {
			return
		}
		err := ioutil.WriteFile(batScriptFile, batScript, 0444)
		if err != nil {
			log.Errorf("write bat script failed: %s", err)
		}
	} else if utils.IsLinux() || utils.IsMac() {
		if utils.IsFile(shScriptFile) {
			return
		}
		err := ioutil.WriteFile(shScriptFile, shScript, 0444)
		if err != nil {
			log.Errorf("write sh script failed: %s", err)
		}
		err = os.Chmod(shScriptFile, 0755)
		if err != nil {
			log.Errorf("chmod sh script failed: %s", err)
		}
	}
}
