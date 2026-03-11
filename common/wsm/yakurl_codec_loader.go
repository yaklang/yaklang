package wsm

import (
	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func loadWebShellAndCodecScripts(
	projectDB *gorm.DB,
	profileDB *gorm.DB,
	id int64,
) (*ypb.WebShell, string, string, error) {
	shell, err := yakit.GetWebShell(projectDB, id)
	if err != nil {
		return nil, "", "", err
	}

	packetScript, err := getYakScriptContent(profileDB, shell.GetPacketCodecName())
	if err != nil {
		return nil, "", "", err
	}
	payloadScript, err := getYakScriptContent(profileDB, shell.GetPayloadCodecName())
	if err != nil {
		return nil, "", "", err
	}
	return shell, packetScript, payloadScript, nil
}

func getYakScriptContent(profileDB *gorm.DB, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	script, err := yakit.GetYakScriptByName(profileDB, name)
	if err != nil {
		return "", err
	}
	return script.Content, nil
}
