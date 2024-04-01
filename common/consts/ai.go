package consts

import "github.com/yaklang/yaklang/common/log"

// AI primary type, default is "openai"
var _aiPrimaryType string

// GetAIPrimaryType returns the primary type of AI
func GetAIPrimaryType() string {
	return _aiPrimaryType
}

func SetAIPrimaryType(t string) {
	// openai / chatglm / moonshot
	switch t {
	case "openai", "chatglm", "moonshot", "":
	default:
		log.Warnf("unstable AI primary type: %s", t)

	}
	_aiPrimaryType = t
}
