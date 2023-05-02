package authhack

import (
	"github.com/yaklang/yaklang/common/utils"
)

var JWTExports = map[string]interface{}{
	"Parse": JwtParse,
	"JWTGenerate": func(alg string, i interface{}, key []byte) (string, error) {
		return JwtGenerate(alg, utils.InterfaceToMapInterface(i), "JWT", key)
	},
	"JWSGenerate": func(alg string, i interface{}, key []byte) (string, error) {
		return JwtGenerate(alg, utils.InterfaceToMapInterface(i), "JWS", key)
	},
	"RemoveAlg": JwtChangeAlgToNone,
	"AllAlgs":   AvailableJWTTokensAlgs,
}
